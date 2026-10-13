package payroll

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// ErrPeriodOverlap is returned when a new payroll period intersects an existing
// period of the same company.
var ErrPeriodOverlap = errors.New("payroll: period overlaps an existing period")

// CreatePeriodInput describes a new OPEN payroll period.
type CreatePeriodInput struct {
	CompanyID int64
	Code      string
	StartsOn  time.Time
	EndsOn    time.Time
	PayDate   time.Time
}

type txBeginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// CreatePeriod inserts an OPEN period after verifying, in the same transaction,
// that no period of the company overlaps it. There is deliberately no DB
// exclusion constraint so pre-existing data cannot break.
func (r *Repository) CreatePeriod(ctx context.Context, in CreatePeriodInput) (int64, error) {
	return createPeriod(ctx, r.pool, in)
}

func createPeriod(ctx context.Context, db txBeginner, in CreatePeriodInput) (int64, error) {
	if in.CompanyID <= 0 || strings.TrimSpace(in.Code) == "" {
		return 0, errors.New("payroll: company and period code are required")
	}
	if in.EndsOn.Before(in.StartsOn) {
		return 0, errors.New("payroll: period end precedes start")
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin create period: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var one int
	err = tx.QueryRow(ctx, `SELECT 1 FROM payroll_periods WHERE company_id=$1 AND starts_on<=$3 AND ends_on>=$2 LIMIT 1`,
		in.CompanyID, in.StartsOn, in.EndsOn).Scan(&one)
	if err == nil {
		return 0, ErrPeriodOverlap
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return 0, fmt.Errorf("check period overlap: %w", err)
	}
	var id int64
	if err = tx.QueryRow(ctx, `INSERT INTO payroll_periods(company_id,code,starts_on,ends_on,pay_date) VALUES($1,$2,$3,$4,$5) RETURNING id`,
		in.CompanyID, strings.TrimSpace(in.Code), in.StartsOn, in.EndsOn, in.PayDate).Scan(&id); err != nil {
		return 0, fmt.Errorf("insert period: %w", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit create period: %w", err)
	}
	return id, nil
}
