package analytics

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/odyssey-erp/odyssey-erp/internal/sqlc"
)

// PGRepository adapts generated PostgreSQL queries to analytics' domain
// repository contract. SQLC and pgtype remain private to this file.
type PGRepository struct {
	queries *sqlc.Queries
}

func NewRepository(pool *pgxpool.Pool) *PGRepository {
	return &PGRepository{queries: sqlc.New(pool)}
}

func (r *PGRepository) KpiSummary(ctx context.Context, filter KPIFilter) (KPISummary, error) {
	row, err := r.queries.KpiSummary(ctx, sqlc.KpiSummaryParams{
		Period:    filter.Period,
		CompanyID: filter.CompanyID,
		BranchID:  optionalBranch(filter.BranchID),
		AsOf:      dateParam(filter.AsOf),
	})
	if err != nil {
		return KPISummary{}, err
	}
	return KPISummary{
		NetProfit:     toFloat64(row.NetProfit),
		Revenue:       toFloat64(row.Revenue),
		Opex:          toFloat64(row.Opex),
		COGS:          toFloat64(row.Cogs),
		CashIn:        toFloat64(row.CashIn),
		CashOut:       toFloat64(row.CashOut),
		AROutstanding: toFloat64(row.ArOutstanding),
		APOutstanding: toFloat64(row.ApOutstanding),
	}, nil
}

func (r *PGRepository) MonthlyPL(ctx context.Context, filter TrendFilter) ([]MonthlyPLRow, error) {
	rows, err := r.queries.MonthlyPL(ctx, sqlc.MonthlyPLParams{
		FromPeriod: filter.From,
		ToPeriod:   filter.To,
		CompanyID:  filter.CompanyID,
		BranchID:   optionalBranch(filter.BranchID),
	})
	if err != nil {
		return nil, err
	}
	result := make([]MonthlyPLRow, 0, len(rows))
	for _, row := range rows {
		result = append(result, MonthlyPLRow{
			Period:  row.Period,
			Revenue: row.Revenue,
			COGS:    row.Cogs,
			Opex:    row.Opex,
			Net:     row.Net,
		})
	}
	return result, nil
}

func (r *PGRepository) MonthlyCashflow(ctx context.Context, filter TrendFilter) ([]MonthlyCashflowRow, error) {
	rows, err := r.queries.MonthlyCashflow(ctx, sqlc.MonthlyCashflowParams{
		FromPeriod: filter.From,
		ToPeriod:   filter.To,
		CompanyID:  filter.CompanyID,
		BranchID:   optionalBranch(filter.BranchID),
	})
	if err != nil {
		return nil, err
	}
	result := make([]MonthlyCashflowRow, 0, len(rows))
	for _, row := range rows {
		result = append(result, MonthlyCashflowRow{Period: row.Period, In: row.CashIn, Out: row.CashOut})
	}
	return result, nil
}

func (r *PGRepository) AgingAR(ctx context.Context, filter AgingFilter) ([]AgingRow, error) {
	rows, err := r.queries.AgingAR(ctx, sqlc.AgingARParams{
		AsOf:      dateParam(filter.AsOf),
		CompanyID: filter.CompanyID,
		BranchID:  optionalBranch(filter.BranchID),
	})
	if err != nil {
		return nil, err
	}
	result := make([]AgingRow, 0, len(rows))
	for _, row := range rows {
		result = append(result, AgingRow{Bucket: row.Bucket, Amount: row.Amount})
	}
	return result, nil
}

func (r *PGRepository) AgingAP(ctx context.Context, filter AgingFilter) ([]AgingRow, error) {
	rows, err := r.queries.AgingAP(ctx, sqlc.AgingAPParams{
		AsOf:      dateParam(filter.AsOf),
		CompanyID: filter.CompanyID,
		BranchID:  optionalBranch(filter.BranchID),
	})
	if err != nil {
		return nil, err
	}
	result := make([]AgingRow, 0, len(rows))
	for _, row := range rows {
		result = append(result, AgingRow{Bucket: row.Bucket, Amount: row.Amount})
	}
	return result, nil
}

func optionalBranch(branchID *int64) pgtype.Int8 {
	if branchID == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *branchID, Valid: true}
}

func dateParam(t time.Time) pgtype.Date {
	if t.IsZero() {
		return pgtype.Date{}
	}
	return pgtype.Date{Time: t, Valid: true}
}

func toFloat64(v interface{}) float64 {
	switch val := v.(type) {
	case nil:
		return 0
	case float32:
		return float64(val)
	case float64:
		return val
	case int64:
		return float64(val)
	case int32:
		return float64(val)
	case uint64:
		return float64(val)
	case uint32:
		return float64(val)
	case int:
		return float64(val)
	case uint:
		return float64(val)
	default:
		return 0
	}
}
