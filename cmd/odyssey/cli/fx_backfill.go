package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/odyssey-erp/odyssey-erp/internal/consol"
	"github.com/odyssey-erp/odyssey-erp/internal/consol/fx"
)

// FXBackfillMode enumerates supported execution strategies.
type FXBackfillMode string

const (
	// FXBackfillModeDry previews gaps without applying changes.
	FXBackfillModeDry FXBackfillMode = "dry"
	// FXBackfillModeApply persists rates after confirmation.
	FXBackfillModeApply FXBackfillMode = "apply"
)

// FXBackfillOptions configures the backfill command execution.
type FXBackfillOptions struct {
	Pair         string
	From         string
	To           string
	Mode         FXBackfillMode
	Source       string
	SourceReader io.Reader
	JSONOutput   bool
	Stdout       io.Writer
	Stderr       io.Writer
	Stdin        io.Reader
	Confirm      func(io.Reader, io.Writer) (bool, error)
}

// FXBackfillSummary captures the structured reporting outcome.
type FXBackfillSummary struct {
	Pair       string                `json:"pair"`
	Mode       FXBackfillMode        `json:"mode"`
	From       string                `json:"from"`
	To         string                `json:"to"`
	Missing    []FXBackfillGap       `json:"missing"`
	Candidates []FXBackfillCandidate `json:"candidates"`
	Applied    []FXBackfillCandidate `json:"applied,omitempty"`
}

// FXBackfillGap describes a missing FX method for a period.
type FXBackfillGap struct {
	Period  string   `json:"period"`
	Missing []string `json:"missing_methods"`
}

// FXBackfillCandidate summarises a rate sourced from CSV/stdin.
type FXBackfillCandidate struct {
	Period  string  `json:"period"`
	Average float64 `json:"average"`
	Closing float64 `json:"closing"`
}

// BackfillCommand executes the fx backfill workflow.
func (c *FXOpsCLI) BackfillCommand(ctx context.Context, opts FXBackfillOptions) int {
	if opts.Stdout == nil {
		opts.Stdout = os.Stdout
	}
	if opts.Stderr == nil {
		opts.Stderr = os.Stderr
	}
	if opts.Stdin == nil {
		opts.Stdin = os.Stdin
	}
	if opts.Mode == "" {
		opts.Mode = FXBackfillModeDry
	}
	mode := FXBackfillMode(strings.ToLower(string(opts.Mode)))
	switch mode {
	case FXBackfillModeDry, FXBackfillModeApply:
	default:
		fmt.Fprintf(opts.Stderr, "fx backfill: invalid mode %q (expected dry or apply)\n", opts.Mode)
		return 1
	}
	pair := strings.ToUpper(strings.TrimSpace(opts.Pair))
	if pair == "" {
		fmt.Fprintln(opts.Stderr, "fx backfill: --pair is required")
		return 1
	}
	from, err := time.Parse("2006-01", strings.TrimSpace(opts.From))
	if err != nil {
		fmt.Fprintf(opts.Stderr, "fx backfill: invalid --from %q (expected YYYY-MM)\n", opts.From)
		return 1
	}
	to, err := time.Parse("2006-01", strings.TrimSpace(opts.To))
	if err != nil {
		fmt.Fprintf(opts.Stderr, "fx backfill: invalid --to %q (expected YYYY-MM)\n", opts.To)
		return 1
	}
	if from.After(to) {
		fmt.Fprintln(opts.Stderr, "fx backfill: --from must be earlier than --to")
		return 1
	}
	periods := enumeratePeriods(from, to)
	provider := repoQuoteProvider{repo: c.repo}
	gaps := make([]FXBackfillGap, 0)
	for _, period := range periods {
		res, err := fx.Validate(ctx, provider, period, []fx.Requirement{{Pair: pair, Methods: []fx.Method{fx.MethodAverage, fx.MethodClosing}}})
		if err != nil {
			fmt.Fprintf(opts.Stderr, "fx backfill: validate %s: %v\n", period.Format("2006-01"), err)
			return 1
		}
		for _, gap := range res.Gaps {
			if gap.Pair != pair {
				continue
			}
			missing := make([]string, len(gap.Methods))
			for i, method := range gap.Methods {
				missing[i] = string(method)
			}
			sort.Strings(missing)
			gaps = append(gaps, FXBackfillGap{Period: period.Format("2006-01"), Missing: missing})
		}
	}
	candidates, err := loadBackfillCandidates(pair, opts)
	if err != nil {
		fmt.Fprintf(opts.Stderr, "fx backfill: %v\n", err)
		return 1
	}
	summary := FXBackfillSummary{
		Pair:    pair,
		Mode:    mode,
		From:    from.Format("2006-01"),
		To:      to.Format("2006-01"),
		Missing: gaps,
	}
	summary.Candidates = filterCandidates(gaps, candidates)
	if mode == FXBackfillModeDry {
		if err := writeBackfillOutput(opts, summary); err != nil {
			fmt.Fprintf(opts.Stderr, "fx backfill: %v\n", err)
			return 1
		}
		if len(gaps) > 0 {
			return 10
		}
		return 0
	}
	if len(gaps) == 0 {
		if err := writeBackfillOutput(opts, summary); err != nil {
			fmt.Fprintf(opts.Stderr, "fx backfill: %v\n", err)
			return 1
		}
		return 0
	}
	rows, err := prepareUpserts(pair, summary.Candidates, gaps)
	if err != nil {
		fmt.Fprintf(opts.Stderr, "fx backfill: %v\n", err)
		return 1
	}
	confirm := opts.Confirm
	if confirm == nil {
		confirm = defaultBackfillConfirm
	}
	ok, err := confirm(opts.Stdin, opts.Stdout)
	if err != nil {
		fmt.Fprintf(opts.Stderr, "fx backfill: confirmation failed: %v\n", err)
		return 1
	}
	if !ok {
		fmt.Fprintln(opts.Stderr, "fx backfill: cancelled by user")
		return 1
	}
	if err := c.repo.UpsertFxRates(ctx, rows); err != nil {
		fmt.Fprintf(opts.Stderr, "fx backfill: apply failed: %v\n", err)
		return 1
	}
	applied := make([]FXBackfillCandidate, len(rows))
	for i, row := range rows {
		applied[i] = FXBackfillCandidate{
			Period:  row.AsOf.Format("2006-01"),
			Average: row.Average,
			Closing: row.Closing,
		}
	}
	sort.Slice(applied, func(i, j int) bool { return applied[i].Period < applied[j].Period })
	summary.Applied = applied
	if err := writeBackfillOutput(opts, summary); err != nil {
		fmt.Fprintf(opts.Stderr, "fx backfill: %v\n", err)
		return 1
	}
	return 0
}

