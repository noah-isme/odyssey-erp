package fx

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

// QuoteProvider exposes lookup for FX quotes on a given period.
type QuoteProvider interface {
	QuoteForPeriod(ctx context.Context, asOf time.Time, pair string) (Quote, bool, error)
}

// Requirement declares which FX conversion methods must be available for a pair.
type Requirement struct {
	Pair    string
	Methods []Method
}

// Gap contains missing conversion methods for a pair.
type Gap struct {
	Pair    string
	Methods []Method
}

// Result summarises the validation outcome.
type Result struct {
	Period    time.Time
	Checked   int
	Gaps      []Gap
	Available map[string]Quote
}

// Validate ensures all requested FX conversion methods are configured for the given period.
func Validate(ctx context.Context, provider QuoteProvider, asOf time.Time, reqs []Requirement) (Result, error) {
	var res Result
	if provider == nil {
		return res, fmt.Errorf("fx: quote provider required")
	}
	if asOf.IsZero() {
		return res, fmt.Errorf("fx: period is required")
	}
	period := time.Date(asOf.Year(), asOf.Month(), 1, 0, 0, 0, 0, time.UTC)
	res.Period = period
	if len(reqs) == 0 {
		res.Available = map[string]Quote{}
		return res, nil
	}
	pairs := make(map[string]map[Method]struct{})
	for _, req := range reqs {
		pair := strings.ToUpper(strings.TrimSpace(req.Pair))
		if pair == "" {
			return Result{}, fmt.Errorf("fx: pair required")
		}
		if len(req.Methods) == 0 {
			return Result{}, fmt.Errorf("fx: methods required for pair %s", pair)
		}
		methodSet := pairs[pair]
		if methodSet == nil {
			methodSet = make(map[Method]struct{}, len(req.Methods))
			pairs[pair] = methodSet
		}
		for _, method := range req.Methods {
			switch method {
			case MethodAverage, MethodClosing:
				methodSet[method] = struct{}{}
			default:
				return Result{}, fmt.Errorf("fx: unsupported method %q for pair %s", method, pair)
			}
		}
	}
	res.Available = make(map[string]Quote, len(pairs))
	res.Gaps = make([]Gap, 0)
	keys := make([]string, 0, len(pairs))
	for pair := range pairs {
		keys = append(keys, pair)
	}
	sort.Strings(keys)
	for _, pair := range keys {
		quote, ok, err := provider.QuoteForPeriod(ctx, period, pair)
		if err != nil {
			return Result{}, err
		}
		res.Checked++
		if !ok {
			missing := sortedMethods(pairs[pair])
			res.Gaps = append(res.Gaps, Gap{Pair: pair, Methods: missing})
			continue
		}
		res.Available[pair] = quote
		missing := missingMethods(quote, pairs[pair])
		if len(missing) > 0 {
			res.Gaps = append(res.Gaps, Gap{Pair: pair, Methods: missing})
		}
	}
	return res, nil
}

func sortedMethods(methods map[Method]struct{}) []Method {
	out := make([]Method, 0, len(methods))
	for method := range methods {
		out = append(out, method)
	}
	sort.Slice(out, func(i, j int) bool { return string(out[i]) < string(out[j]) })
	return out
}

func missingMethods(quote Quote, required map[Method]struct{}) []Method {
	missing := make([]Method, 0, len(required))
	for method := range required {
		switch method {
		case MethodAverage:
			if quote.Average <= 0 {
				missing = append(missing, MethodAverage)
			}
		case MethodClosing:
			if quote.Closing <= 0 {
				missing = append(missing, MethodClosing)
			}
		}
	}
	if len(missing) > 1 {
		sort.Slice(missing, func(i, j int) bool { return string(missing[i]) < string(missing[j]) })
	}
	return missing
}
