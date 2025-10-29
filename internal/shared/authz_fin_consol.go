package shared

// Finance consolidation permissions for RBAC enforcement.
const (
	PermFinanceConsolView     = "finance.view_consolidation"
	PermFinanceConsolPostElim = "finance.post_elimination"
	PermFinanceConsolManage   = "finance.manage_consolidation"
	PermFinanceConsolExport   = "finance.export_consolidation"
)

// FinanceConsolidationScopes returns all consolidation permissions.
func FinanceConsolidationScopes() []string {
	return []string{
		PermFinanceConsolView,
		PermFinanceConsolPostElim,
		PermFinanceConsolManage,
		PermFinanceConsolExport,
	}
}
