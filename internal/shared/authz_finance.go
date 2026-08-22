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

	// Finance automation permissions. Payment duty separation is enforced by
	// the workflow policy, not by assigning one mutually exclusive global role.
	PermFinanceAutomationManage        = "finance.automation.manage"
	PermFinanceBankFeedManage          = "finance.bank_feed.manage"
	PermFinanceForecastView            = "finance.forecast.view"
	PermFinanceForecastManage          = "finance.forecast.manage"
	PermFinancePaymentPropose          = "finance.payment.propose"
	PermFinancePaymentApprove          = "finance.payment.approve"
	PermFinancePaymentExport           = "finance.payment.export"
	PermFinancePaymentExecute          = "finance.payment.execute"
	PermFinancePaymentView             = "finance.payment.view"
	PermProcurementP2PExceptionView    = "procurement.p2p_exception.view"
	PermProcurementP2PExceptionResolve = "procurement.p2p_exception.resolve"
	PermFixedAssetsLocationManage      = "fixedassets.location.manage"
	PermFixedAssetsTransferManage      = "fixedassets.transfer.manage"
	PermFixedAssetsMaintenanceManage   = "fixedassets.maintenance.manage"
	PermFixedAssetsWarrantyManage      = "fixedassets.warranty.manage"
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
		PermFinanceAutomationManage,
		PermFinanceBankFeedManage,
		PermFinanceForecastView,
		PermFinanceForecastManage,
		PermFinancePaymentPropose,
		PermFinancePaymentApprove,
		PermFinancePaymentExport,
		PermFinancePaymentExecute,
		PermFinancePaymentView,
		PermProcurementP2PExceptionView,
		PermProcurementP2PExceptionResolve,
		PermFixedAssetsLocationManage,
		PermFixedAssetsTransferManage,
		PermFixedAssetsMaintenanceManage,
		PermFixedAssetsWarrantyManage,
	}
}
