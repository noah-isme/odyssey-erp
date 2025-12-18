package shared

// Finance permissions declared for RBAC.
const (
	PermFinanceGLView      = "finance.gl.view"
	PermFinanceGLEdit      = "finance.gl.edit"
	PermFinancePeriodClose = "finance.period.close"
	PermFinanceOverride    = "finance.override.lock"
	PermFinanceBoardPack   = "finance.boardpack"

	// AR permissions
	PermFinanceARView = "finance.ar.view"
	PermFinanceAREdit = "finance.ar.edit"
)

// FinanceScopes lists all permissions related to the finance module.
func FinanceScopes() []string {
	return []string{
		PermFinanceGLView,
		PermFinanceGLEdit,
		PermFinancePeriodClose,
		PermFinanceOverride,
		PermFinanceBoardPack,
		PermFinanceARView,
		PermFinanceAREdit,
	}
}
