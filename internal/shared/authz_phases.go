package shared

// PermissionSpec is the authoritative description of a permission introduced
// by the Phase 1–6 route guards. Keep this inventory in sync with those guards
// and use it when provisioning or repairing RBAC data.
type PermissionSpec struct {
	Name        string
	Description string
}

const (
	PermProcurementReturnView    = "procurement.return.view"
	PermProcurementReturnCreate  = "procurement.return.create"
	PermProcurementReturnPost    = "procurement.return.post"
	PermProcurementReturnVoid    = "procurement.return.void"
	PermFinanceAPDebitNoteView   = "finance.ap.debit_note.view"
	PermFinanceAPDebitNoteCreate = "finance.ap.debit_note.create"
	PermFinanceAPDebitNotePost   = "finance.ap.debit_note.post"
	PermFinanceAPDebitNoteVoid   = "finance.ap.debit_note.void"
	PermApprovalsInbox           = "approvals.inbox"
	PermApprovalsPolicyAdmin     = "approvals.policy.admin"
	PermApprovalsDelegate        = "approvals.delegate"
	PermHREmployeeView           = "hr.employee.view"
	PermHREmployeeAdmin          = "hr.employee.admin"
	PermHRLeaveRequest           = "hr.leave.request"
	PermHRLeaveAdmin             = "hr.leave.admin"
	PermHRAttendanceImport       = "hr.attendance.import"
	PermPayrollView              = "payroll.view"
	PermPayrollProcess           = "payroll.process"
	PermPayrollPost              = "payroll.post"
	PermPayrollPolicyAdmin       = "payroll.policy.admin"
	PermPayrollPayslipOwn        = "payroll.payslip.own"
	PermPayrollPayslipManager    = "payroll.payslip.manager"
	PermTaxView                  = "tax.view"
	PermTaxConfigManage          = "tax.config.manage"
	PermTaxPeriodLock            = "tax.period.lock"
	PermTaxDocumentCorrect       = "tax.document.correct"
	PermTaxReportExport          = "tax.report.export"
	PermCRMView                  = "crm.view"
	PermCRMCreate                = "crm.create"
	PermCRMEdit                  = "crm.edit"
	PermCRMConvert               = "crm.convert"
	PermCRMTeamView              = "crm.team.view"
	PermCRMManage                = "crm.manage"
)

// Phase1To6Permissions contains every permission used by the Phase 1–6
// RequireAny and RequireAll guards. Notifications intentionally have no entry:
// their handlers require an authenticated session but no module permission.
var Phase1To6Permissions = []PermissionSpec{
	{PermDeliveryReturnView, "View return delivery orders"},
	{PermDeliveryReturnCreate, "Create return delivery orders"},
	{PermDeliveryReturnPost, "Post return delivery orders"},
	{PermDeliveryReturnVoid, "Void return delivery orders"},
	{PermFinanceARCreditNoteView, "View AR credit notes"},
	{PermFinanceARCreditNoteCreate, "Create AR credit notes"},
	{PermFinanceARCreditNotePost, "Post AR credit notes"},
	{PermFinanceARCreditNoteVoid, "Void AR credit notes"},
	{PermFinanceAPDebitNoteView, "View AP debit notes"},
	{PermFinanceAPDebitNoteCreate, "Create AP debit notes"},
	{PermFinanceAPDebitNotePost, "Post AP debit notes"},
	{PermFinanceAPDebitNoteVoid, "Void AP debit notes"},
	{PermProcurementReturnView, "View goods returns"},
	{PermProcurementReturnCreate, "Create goods returns"},
	{PermProcurementReturnPost, "Post goods returns"},
	{PermProcurementReturnVoid, "Void goods returns"},
	{PermApprovalsInbox, "View and decide assigned approvals"},
	{PermApprovalsPolicyAdmin, "Create and manage approval policies"},
	{PermApprovalsDelegate, "Manage approval delegations"},
	{PermHREmployeeView, "View employee directory"},
	{PermHREmployeeAdmin, "Manage employee records"},
	{PermHRLeaveRequest, "Create and view own leave requests"},
	{PermHRLeaveAdmin, "Manage leave types and balances"},
	{PermHRAttendanceImport, "Import attendance CSV files"},
	{PermPayrollView, "View payroll runs"},
	{PermPayrollProcess, "Create, calculate, and submit payroll runs"},
	{PermPayrollPost, "Post approved payroll and export payments"},
	{PermPayrollPolicyAdmin, "Manage payroll rules and account mappings"},
	{PermPayrollPayslipOwn, "View own payslips"},
	{PermPayrollPayslipManager, "View authorized reports payslips"},
	{PermTaxView, "View tax documents, ledgers, and recaps"},
	{PermTaxConfigManage, "Manage reviewed tax configuration"},
	{PermTaxPeriodLock, "Lock tax reporting periods"},
	{PermTaxDocumentCorrect, "Cancel or replace tax documents"},
	{PermTaxReportExport, "Generate tax authority exports"},
	{PermCRMView, "View owned CRM records"},
	{PermCRMCreate, "Create CRM leads, opportunities, and activities"},
	{PermCRMEdit, "Update owned CRM records"},
	{PermCRMConvert, "Convert won opportunities to customers and quotations"},
	{PermCRMTeamView, "View all company CRM records"},
	{PermCRMManage, "Administer all company CRM records"},
}

// Phase1To6PermissionNames returns a copy so callers cannot mutate the
// authoritative inventory accidentally.
func Phase1To6PermissionNames() []string {
	names := make([]string, 0, len(Phase1To6Permissions))
	for _, permission := range Phase1To6Permissions {
		names = append(names, permission.Name)
	}
	return names
}
