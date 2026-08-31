package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

type TransactionFXOperations interface {
	Fetch(context.Context, time.Time, bool) error
	Status(context.Context, time.Time) ([]FXRateStatus, error)
}
type FXRateStatus struct {
	BaseCurrency, QuoteCurrency, Source, Status string
	RateDate                                    time.Time
	Rate                                        string
}
type TransactionFXCommandOptions struct {
	Date           string
	Force          bool
	Stdout, Stderr io.Writer
}

func RunTransactionFXFetch(ctx context.Context, ops TransactionFXOperations, opts TransactionFXCommandOptions) int {
	if opts.Stdout == nil {
		opts.Stdout = os.Stdout
	}
	if opts.Stderr == nil {
		opts.Stderr = os.Stderr
	}
	date, err := parseFXDate(opts.Date)
	if err != nil {
		_, _ = fmt.Fprintln(opts.Stderr, err)
		return 1
	}
	if ops == nil {
		_, _ = fmt.Fprintln(opts.Stderr, "fx fetch: operations are not configured")
		return 1
	}
	if err := ops.Fetch(ctx, date, opts.Force); err != nil {
		_, _ = fmt.Fprintf(opts.Stderr, "fx fetch: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(opts.Stdout, "FX rates fetched for %s\n", date.Format("2006-01-02"))
	return 0
}
func RunTransactionFXStatus(ctx context.Context, ops TransactionFXOperations, opts TransactionFXCommandOptions) int {
	if opts.Stdout == nil {
		opts.Stdout = os.Stdout
	}
	if opts.Stderr == nil {
		opts.Stderr = os.Stderr
	}
	date, err := parseFXDate(opts.Date)
	if err != nil {
		_, _ = fmt.Fprintln(opts.Stderr, err)
		return 1
	}
	if ops == nil {
		_, _ = fmt.Fprintln(opts.Stderr, "fx status: operations are not configured")
		return 1
	}
	rows, err := ops.Status(ctx, date)
	if err != nil {
		_, _ = fmt.Fprintf(opts.Stderr, "fx status: %v\n", err)
		return 1
	}
	for _, row := range rows {
		_, _ = fmt.Fprintf(opts.Stdout, "%s/%s %s %s %s\n", row.BaseCurrency, row.QuoteCurrency, row.Rate, row.Source, row.Status)
	}
	return 0
}
func parseFXDate(value string) (time.Time, error) {
	if strings.TrimSpace(value) == "" {
		loc, _ := time.LoadLocation("Asia/Jakarta")
		return time.Now().In(loc), nil
	}
	d, err := time.Parse("2006-01-02", strings.TrimSpace(value))
	if err != nil {
		return time.Time{}, fmt.Errorf("fx: invalid --date %q (expected YYYY-MM-DD)", value)
	}
	return d, nil
}
