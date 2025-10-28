package analytics

import (
	"context"

	"github.com/odyssey-erp/odyssey-erp/internal/analytics/db"
)

// TrendFilter controls the date range for monthly trend queries.
type TrendFilter struct {
	From      string
	To        string
	CompanyID int64
	BranchID  *int64
}

// PLTrendPoint conveys revenue and expense movements.
type PLTrendPoint struct {
	Period  string
	Revenue float64
	COGS    float64
	Opex    float64
	Net     float64
}

// GetPLTrend returns aggregated monthly P&L movements respecting cache constraints.
func (s *Service) GetPLTrend(ctx context.Context, filter TrendFilter) ([]PLTrendPoint, error) {
	loader := func(ctx context.Context) (interface{}, error) {
		rows, err := s.repo.MonthlyPL(ctx, analyticsdb.MonthlyPLParams{
			FromPeriod: filter.From,
			ToPeriod:   filter.To,
			CompanyID:  filter.CompanyID,
			BranchID:   optionalBranch(filter.BranchID),
		})
		if err != nil {
			return nil, err
		}
		points := make([]PLTrendPoint, 0, len(rows))
		for _, row := range rows {
			points = append(points, PLTrendPoint{
				Period:  row.Period,
				Revenue: row.Revenue,
				COGS:    row.Cogs,
				Opex:    row.Opex,
				Net:     row.Net,
			})
		}
		return points, nil
	}

	if s.cache == nil {
		value, err := loader(ctx)
		if err != nil {
			return nil, err
		}
		return value.([]PLTrendPoint), nil
	}

	keyBase := keyPLTrend(filter.CompanyID, filter.BranchID, filter.From, filter.To)
	key, err := s.cache.BuildKey(ctx, keyBase)
	if err != nil {
		return nil, err
	}
	var points []PLTrendPoint
	if err := s.cache.FetchJSON(ctx, key, &points, loader); err != nil {
		return nil, err
	}
	return points, nil
}
