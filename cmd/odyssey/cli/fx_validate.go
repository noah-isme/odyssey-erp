package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/odyssey-erp/odyssey-erp/internal/consol/fx"
)

// FXValidateOptions defines available flags for the fx validate command.
type FXValidateOptions struct {
	GroupID    int64
	Period     string
	Pairs      []string
	JSONOutput bool
	Stdout     io.Writer
	Stderr     io.Writer
}

// FXValidateSummary describes the JSON response for fx validate.
type FXValidateSummary struct {
	OK              bool                       `json:"ok"`
	Gaps            []FXValidationGap          `json:"gaps"`
	AvailableQuotes []FXValidationAvailability `json:"available_quotes"`
}

// FXValidationGap captures a missing FX method for a pair.
type FXValidationGap struct {
	Pair   string `json:"pair"`
	Period string `json:"period"`
	Method string `json:"method"`
}

// FXValidationAvailability reports a configured FX quote.
type FXValidationAvailability struct {
	Pair   string `json:"pair"`
	Period string `json:"period"`
	Method string `json:"method"`
}

// ValidateCommand executes the fx validate workflow and prints the outcome.
func (c *FXOpsCLI) ValidateCommand(ctx context.Context, opts FXValidateOptions) int {
	if opts.Stdout == nil {
		opts.Stdout = os.Stdout
	}
	if opts.Stderr == nil {
		opts.Stderr = os.Stderr
	}
	if opts.GroupID <= 0 {
		_, _ = fmt.Fprintln(opts.Stderr, "fx validate: --group is required and must be positive")
		return 1
	}
	period, err := time.Parse("2006-01", strings.TrimSpace(opts.Period))
	if err != nil {
		_, _ = fmt.Fprintf(opts.Stderr, "fx validate: invalid period %q (expected YYYY-MM)\n", opts.Period)
		return 1
	}
	result, err := c.ValidateGaps(ctx, ValidateParams{GroupID: opts.GroupID, Period: period, Pairs: opts.Pairs})
	if err != nil {
		_, _ = fmt.Fprintf(opts.Stderr, "fx validate: %v\n", err)
		return 1
	}
	if opts.JSONOutput {
		summary := buildValidateSummary(result)
		if err := json.NewEncoder(opts.Stdout).Encode(summary); err != nil {
			_, _ = fmt.Fprintf(opts.Stderr, "fx validate: encode json: %v\n", err)
			return 1
		}
	} else {
		renderValidateHuman(opts.Stdout, result)
	}
	if len(result.Result.Gaps) > 0 {
		return 10
	}
	return 0
}

func buildValidateSummary(result ValidateResult) FXValidateSummary {
	period := result.Result.Period.Format("2006-01")
	gaps := make([]FXValidationGap, 0, len(result.Result.Gaps))
	for _, gap := range result.Result.Gaps {
		methods := make([]string, len(gap.Methods))
		for i, method := range gap.Methods {
			methods[i] = string(method)
		}
		sort.Strings(methods)
		for _, method := range methods {
			gaps = append(gaps, FXValidationGap{Pair: gap.Pair, Period: period, Method: method})
		}
	}
	sort.Slice(gaps, func(i, j int) bool {
		if gaps[i].Pair == gaps[j].Pair {
			if gaps[i].Period == gaps[j].Period {
				return gaps[i].Method < gaps[j].Method
			}
			return gaps[i].Period < gaps[j].Period
		}
		return gaps[i].Pair < gaps[j].Pair
	})
	available := make([]FXValidationAvailability, 0, len(result.Result.Available)*2)
	for pair, quote := range result.Result.Available {
		if quote.Average > 0 {
			available = append(available, FXValidationAvailability{Pair: pair, Period: period, Method: string(fx.MethodAverage)})
		}
		if quote.Closing > 0 {
			available = append(available, FXValidationAvailability{Pair: pair, Period: period, Method: string(fx.MethodClosing)})
		}
	}
	sort.Slice(available, func(i, j int) bool {
		if available[i].Pair == available[j].Pair {
			if available[i].Period == available[j].Period {
				return available[i].Method < available[j].Method
			}
			return available[i].Period < available[j].Period
		}
		return available[i].Pair < available[j].Pair
	})
	return FXValidateSummary{
		OK:              len(gaps) == 0,
		Gaps:            gaps,
		AvailableQuotes: available,
	}
}

func renderValidateHuman(out io.Writer, result ValidateResult) {
	period := result.Result.Period.Format("2006-01")
	_, _ = fmt.Fprintf(out, "FX validation for group %d (%s) — period %s\n", result.GroupID, result.ReportingCurrency, period)
	if len(result.Result.Gaps) == 0 {
		_, _ = fmt.Fprintln(out, "All required FX rates are present.")
	} else {
		_, _ = fmt.Fprintf(out, "%d gap(s) detected:\n", len(result.Result.Gaps))
		for _, gap := range result.Result.Gaps {
			missing := make([]string, len(gap.Methods))
			for i, method := range gap.Methods {
				missing[i] = string(method)
			}
			sort.Strings(missing)
			_, _ = fmt.Fprintf(out, " - %s missing %s\n", gap.Pair, strings.Join(missing, ", "))
		}
	}
	if len(result.ConsideredPairs) > 0 {
		_, _ = fmt.Fprintln(out, "Checked pairs:")
		for _, pair := range result.ConsideredPairs {
			quote, ok := result.Result.Available[pair]
			if !ok {
				_, _ = fmt.Fprintf(out, " - %s (missing)\n", pair)
				continue
			}
			methods := make([]string, 0, 2)
			if quote.Average > 0 {
				methods = append(methods, string(fx.MethodAverage))
			}
			if quote.Closing > 0 {
				methods = append(methods, string(fx.MethodClosing))
			}
			_, _ = fmt.Fprintf(out, " - %s (%s)\n", pair, strings.Join(methods, ", "))
		}
	}
	if len(result.RequestedPairNames) > 0 {
		_, _ = fmt.Fprintf(out, "Requested pairs: %s\n", strings.Join(result.RequestedPairNames, ", "))
	}
}
