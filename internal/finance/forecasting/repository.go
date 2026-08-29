package forecasting

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/odyssey-erp/odyssey-erp/internal/finance/automation"
	"github.com/odyssey-erp/odyssey-erp/internal/sqlc"
)

type PGRepository struct {
	db      *pgxpool.Pool
	queries *sqlc.Queries
}

func NewPGRepository(db *pgxpool.Pool) *PGRepository {
	return &PGRepository{
		db:      db,
		queries: sqlc.New(db),
	}
}

func (r *PGRepository) ScenarioBelongsToCompany(ctx context.Context, scenarioID, companyID int64) (bool, error) {
	return r.queries.ForecastScenarioBelongsToCompany(ctx, sqlc.ForecastScenarioBelongsToCompanyParams{
		ID:        scenarioID,
		CompanyID: companyID,
	})
}

func (r *PGRepository) CompanyBaseCurrency(ctx context.Context, companyID int64) (string, error) {
	var currency string
	if err := r.db.QueryRow(ctx, `SELECT base_currency FROM companies WHERE id = $1`, companyID).Scan(&currency); err != nil {
		return "", err
	}
	return currency, nil
}

func (r *PGRepository) CreateForecastRun(ctx context.Context, input CreateForecastRunInput) (ForecastRun, error) {
	row, err := r.queries.CreateForecastRun(ctx, sqlc.CreateForecastRunParams{
		CompanyID:  input.CompanyID,
		ScenarioID: input.ScenarioID,
		Status:     input.Status,
		FxSnapshot: input.FxSnapshot,
	})
	return mapForecastRun(row), err
}

func (r *PGRepository) UpdateForecastRunStatus(ctx context.Context, update ForecastRunStatusUpdate) error {
	return r.queries.UpdateForecastRunStatus(ctx, sqlc.UpdateForecastRunStatusParams{
		ID:           update.ID,
		Status:       update.Status,
		CompletedAt:  nullableTime(update.CompletedAt),
		ErrorDetails: nullableText(update.ErrorDetails),
		FxSnapshot:   update.FxSnapshot,
	})
}

func (r *PGRepository) CreateForecastDailyBucket(ctx context.Context, input CreateForecastDailyBucketInput) (ForecastDailyBucket, error) {
	row, err := r.queries.CreateForecastDailyBucket(ctx, sqlc.CreateForecastDailyBucketParams{
		RunID:          input.RunID,
		BankAccountID:  pgtype.Int8{},
		Currency:       input.Currency,
		BucketDate:     nullableDate(input.BucketDate),
		OpeningBalance: exactAmountNumeric(input.OpeningBalance),
		TotalInflow:    exactAmountNumeric(input.TotalInflow),
		TotalOutflow:   exactAmountNumeric(input.TotalOutflow),
		ClosingBalance: exactAmountNumeric(input.ClosingBalance),
	})
	return mapForecastBucket(row), err
}

func (r *PGRepository) CreateForecastSourceLine(ctx context.Context, input CreateForecastSourceLineInput) (int64, error) {
	row, err := r.queries.CreateForecastSourceLine(ctx, sqlc.CreateForecastSourceLineParams{
		RunID:         input.RunID,
		DailyBucketID: input.DailyBucketID,
		SourceType:    input.SourceType,
		SourceRef:     input.SourceRef,
		Amount:        exactAmountNumeric(input.Amount),
		Currency:      input.Currency,
		ExpectedDate:  nullableDate(input.ExpectedDate),
		Certainty:     input.Certainty,
	})
	return row.ID, err
}

func exactAmountNumeric(value automation.ExactAmount) pgtype.Numeric {
	var numeric pgtype.Numeric
	_ = numeric.Scan(value.Amount.String())
	return numeric
}

func (r *PGRepository) GetLatestForecastRun(ctx context.Context, query ForecastRunQuery) (ForecastRun, error) {
	row, err := r.queries.GetLatestForecastRun(ctx, sqlc.GetLatestForecastRunParams{
		CompanyID:  query.CompanyID,
		ScenarioID: query.ScenarioID,
	})
	return mapForecastRun(row), err
}

