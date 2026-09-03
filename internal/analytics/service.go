package analytics

import (
	"context"
	"fmt"
	"strconv"
	"time"
)

// Repository exposes analytics operations in domain terms. SQLC types and
// PostgreSQL encodings are translated by the concrete repository adapter.
type Repository interface {
	KpiSummary(ctx context.Context, filter KPIFilter) (KPISummary, error)
	MonthlyPL(ctx context.Context, filter TrendFilter) ([]MonthlyPLRow, error)
	MonthlyCashflow(ctx context.Context, filter TrendFilter) ([]MonthlyCashflowRow, error)
	AgingAR(ctx context.Context, filter AgingFilter) ([]AgingRow, error)
	AgingAP(ctx context.Context, filter AgingFilter) ([]AgingRow, error)
}

// Service coordinates analytics query execution with the cache layer.
type Service struct {
	repo  Repository
	cache *Cache
}

// NewService wires a Repository with a Cache helper.
func NewService(repo Repository, cache *Cache) *Service {
	return &Service{repo: repo, cache: cache}
}

func branchToken(branchID *int64) string {
	if branchID == nil {
		return "-"
	}
	return formatInt(*branchID)
}

func formatInt(v int64) string {
	return strconv.FormatInt(v, 10)
}

// GenerateBIExport pulls curated data (e.g., KPIs) and returns it as a CSV string,
// which can be passed to the connector outbox for uploading to external BI tools.
func (s *Service) GenerateBIExport(ctx context.Context, companyID int64, period string, branchID *int64) (string, error) {
	summary, err := s.GetKPISummary(ctx, KPIFilter{
		CompanyID: companyID,
		Period:    period,
		BranchID:  branchID,
		AsOf:      time.Now(),
	})
	if err != nil {
		return "", err
	}

	// Temporarily import local export package to generate CSV.
	// Since we can't easily do cyclical imports if export imports analytics,
	// let's just generate the CSV manually here or assume the caller handles serialization.
	// For now we'll format the summary manually to avoid cyclic import since export depends on analytics.

	var buf string
	buf += "Metric,Value\n"
	buf += "Period," + period + "\n"
	buf += "Net Profit," + fmt.Sprintf("%.2f", summary.NetProfit) + "\n"
	buf += "Revenue," + fmt.Sprintf("%.2f", summary.Revenue) + "\n"
	buf += "Operating Expense," + fmt.Sprintf("%.2f", summary.Opex) + "\n"
	buf += "Cost of Goods Sold," + fmt.Sprintf("%.2f", summary.COGS) + "\n"
	buf += "Cash In," + fmt.Sprintf("%.2f", summary.CashIn) + "\n"
	buf += "Cash Out," + fmt.Sprintf("%.2f", summary.CashOut) + "\n"
	buf += "AR Outstanding," + fmt.Sprintf("%.2f", summary.AROutstanding) + "\n"
	buf += "AP Outstanding," + fmt.Sprintf("%.2f", summary.APOutstanding) + "\n"

	return buf, nil
}
