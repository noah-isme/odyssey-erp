package insights

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/odyssey-erp/odyssey-erp/internal/sqlc"
)

// PGRepository adapts SQLC queries to the insights repository contract.
type PGRepository struct {
	queries *sqlc.Queries
}

func NewRepository(pool *pgxpool.Pool) *PGRepository {
	return &PGRepository{queries: sqlc.New(pool)}
}

func (r *PGRepository) CompareMonthlyNetRevenue(ctx context.Context, query RevenueQuery) ([]MonthlyNetRevenueRow, error) {
	rows, err := r.queries.CompareMonthlyNetRevenue(ctx, sqlc.CompareMonthlyNetRevenueParams{
		FromPeriod: query.FromPeriod,
		ToPeriod:   query.ToPeriod,
		CompanyID:  query.CompanyID,
		BranchID:   optionalBranch(query.BranchID),
	})
	if err != nil {
		return nil, err
	}
	result := make([]MonthlyNetRevenueRow, 0, len(rows))
	for _, row := range rows {
		result = append(result, MonthlyNetRevenueRow{Period: row.Period, Net: row.Net, Revenue: row.Revenue})
	}
	return result, nil
}

func (r *PGRepository) ContributionByBranch(ctx context.Context, query ContributionQuery) ([]BranchContributionRow, error) {
	rows, err := r.queries.ContributionByBranch(ctx, sqlc.ContributionByBranchParams{
		Period:    query.Period,
		CompanyID: query.CompanyID,
	})
	if err != nil {
		return nil, err
	}
	result := make([]BranchContributionRow, 0, len(rows))
	for _, row := range rows {
		result = append(result, BranchContributionRow{BranchID: row.BranchID, Net: row.Net, Revenue: row.Revenue})
	}
	return result, nil
}

func optionalBranch(value *int64) pgtype.Int8 {
	if value == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *value, Valid: true}
}