func (r *PGRepository) ListForecastDailyBucketsByRun(ctx context.Context, runID int64) ([]ForecastDailyBucket, error) {
	rows, err := r.queries.ListForecastDailyBucketsByRun(ctx, runID)
	if err != nil {
		return nil, err
	}
	result := make([]ForecastDailyBucket, 0, len(rows))
	for _, row := range rows {
		result = append(result, mapForecastBucket(row))
	}
	return result, nil
}

// ListForecastSourceLinesByRun returns exact source identities for a scoped
// run. The company predicate is repeated here rather than relying only on the
// handler's latest-run lookup so callers cannot use a guessed run ID to cross a
// tenant boundary.
func (r *PGRepository) ListForecastSourceLinesByRun(ctx context.Context, companyID, runID int64) ([]ForecastSourceLine, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("forecast repository database is not configured")
	}
	if companyID <= 0 || runID <= 0 {
		return nil, fmt.Errorf("forecast company and run IDs are required")
	}
	rows, err := r.db.Query(ctx, `
		SELECT l.id,
		       l.run_id,
		       l.daily_bucket_id,
		       l.source_type,
		       l.source_ref,
		       l.amount::text,
		       l.currency,
		       l.expected_date,
		       l.certainty
		FROM forecast_source_lines l
		JOIN forecast_runs run ON run.id = l.run_id
		WHERE run.company_id = $1
		  AND l.run_id = $2
		ORDER BY l.expected_date, l.id`, companyID, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []ForecastSourceLine
	for rows.Next() {
		var line ForecastSourceLine
		if err := rows.Scan(
			&line.ID,
			&line.RunID,
			&line.DailyBucketID,
			&line.SourceType,
			&line.SourceRef,
			&line.Amount,
			&line.Currency,
			&line.ExpectedDate,
			&line.Certainty,
		); err != nil {
			return nil, err
		}
		line.SourceType = strings.ToUpper(strings.TrimSpace(line.SourceType))
		line.SourceRef = strings.TrimSpace(line.SourceRef)
		line.Currency = strings.ToUpper(strings.TrimSpace(line.Currency))
		line.Certainty = strings.ToUpper(strings.TrimSpace(line.Certainty))
		line.ExpectedDate = dateOnlyUTC(line.ExpectedDate)
		result = append(result, line)
	}
	return result, rows.Err()
}

func mapForecastRun(row sqlc.ForecastRun) ForecastRun {
	return ForecastRun{
		ID:           row.ID,
		CompanyID:    row.CompanyID,
		ScenarioID:   row.ScenarioID,
		Status:       row.Status,
		FxSnapshot:   row.FxSnapshot,
		CompletedAt:  validTime(row.CompletedAt),
		ErrorDetails: validText(row.ErrorDetails),
	}
}

func mapForecastBucket(row sqlc.ForecastDailyBucket) ForecastDailyBucket {
	return ForecastDailyBucket{
		ID:             row.ID,
		RunID:          row.RunID,
		Currency:       row.Currency,
		BucketDate:     validDate(row.BucketDate),
		OpeningBalance: numericFloat(row.OpeningBalance),
		TotalInflow:    numericFloat(row.TotalInflow),
		TotalOutflow:   numericFloat(row.TotalOutflow),
		ClosingBalance: numericFloat(row.ClosingBalance),
	}
}

func nullableTime(value time.Time) pgtype.Timestamptz {
	if value.IsZero() {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: value, Valid: true}
}

func nullableDate(value time.Time) pgtype.Date {
	if value.IsZero() {
		return pgtype.Date{}
	}
	return pgtype.Date{Time: value, Valid: true}
}

func nullableText(value string) pgtype.Text {
	if value == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: value, Valid: true}
}

func validTime(value pgtype.Timestamptz) time.Time {
	if !value.Valid {
		return time.Time{}
	}
	return value.Time
}

func validDate(value pgtype.Date) time.Time {
	if !value.Valid {
		return time.Time{}
	}
	return value.Time
}

func validText(value pgtype.Text) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

func numericFloat(value pgtype.Numeric) float64 {
	result, _ := value.Float64Value()
	return result.Float64
}
