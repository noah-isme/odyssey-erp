package analytics

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/odyssey-erp/odyssey-erp/internal/sqlc"
)

// Repository exposes the generated sqlc queries we rely on.
type Repository interface {
	KpiSummary(ctx context.Context, arg sqlc.KpiSummaryParams) (sqlc.KpiSummaryRow, error)
	MonthlyPL(ctx context.Context, arg sqlc.MonthlyPLParams) ([]sqlc.MonthlyPLRow, error)
	MonthlyCashflow(ctx context.Context, arg sqlc.MonthlyCashflowParams) ([]sqlc.MonthlyCashflowRow, error)
	AgingAR(ctx context.Context, arg sqlc.AgingARParams) ([]sqlc.AgingARRow, error)
	AgingAP(ctx context.Context, arg sqlc.AgingAPParams) ([]sqlc.AgingAPRow, error)
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

func optionalBranch(branchID *int64) pgtype.Int8 {
	if branchID == nil {
		return pgtype.Int8{Valid: false}
	}
	return pgtype.Int8{Int64: *branchID, Valid: true}
}

func dateParam(t time.Time) pgtype.Date {
	if t.IsZero() {
		return pgtype.Date{Valid: false}
	}
	return pgtype.Date{Time: t, Valid: true}
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

