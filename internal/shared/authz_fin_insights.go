package shared

// Finance insights permissions for RBAC enforcement.
const (
	PermFinanceInsightsView   = "finance.view_insights"
	PermFinanceInsightsExport = "finance.export_insights"
)

// FinanceInsightsScopes lists required permissions for insights modules.
func FinanceInsightsScopes() []string {
	return []string{
		PermFinanceInsightsView,
		PermFinanceInsightsExport,
	}
}
