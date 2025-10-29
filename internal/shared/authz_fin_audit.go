package shared

// Finance audit permissions for RBAC enforcement.
const (
	PermFinanceAuditView = "finance.view_audit"
)

// FinanceAuditScopes lists permissions used by audit timeline features.
func FinanceAuditScopes() []string {
	return []string{PermFinanceAuditView}
}
