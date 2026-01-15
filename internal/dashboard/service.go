package dashboard

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// KPIData holds all dashboard KPI metrics.
type KPIData struct {
	// Sales KPIs
	OpenSalesOrders     int     `json:"open_sales_orders"`
	OverdueSO           int     `json:"overdue_so"`
	PendingDeliveries   int     `json:"pending_deliveries"`
	TotalSalesThisMonth float64 `json:"total_sales_this_month"`

	// AR KPIs
	AROutstanding float64 `json:"ar_outstanding"`
	AROverdue     float64 `json:"ar_overdue"`
	ARCurrent     float64 `json:"ar_current"`

	// AP KPIs
	APOutstanding float64 `json:"ap_outstanding"`
	APDueThisWeek float64 `json:"ap_due_this_week"`

	// Inventory KPIs
	LowStockItems       int     `json:"low_stock_items"`
	TotalInventoryValue float64 `json:"total_inventory_value"`

	// Finance KPIs
	CashPosition     float64 `json:"cash_position"`
	NetProfitMTD     float64 `json:"net_profit_mtd"`
	RevenueThisMonth float64 `json:"revenue_this_month"`

	// Pending Actions
	SOPendingApproval int `json:"so_pending_approval"`
	POPendingApproval int `json:"po_pending_approval"`
	InvoicesPending   int `json:"invoices_pending"`
}

// RecentActivity represents a single activity entry.
type RecentActivity struct {
	At       time.Time `json:"at"`
	Actor    string    `json:"actor"`
	Action   string    `json:"action"`
	Entity   string    `json:"entity"`
	EntityID string    `json:"entity_id"`
	Summary  string    `json:"summary"`
}

// Service provides dashboard data.
type Service struct {
	pool *pgxpool.Pool
}

// NewService creates a dashboard service.
func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

// GetKPIs fetches all dashboard KPIs.
func (s *Service) GetKPIs(ctx context.Context, companyID int64) (*KPIData, error) {
	kpi := &KPIData{}

	// Open Sales Orders and overdue
	row := s.pool.QueryRow(ctx, `
		SELECT 
			COUNT(*) FILTER (WHERE status IN ('DRAFT', 'CONFIRMED', 'PROCESSING')),
			COUNT(*) FILTER (WHERE status IN ('CONFIRMED', 'PROCESSING') AND expected_delivery_date < CURRENT_DATE)
		FROM sales_orders
		WHERE company_id = $1
	`, companyID)
	_ = row.Scan(&kpi.OpenSalesOrders, &kpi.OverdueSO)

	// Pending Deliveries
	row = s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM delivery_orders 
		WHERE status IN ('DRAFT', 'CONFIRMED') AND company_id = $1
	`, companyID)
	_ = row.Scan(&kpi.PendingDeliveries)

	// AR Outstanding and Overdue
	row = s.pool.QueryRow(ctx, `
		SELECT 
			COALESCE(SUM(total_amount - COALESCE(paid_amount, 0)), 0),
			COALESCE(SUM(CASE WHEN due_date < CURRENT_DATE THEN total_amount - COALESCE(paid_amount, 0) ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN due_date >= CURRENT_DATE THEN total_amount - COALESCE(paid_amount, 0) ELSE 0 END), 0)
		FROM ar_invoices
		WHERE company_id = $1 AND status = 'POSTED'
	`, companyID)
	_ = row.Scan(&kpi.AROutstanding, &kpi.AROverdue, &kpi.ARCurrent)

	// AP Outstanding and Due This Week
	row = s.pool.QueryRow(ctx, `
		SELECT 
			COALESCE(SUM(total_amount - COALESCE(paid_amount, 0)), 0),
			COALESCE(SUM(CASE WHEN due_date <= CURRENT_DATE + INTERVAL '7 days' THEN total_amount - COALESCE(paid_amount, 0) ELSE 0 END), 0)
		FROM ap_invoices
		WHERE status = 'POSTED'
	`, companyID)
	_ = row.Scan(&kpi.APOutstanding, &kpi.APDueThisWeek)

	// Pending Approvals
	row = s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM sales_orders WHERE status = 'DRAFT' AND company_id = $1
	`, companyID)
	_ = row.Scan(&kpi.SOPendingApproval)

	row = s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM pos WHERE status = 'APPROVAL'
	`)
	_ = row.Scan(&kpi.POPendingApproval)

	// Low stock items (items with qty < 10 as threshold)
	row = s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM inventory_balances WHERE qty < 10 AND qty > 0
	`)
	_ = row.Scan(&kpi.LowStockItems)

	// Total Inventory Value
	row = s.pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(qty * avg_cost), 0) FROM inventory_balances
	`)
	_ = row.Scan(&kpi.TotalInventoryValue)

	// Revenue this month from analytics MV if available
	row = s.pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(revenue), 0), COALESCE(SUM(net), 0)
		FROM mv_pl_monthly 
		WHERE period = TO_CHAR(CURRENT_DATE, 'YYYY-MM')
		  AND company_id = $1
	`, companyID)
	_ = row.Scan(&kpi.RevenueThisMonth, &kpi.NetProfitMTD)

	return kpi, nil
}

// GetRecentActivity fetches recent audit log entries.
func (s *Service) GetRecentActivity(ctx context.Context, limit int) ([]RecentActivity, error) {
	if limit <= 0 {
		limit = 10
	}

	rows, err := s.pool.Query(ctx, `
		SELECT 
			a.created_at,
			COALESCE(u.email, 'System'),
			a.action,
			a.entity_type,
			COALESCE(a.entity_id, ''),
			COALESCE(a.meta->>'summary', a.action || ' ' || a.entity_type)
		FROM audit_logs a
		LEFT JOIN users u ON u.id = a.actor_id
		ORDER BY a.created_at DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var activities []RecentActivity
	for rows.Next() {
		var act RecentActivity
		if err := rows.Scan(&act.At, &act.Actor, &act.Action, &act.Entity, &act.EntityID, &act.Summary); err != nil {
			continue
		}
		activities = append(activities, act)
	}

	return activities, nil
}
