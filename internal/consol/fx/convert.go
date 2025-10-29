package fx

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// Converter applies FX policy rules to consolidated balances.
type Converter struct {
	policy Policy
	quotes map[string]Quote
}

// Quote represents the configured FX rates for a currency pair.
type Quote struct {
	Average float64
	Closing float64
}

// MissingRateError indicates one or more FX rates are unavailable.
type MissingRateError struct {
	Pairs []string
}

func (e *MissingRateError) Error() string {
	if e == nil || len(e.Pairs) == 0 {
		return "fx: missing rates"
	}
	parts := append([]string(nil), e.Pairs...)
	sort.Strings(parts)
	return fmt.Sprintf("fx: missing rates for %s", strings.Join(parts, ", "))
}

// NewConverter constructs a converter instance.
func NewConverter(policy Policy, quotes map[string]Quote) *Converter {
	if policy.ProfitLossMethod == "" {
		policy.ProfitLossMethod = MethodAverage
	}
	if policy.BalanceSheetMethod == "" {
		policy.BalanceSheetMethod = MethodClosing
	}
	normalised := make(map[string]Quote, len(quotes))
	for pair, quote := range quotes {
		normalised[strings.ToUpper(strings.TrimSpace(pair))] = quote
	}
	return &Converter{policy: policy, quotes: normalised}
}

// ErrConversionNotImplemented signals the FX conversion logic still needs to be delivered.
var ErrConversionNotImplemented = errors.New("consol: fx conversion not implemented")

// ConvertProfitLoss applies the configured FX policy to P&L amounts.
func (c *Converter) ConvertProfitLoss(input []Line) ([]Line, float64, error) {
	if c == nil {
		return nil, 0, ErrConversionNotImplemented
	}
	return c.convert(c.policy.ProfitLossMethod, input)
}

// ConvertBalanceSheet applies the configured FX policy to balance sheet amounts.
func (c *Converter) ConvertBalanceSheet(input []Line) ([]Line, float64, error) {
	if c == nil {
		return nil, 0, ErrConversionNotImplemented
	}
	return c.convert(c.policy.BalanceSheetMethod, input)
}

// Line is a simplified representation of an amount eligible for FX conversion.
type Line struct {
	AccountCode   string
	LocalCurrency string
	LocalAmount   float64
	GroupAmount   float64
}

func (c *Converter) convert(method Method, input []Line) ([]Line, float64, error) {
	if len(input) == 0 {
		return nil, 0, nil
	}
	target := strings.ToUpper(strings.TrimSpace(c.policy.ReportingCurrency))
	if target == "" {
		target = "IDR"
	}
	converted := make([]Line, len(input))
	missing := make(map[string]struct{})
	var delta float64
	for i, line := range input {
		local := strings.ToUpper(strings.TrimSpace(line.LocalCurrency))
		if local == "" {
			local = target
		}
		rate, pair, ok := c.rateFor(local, target, method)
		if !ok {
			if pair != "" {
				missing[pair] = struct{}{}
			}
			converted[i] = line
			continue
		}
		converted[i] = line
		converted[i].GroupAmount = line.LocalAmount * rate
		delta += converted[i].GroupAmount - line.GroupAmount
	}
	if len(missing) > 0 {
		pairs := make([]string, 0, len(missing))
		for pair := range missing {
			pairs = append(pairs, pair)
		}
		return nil, 0, &MissingRateError{Pairs: pairs}
	}
	return converted, delta, nil
}

func (c *Converter) rateFor(local, target string, method Method) (float64, string, bool) {
	if local == "" || local == target {
		return 1, "", true
	}
	pair := local + target
	quote, ok := c.quotes[pair]
	if !ok {
		return 0, pair, false
	}
	switch method {
	case MethodClosing:
		if quote.Closing <= 0 {
			return 0, pair, false
		}
		return quote.Closing, pair, true
	default:
		if quote.Average <= 0 {
			return 0, pair, false
		}
		return quote.Average, pair, true
	}
}
