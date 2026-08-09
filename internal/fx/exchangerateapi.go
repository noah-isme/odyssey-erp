package fx

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type ExchangeRateAPI struct{ cfg ProviderConfig }

func NewExchangeRateAPI(cfg ProviderConfig) *ExchangeRateAPI {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://open.er-api.com/v6"
	}
	if cfg.Source == "" {
		cfg.Source = "EXCHANGERATE_API"
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 10 * time.Second
	}
	if cfg.Client == nil {
		cfg.Client = &http.Client{Timeout: cfg.Timeout}
	}
	return &ExchangeRateAPI{cfg: cfg}
}

type exchangeResponse struct {
	Result             string                 `json:"result"`
	ErrorType          string                 `json:"error-type"`
	TimeLastUpdateUnix int64                  `json:"time_last_update_unix"`
	Rates              map[string]json.Number `json:"rates"`
}

func (p *ExchangeRateAPI) DailyRates(ctx context.Context, baseCurrency string, date time.Time) (FXQuoteSet, error) {
	base, err := Currency(baseCurrency)
	if err != nil {
		return FXQuoteSet{}, err
	}
	requestedDate := date.Format("2006-01-02")
	today := time.Now().Format("2006-01-02")
	if requestedDate != today && p.cfg.APIKey == "" {
		return FXQuoteSet{}, fmt.Errorf("exchange rate api: historical rates require an API key")
	}
	path := "latest/" + url.PathEscape(base)
	if requestedDate != today {
		path = fmt.Sprintf("history/%s/%s/%s/%s", url.PathEscape(base), date.Format("2006"), date.Format("1"), date.Format("2"))
	}
	endpoint := strings.TrimRight(p.cfg.BaseURL, "/") + "/" + path
	if p.cfg.APIKey != "" {
		endpoint = strings.TrimRight(p.cfg.BaseURL, "/") + "/" + url.PathEscape(p.cfg.APIKey) + "/" + path
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		// Never return a URL-bearing error: API-key based providers put the
		// credential in the request path.
		return FXQuoteSet{}, &ProviderError{Kind: ErrorMalformed, Err: fmt.Errorf("invalid provider request")}
	}
	resp, err := p.cfg.Client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return FXQuoteSet{}, &ProviderError{Kind: ErrorTimeout, Err: ctx.Err()}
		}
		return FXQuoteSet{}, &ProviderError{Kind: ErrorTimeout, Err: fmt.Errorf("provider request failed")}
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return FXQuoteSet{}, &ProviderError{Kind: ErrorMalformed, StatusCode: resp.StatusCode, Err: fmt.Errorf("provider response could not be read")}
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return FXQuoteSet{}, &ProviderError{Kind: ErrorAuthentication, StatusCode: resp.StatusCode, Err: fmt.Errorf("provider rejected credentials")}
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return FXQuoteSet{}, &ProviderError{Kind: ErrorQuota, StatusCode: resp.StatusCode, Err: fmt.Errorf("provider quota exceeded")}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return FXQuoteSet{}, &ProviderError{Kind: ErrorMalformed, StatusCode: resp.StatusCode, Err: fmt.Errorf("unexpected HTTP status")}
	}
	var payload exchangeResponse
	if err := json.Unmarshal(body, &payload); err != nil || payload.Result != "success" || len(payload.Rates) == 0 {
		if err == nil {
			err = fmt.Errorf("result=%q or rates missing", payload.Result)
		}
		kind := ErrorMalformed
		if strings.Contains(strings.ToLower(payload.ErrorType), "quota") {
			kind = ErrorQuota
		}
		if strings.Contains(strings.ToLower(payload.ErrorType), "key") || strings.Contains(strings.ToLower(payload.ErrorType), "auth") {
			kind = ErrorAuthentication
		}
		return FXQuoteSet{}, &ProviderError{Kind: kind, StatusCode: resp.StatusCode, Err: err}
	}
	rates := make(map[string]Decimal, len(payload.Rates))
	for code, raw := range payload.Rates {
		c, e := Currency(code)
		if e != nil {
			return FXQuoteSet{}, &ProviderError{Kind: ErrorMalformed, Err: e}
		}
		d, e := ParseDecimal(raw.String())
		if e != nil || d.Cmp(MustDecimal("0")) <= 0 {
			if e == nil {
				e = ErrInvalidRate
			}
			return FXQuoteSet{}, &ProviderError{Kind: ErrorMalformed, Err: fmt.Errorf("%s: %w", c, e)}
		}
		rates[c] = d
	}
	return FXQuoteSet{BaseCurrency: base, RateDate: date, Source: p.cfg.Source, ProviderUpdatedAt: time.Unix(payload.TimeLastUpdateUnix, 0).UTC(), RawPayloadHash: hash(body), Rates: rates}, nil
}
func hash(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
