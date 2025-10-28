package shared

// Finance analytics permissions for RBAC enforcement.
const (
	PermFinanceAnalyticsView   = "finance.view_analytics"
	PermFinanceAnalyticsExport = "finance.export_analytics"
)

// FinanceAnalyticsScopes returns permissions needed for the analytics dashboard.
func FinanceAnalyticsScopes() []string {
	return []string{
		PermFinanceAnalyticsView,
		PermFinanceAnalyticsExport,
	}
}
