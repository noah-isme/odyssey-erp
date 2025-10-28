package analytics

import (
	"context"

	"github.com/odyssey-erp/odyssey-erp/internal/analytics/db"
)

// CashflowTrendPoint captures monthly cash inflow and outflow.
type CashflowTrendPoint struct {
	Period string
	In     float64
	Out    float64
}

// GetCashflowTrend computes cash movement per month across the selected range.
func (s *Service) GetCashflowTrend(ctx context.Context, filter TrendFilter) ([]CashflowTrendPoint, error) {
	loader := func(ctx context.Context) (interface{}, error) {
		rows, err := s.repo.MonthlyCashflow(ctx, analyticsdb.MonthlyCashflowParams{
			FromPeriod: filter.From,
			ToPeriod:   filter.To,
			CompanyID:  filter.CompanyID,
			BranchID:   optionalBranch(filter.BranchID),
		})
		if err != nil {
			return nil, err
		}
		points := make([]CashflowTrendPoint, 0, len(rows))
		for _, row := range rows {
			points = append(points, CashflowTrendPoint{
				Period: row.Period,
				In:     row.CashIn,
				Out:    row.CashOut,
			})
		}
		return points, nil
	}

	if s.cache == nil {
		value, err := loader(ctx)
		if err != nil {
			return nil, err
		}
		return value.([]CashflowTrendPoint), nil
	}

	keyBase := keyCashflow(filter.CompanyID, filter.BranchID, filter.From, filter.To)
	key, err := s.cache.BuildKey(ctx, keyBase)
	if err != nil {
		return nil, err
	}
	var points []CashflowTrendPoint
	if err := s.cache.FetchJSON(ctx, key, &points, loader); err != nil {
		return nil, err
	}
	return points, nil
}
