package fx

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"strings"
	"time"
)

var isoCurrency = regexp.MustCompile(`^[A-Z]{3}$`)

var (
	ErrRateNotFound         = errors.New("fx: rate not found")
	ErrRateStale            = errors.New("fx: rate is stale")
	ErrInvalidCurrency      = errors.New("fx: invalid currency")
	ErrInvalidRate          = errors.New("fx: invalid rate")
	ErrManualOverrideDenied = errors.New("fx: manual rate override requires authorization")
)

// Decimal is an exact decimal value. It is intentionally backed by a string
// at API boundaries so NUMERIC values never pass through float64.
type Decimal struct{ value *big.Rat }

func ParseDecimal(s string) (Decimal, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Decimal{}, fmt.Errorf("empty decimal")
	}
	r, ok := new(big.Rat).SetString(s)
	if !ok {
		return Decimal{}, fmt.Errorf("invalid decimal %q", s)
	}
	return Decimal{value: r}, nil
}

func MustDecimal(s string) Decimal {
	d, err := ParseDecimal(s)
	if err != nil {
		panic(err)
	}
	return d
}
func (d Decimal) String() string {
	if d.value == nil {
		return "0"
	}
	return d.value.FloatString(10)
}
func (d Decimal) IsZero() bool              { return d.value == nil || d.value.Sign() == 0 }
func (d Decimal) Cmp(other Decimal) int     { return d.rat().Cmp(other.rat()) }
func (d Decimal) Add(other Decimal) Decimal { return Decimal{new(big.Rat).Add(d.rat(), other.rat())} }
func (d Decimal) Sub(other Decimal) Decimal { return Decimal{new(big.Rat).Sub(d.rat(), other.rat())} }
func (d Decimal) Mul(other Decimal) Decimal { return Decimal{new(big.Rat).Mul(d.rat(), other.rat())} }
func (d Decimal) Div(other Decimal) Decimal { return Decimal{new(big.Rat).Quo(d.rat(), other.rat())} }
func (d Decimal) Round(scale int) Decimal {
	if scale < 0 {
		scale = 0
	}
	den := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(scale)), nil)
	n := new(big.Int).Mul(d.rat().Num(), den)
	q := new(big.Int).Quo(n, d.rat().Denom())
	rem := new(big.Int).Rem(n, d.rat().Denom())
	if new(big.Int).Abs(rem).Mul(new(big.Int).Abs(rem), big.NewInt(2)).Cmp(d.rat().Denom()) >= 0 {
		q.Add(q, big.NewInt(int64(d.rat().Sign())))
	}
	return Decimal{new(big.Rat).SetFrac(q, den)}
}
func (d Decimal) rat() *big.Rat {
	if d.value == nil {
		return new(big.Rat)
	}
	return new(big.Rat).Set(d.value)
}

func Currency(code string) (string, error) {
	code = strings.ToUpper(strings.TrimSpace(code))
	if !isoCurrency.MatchString(code) {
		return "", fmt.Errorf("%w: %q", ErrInvalidCurrency, code)
	}
	return code, nil
}

type FXQuote struct {
	BaseCurrency, QuoteCurrency string
	Rate                        Decimal
	RateDate                    time.Time
	Source                      string
	SourceReference             string
	ProviderUpdatedAt           time.Time
	RawPayloadHash              string
}
type FXQuoteSet struct {
	BaseCurrency      string
	RateDate          time.Time
	Source            string
	SourceReference   string
	ProviderUpdatedAt time.Time
	RawPayloadHash    string
	Rates             map[string]Decimal
}

// Quote returns a direct or inverse quote from a provider response without
// introducing floating-point arithmetic.
func (s FXQuoteSet) Quote(quoteCurrency string) (FXQuote, error) {
	quote, err := Currency(quoteCurrency)
	if err != nil {
		return FXQuote{}, err
	}
	if quote == s.BaseCurrency {
		return FXQuote{BaseCurrency: s.BaseCurrency, QuoteCurrency: quote, Rate: MustDecimal("1"), RateDate: s.RateDate, Source: s.Source, SourceReference: s.SourceReference, ProviderUpdatedAt: s.ProviderUpdatedAt, RawPayloadHash: s.RawPayloadHash}, nil
	}
	rate, ok := s.Rates[quote]
	if !ok || rate.Cmp(MustDecimal("0")) <= 0 {
		return FXQuote{}, fmt.Errorf("%w: %s/%s", ErrRateNotFound, s.BaseCurrency, quote)
	}
	return FXQuote{BaseCurrency: s.BaseCurrency, QuoteCurrency: quote, Rate: rate, RateDate: s.RateDate, Source: s.Source, SourceReference: s.SourceReference, ProviderUpdatedAt: s.ProviderUpdatedAt, RawPayloadHash: s.RawPayloadHash}, nil
}

type FXProvider interface {
	DailyRates(context.Context, string, time.Time) (FXQuoteSet, error)
}

type RateRepository interface {
	UpsertDailyRates(context.Context, FXQuoteSet) error
	RecordFetchRun(context.Context, FetchRun) error
	DailyRate(context.Context, string, string, time.Time, time.Duration) (FXQuote, error)
}
type FetchRun struct {
	RateDate                                   time.Time
	Source, Status, ResponseHash, ErrorMessage string
}

type Resolver struct {
	Repo   RateRepository
	MaxAge time.Duration
	Now    func() time.Time
}

func (r Resolver) Resolve(ctx context.Context, base, quote string, date time.Time) (FXQuote, error) {
	base, err := Currency(base)
	if err != nil {
		return FXQuote{}, err
	}
	quote, err = Currency(quote)
	if err != nil {
		return FXQuote{}, err
	}
	if base == quote {
		return FXQuote{BaseCurrency: base, QuoteCurrency: quote, Rate: MustDecimal("1"), RateDate: date, Source: "INTERNAL"}, nil
	}
	if r.Repo == nil {
		return FXQuote{}, ErrRateNotFound
	}
	if r.MaxAge <= 0 {
		r.MaxAge = 48 * time.Hour
	}
	return r.Repo.DailyRate(ctx, base, quote, date, r.MaxAge)
}

func CalculateBaseAmount(original, rate Decimal) (Decimal, error) {
	if original.Cmp(MustDecimal("0")) < 0 || rate.Cmp(MustDecimal("0")) <= 0 {
		return Decimal{}, ErrInvalidRate
	}
	return original.Mul(rate).Round(2), nil
}

type PaymentValuation struct{ CarryingBaseAmount, SettlementBaseAmount, RealizedDifference Decimal }

func CalculatePaymentValuation(allocated, invoiceRate, paymentRate Decimal) (PaymentValuation, error) {
	if allocated.Cmp(MustDecimal("0")) < 0 {
		return PaymentValuation{}, fmt.Errorf("allocated amount must not be negative")
	}
	carrying, err := CalculateBaseAmount(allocated, invoiceRate)
	if err != nil {
		return PaymentValuation{}, err
	}
	settlement, err := CalculateBaseAmount(allocated, paymentRate)
	if err != nil {
		return PaymentValuation{}, err
	}
	return PaymentValuation{carrying, settlement, settlement.Sub(carrying).Round(2)}, nil
}
