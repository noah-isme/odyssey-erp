package analytics

import (
	"context"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/odyssey-erp/odyssey-erp/internal/analytics/db"
)

// Repository exposes the generated sqlc queries we rely on.
type Repository interface {
	KpiSummary(ctx context.Context, arg analyticsdb.KpiSummaryParams) (analyticsdb.KpiSummaryRow, error)
	MonthlyPL(ctx context.Context, arg analyticsdb.MonthlyPLParams) ([]analyticsdb.MonthlyPLRow, error)
	MonthlyCashflow(ctx context.Context, arg analyticsdb.MonthlyCashflowParams) ([]analyticsdb.MonthlyCashflowRow, error)
	AgingAR(ctx context.Context, arg analyticsdb.AgingARParams) ([]analyticsdb.AgingARRow, error)
	AgingAP(ctx context.Context, arg analyticsdb.AgingAPParams) ([]analyticsdb.AgingAPRow, error)
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