func enumeratePeriods(from, to time.Time) []time.Time {
	start := time.Date(from.Year(), from.Month(), 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(to.Year(), to.Month(), 1, 0, 0, 0, 0, time.UTC)
	var periods []time.Time
	for current := start; !current.After(end); current = current.AddDate(0, 1, 0) {
		periods = append(periods, current)
	}
	return periods
}

func loadBackfillCandidates(pair string, opts FXBackfillOptions) (map[string]FXBackfillCandidate, error) {
	var data []byte
	var err error
	switch {
	case opts.SourceReader != nil:
		data, err = io.ReadAll(opts.SourceReader)
	case opts.Source == "-":
		if opts.Stdin == nil {
			return nil, errors.New("source - requires stdin")
		}
		data, err = io.ReadAll(opts.Stdin)
	case strings.TrimSpace(opts.Source) == "":
		return map[string]FXBackfillCandidate{}, nil
	default:
		data, err = os.ReadFile(opts.Source)
	}
	if err != nil {
		return nil, err
	}
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return map[string]FXBackfillCandidate{}, nil
	}
	reader := csv.NewReader(bytes.NewReader(data))
	reader.TrimLeadingSpace = true
	header, err := nextNonEmptyRecord(reader)
	if err != nil {
		if errors.Is(err, io.EOF) {
			return map[string]FXBackfillCandidate{}, nil
		}
		return nil, err
	}
	indexes := map[string]int{"period": -1, "pair": -1, "average": -1, "closing": -1}
	for i, col := range header {
		switch strings.ToLower(strings.TrimSpace(col)) {
		case "period":
			indexes["period"] = i
		case "pair":
			indexes["pair"] = i
		case "average", "average_rate":
			indexes["average"] = i
		case "closing", "closing_rate":
			indexes["closing"] = i
		}
	}
	if indexes["period"] < 0 || indexes["pair"] < 0 || indexes["average"] < 0 || indexes["closing"] < 0 {
		return nil, fmt.Errorf("missing required columns in source (need period, pair, average, closing)")
	}
	result := make(map[string]FXBackfillCandidate)
	for {
		record, err := nextNonEmptyRecord(reader)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}
		if indexes["period"] >= len(record) || indexes["pair"] >= len(record) || indexes["average"] >= len(record) || indexes["closing"] >= len(record) {
			return nil, fmt.Errorf("invalid record length in source")
		}
		periodStr := strings.TrimSpace(record[indexes["period"]])
		if periodStr == "" {
			continue
		}
		asOf, err := time.Parse("2006-01", periodStr)
		if err != nil {
			return nil, fmt.Errorf("invalid period %q in source", periodStr)
		}
		periodKey := asOf.Format("2006-01")
		sourcePair := strings.ToUpper(strings.TrimSpace(record[indexes["pair"]]))
		if sourcePair == "" || sourcePair != pair {
			continue
		}
		avg, err := strconv.ParseFloat(strings.TrimSpace(record[indexes["average"]]), 64)
		if err != nil {
			return nil, fmt.Errorf("invalid average for %s: %v", periodKey, err)
		}
		closing, err := strconv.ParseFloat(strings.TrimSpace(record[indexes["closing"]]), 64)
		if err != nil {
			return nil, fmt.Errorf("invalid closing for %s: %v", periodKey, err)
		}
		result[periodKey] = FXBackfillCandidate{Period: periodKey, Average: avg, Closing: closing}
	}
	return result, nil
}

