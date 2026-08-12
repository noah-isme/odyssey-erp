package treasury

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"strings"

	fxservice "github.com/odyssey-erp/odyssey-erp/internal/fx"
)

// Treasury amounts are stored in NUMERIC(19,4). Keeping the wire value as a
// decimal string means request values and database values never pass through
// float64 (which would make otherwise valid cent/fraction amounts lossy).
const (
	AmountPrecision = 19
	AmountScale     = 4
)

var (
	ErrInvalidAmount = errors.New("treasury: invalid amount")
	decimalPattern   = regexp.MustCompile(`^-?[0-9]+(?:\.[0-9]{1,4})?$`)
)

// Amount is a validated decimal compatible with treasury NUMERIC(19,4)
// columns. It marshals as a JSON string, as required for exact money APIs.
type Amount string

// ParseAmount validates and canonicalizes a decimal amount. Leading zeroes
// are removed while fractional precision is retained, so callers can choose
// whether to display two or four decimal places without changing its value.
func ParseAmount(value string) (Amount, error) {
	value = strings.TrimSpace(value)
	if value == "" || !decimalPattern.MatchString(value) {
		return "", fmt.Errorf("%w: expected a decimal string with at most %d fractional digits", ErrInvalidAmount, AmountScale)
	}

	negative := strings.HasPrefix(value, "-")
	if negative {
		value = value[1:]
	}
	parts := strings.SplitN(value, ".", 2)
	integer := strings.TrimLeft(parts[0], "0")
	if integer == "" {
		integer = "0"
	}
	if len(integer) > AmountPrecision-AmountScale {
		return "", fmt.Errorf("%w: exceeds NUMERIC(%d,%d) precision", ErrInvalidAmount, AmountPrecision, AmountScale)
	}

	fraction := ""
	if len(parts) == 2 {
		fraction = parts[1]
	}
	// A negative zero is not useful as a persisted payment amount and can
	// produce confusing fingerprints, so canonicalize it to a positive zero.
	if integer == "0" && strings.Trim(fraction, "0") == "" {
		negative = false
	}

	result := integer
	if fraction != "" {
		result += "." + fraction
	}
	if negative {
		result = "-" + result
	}
	return Amount(result), nil
}

// MustParseAmount is intended for static fixtures and tests.
func MustParseAmount(value string) Amount {
	amount, err := ParseAmount(value)
	if err != nil {
		panic(err)
	}
	return amount
}

func (a Amount) String() string {
	if a == "" {
		return "0"
	}
	return string(a)
}

func (a Amount) Validate() error {
	_, err := ParseAmount(a.String())
	return err
}

func (a Amount) IsPositive() bool {
	r, err := a.rat()
	return err == nil && r.Sign() > 0
}

// Add performs exact rational addition and emits no more fractional digits
// than the two operands contain. Since both operands are validated decimal
// strings, this remains exact and compatible with NUMERIC(19,4).
func (a Amount) Add(other Amount) (Amount, error) {
	left, err := a.rat()
	if err != nil {
		return "", err
	}
	right, err := other.rat()
	if err != nil {
		return "", err
	}
	scale := maxDecimalPlaces(a.String(), other.String())
	return ParseAmount(new(big.Rat).Add(left, right).FloatString(scale))
}

func (a Amount) Cmp(other Amount) (int, error) {
	left, err := a.rat()
	if err != nil {
		return 0, err
	}
	right, err := other.rat()
	if err != nil {
		return 0, err
	}
	return left.Cmp(right), nil
}

func (a Amount) rat() (*big.Rat, error) {
	parsed, err := ParseAmount(a.String())
	if err != nil {
		return nil, err
	}
	r, ok := new(big.Rat).SetString(parsed.String())
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrInvalidAmount, parsed)
	}
	return r, nil
}

func (a Amount) MarshalJSON() ([]byte, error) {
	if err := a.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(a.String())
}

func (a *Amount) UnmarshalJSON(data []byte) error {
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return fmt.Errorf("%w: amount must be a JSON string", ErrInvalidAmount)
	}
	parsed, err := ParseAmount(value)
	if err != nil {
		return err
	}
	*a = parsed
	return nil
}

func maxDecimalPlaces(values ...string) int {
	max := 0
	for _, value := range values {
		if dot := strings.IndexByte(value, '.'); dot >= 0 && len(value)-dot-1 > max {
			max = len(value) - dot - 1
		}
	}
	return max
}

func normalizeCurrency(value string) (string, error) {
	currency, err := fxservice.Currency(value)
	if err != nil {
		return "", err
	}
	return currency, nil
}
