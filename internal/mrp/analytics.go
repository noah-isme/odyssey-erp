package mrp

import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
	"time"
)

type ManufacturingMetrics struct {
	GoodQuantity, ScrapQuantity, WIPValue float64
	OnTimeOperations, CompletedOperations int64
}
type ManufacturingAnalytics struct{ pool *pgxpool.Pool }

func NewManufacturingAnalytics(pool *pgxpool.Pool) *ManufacturingAnalytics {
	return &ManufacturingAnalytics{pool: pool}
}
func (s *ManufacturingAnalytics) Metrics(ctx context.Context, companyID, workCenterID, productID int64, from, to time.Time) (ManufacturingMetrics, error) {
	var m ManufacturingMetrics
	err := s.pool.QueryRow(ctx, `SELECT COALESCE(SUM(good_quantity),0)::float8,COALESCE(SUM(scrap_quantity),0)::float8 FROM mrp_analytics_operation_daily WHERE company_id=$1 AND ($2=0 OR work_center_id=$2) AND ($3=0 OR product_id=$3) AND ($4::date IS NULL OR day>=$4::date) AND ($5::date IS NULL OR day<=$5::date)`, companyID, workCenterID, productID, nullableTime(from), nullableTime(to)).Scan(&m.GoodQuantity, &m.ScrapQuantity)
	if err != nil {
		return m, err
	}
	if err = s.pool.QueryRow(ctx, `SELECT COALESCE(SUM(value),0)::float8 FROM mrp_analytics_wip_value WHERE company_id=$1 AND ($2=0 OR product_id=$2)`, companyID, productID).Scan(&m.WIPValue); err != nil {
		return m, err
	}
	err = s.pool.QueryRow(ctx, `SELECT COUNT(*) FILTER(WHERE on_time),COUNT(*) FILTER(WHERE completed_at IS NOT NULL) FROM mrp_analytics_schedule_adherence WHERE company_id=$1 AND ($2=0 OR work_center_id=$2) AND ($3=0 OR product_id=$3)`, companyID, workCenterID, productID).Scan(&m.OnTimeOperations, &m.CompletedOperations)
	return m, err
}
func nullableTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t
}