func nextNonEmptyRecord(r *csv.Reader) ([]string, error) {
	for {
		record, err := r.Read()
		if err != nil {
			return nil, err
		}
		if len(record) == 0 {
			continue
		}
		skip := true
		for _, field := range record {
			trimmed := strings.TrimSpace(field)
			if trimmed == "" {
				continue
			}
			if strings.HasPrefix(trimmed, "#") {
				continue
			}
			skip = false
		}
		if skip {
			continue
		}
		return record, nil
	}
}

func filterCandidates(gaps []FXBackfillGap, candidates map[string]FXBackfillCandidate) []FXBackfillCandidate {
	rows := make([]FXBackfillCandidate, 0, len(candidates))
	seen := make(map[string]struct{}, len(gaps))
	for _, gap := range gaps {
		if candidate, ok := candidates[gap.Period]; ok {
			rows = append(rows, candidate)
			seen[gap.Period] = struct{}{}
		}
	}
	for period, candidate := range candidates {
		if _, ok := seen[period]; ok {
			continue
		}
		rows = append(rows, candidate)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Period < rows[j].Period })
	return rows
}

func prepareUpserts(pair string, candidates []FXBackfillCandidate, gaps []FXBackfillGap) ([]consol.FxRateInput, error) {
	if len(gaps) == 0 {
		return nil, nil
	}
	lookup := make(map[string]FXBackfillCandidate, len(candidates))
	for _, candidate := range candidates {
		lookup[candidate.Period] = candidate
	}
	rows := make([]consol.FxRateInput, 0, len(gaps))
	for _, gap := range gaps {
		candidate, ok := lookup[gap.Period]
		if !ok {
			return nil, fmt.Errorf("missing source rates for %s", gap.Period)
		}
		if candidate.Average <= 0 || candidate.Closing <= 0 {
			return nil, fmt.Errorf("non-positive rates for %s", gap.Period)
		}
		asOf, err := time.Parse("2006-01", gap.Period)
		if err != nil {
			return nil, err
		}
		rows = append(rows, consol.FxRateInput{
			AsOf:    asOf,
			Pair:    pair,
			Average: candidate.Average,
			Closing: candidate.Closing,
		})
	}
	return rows, nil
}

func writeBackfillOutput(opts FXBackfillOptions, summary FXBackfillSummary) error {
	if opts.JSONOutput {
		return json.NewEncoder(opts.Stdout).Encode(summary)
	}
	renderBackfillHuman(opts.Stdout, summary)
	return nil
}

func renderBackfillHuman(out io.Writer, summary FXBackfillSummary) {
	fmt.Fprintf(out, "FX backfill (%s) for %s — %s to %s\n", summary.Mode, summary.Pair, summary.From, summary.To)
	if len(summary.Missing) == 0 {
		fmt.Fprintln(out, "No gaps detected.")
	} else {
		fmt.Fprintf(out, "%d gap(s) detected:\n", len(summary.Missing))
		for _, gap := range summary.Missing {
			fmt.Fprintf(out, " - %s missing %s\n", gap.Period, strings.Join(gap.Missing, ", "))
		}
	}
	if len(summary.Candidates) > 0 {
		fmt.Fprintln(out, "Source candidates:")
		for _, candidate := range summary.Candidates {
			fmt.Fprintf(out, " - %s average %.6f closing %.6f\n", candidate.Period, candidate.Average, candidate.Closing)
		}
	}
	if len(summary.Applied) > 0 {
		fmt.Fprintln(out, "Applied:")
		for _, row := range summary.Applied {
			fmt.Fprintf(out, " - %s average %.6f closing %.6f\n", row.Period, row.Average, row.Closing)
		}
	}
}

func defaultBackfillConfirm(r io.Reader, w io.Writer) (bool, error) {
	fmt.Fprint(w, "Apply FX backfill? Type YES to confirm: ")
	reader := bufio.NewReader(r)
	line, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, err
	}
	response := strings.TrimSpace(line)
	return strings.EqualFold(response, "YES"), nil
}
