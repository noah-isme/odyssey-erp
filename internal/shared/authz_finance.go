package shared

// Finance permissions declared for RBAC.
const (
	PermFinanceGLView      = "finance.gl.view"
	PermFinanceGLEdit      = "finance.gl.edit"
	PermFinancePeriodClose = "finance.period.close"
	PermFinanceOverride    = "finance.override.lock"
	PermFinanceBoardPack   = "finance.boardpack"

	// AR permissions
	PermFinanceARView             = "finance.ar.view"
	PermFinanceAREdit             = "finance.ar.edit"
	PermFinanceARCreditNoteView   = "finance.ar.credit_note.view"
	PermFinanceARCreditNoteCreate = "finance.ar.credit_note.create"
	PermFinanceARCreditNotePost   = "finance.ar.credit_note.post"
	PermFinanceARCreditNoteVoid   = "finance.ar.credit_note.void"
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
		PermFinanceARCreditNoteView,
		PermFinanceARCreditNoteCreate,
		PermFinanceARCreditNotePost,
		PermFinanceARCreditNoteVoid,
	}
}
