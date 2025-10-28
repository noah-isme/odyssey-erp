package analytics

import (
	"context"
	"time"

	"github.com/odyssey-erp/odyssey-erp/internal/analytics/db"
)

// KPIFilter defines the scope for KPI aggregation.
type KPIFilter struct {
	Period    string
	CompanyID int64
	BranchID  *int64
	AsOf      time.Time
}

// KPISummary contains the key finance indicators surfaced on the dashboard.
type KPISummary struct {
	NetProfit     float64
	Revenue       float64
	Opex          float64
	COGS          float64
	CashIn        float64
	CashOut       float64
	AROutstanding float64
	APOutstanding float64
}

// GetKPISummary resolves the KPI card using cache-aware lookups.
func (s *Service) GetKPISummary(ctx context.Context, filter KPIFilter) (KPISummary, error) {
	loader := func(ctx context.Context) (interface{}, error) {
		row, err := s.repo.KpiSummary(ctx, analyticsdb.KpiSummaryParams{
			Period:    filter.Period,
			CompanyID: filter.CompanyID,
			BranchID:  optionalBranch(filter.BranchID),
			AsOf:      dateParam(filter.AsOf),
		})
		if err != nil {
			return KPISummary{}, err
		}
		summary := KPISummary{
			NetProfit:     toFloat64(row.NetProfit),
			Revenue:       toFloat64(row.Revenue),
			Opex:          toFloat64(row.Opex),
			COGS:          toFloat64(row.Cogs),
			CashIn:        toFloat64(row.CashIn),
			CashOut:       toFloat64(row.CashOut),
			AROutstanding: toFloat64(row.ArOutstanding),
			APOutstanding: toFloat64(row.ApOutstanding),
		}
		return summary, nil
	}

	if s.cache == nil {
		value, err := loader(ctx)
		if err != nil {
			return KPISummary{}, err
		}
		return value.(KPISummary), nil
	}

	keyBase := keyKPI(filter.CompanyID, filter.BranchID, filter.Period)
	key, err := s.cache.BuildKey(ctx, keyBase)
	if err != nil {
		return KPISummary{}, err
	}
	var summary KPISummary
	if err := s.cache.FetchJSON(ctx, key, &summary, loader); err != nil {
		return KPISummary{}, err
	}
	return summary, nil
}
