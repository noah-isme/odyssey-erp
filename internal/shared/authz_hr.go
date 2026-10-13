package shared

// HR permissions declared for RBAC. Later HRIS phases append to this block.
const (
	PermHRCompensationView   = "hr.compensation.view"
	PermHRCompensationAdmin  = "hr.compensation.admin"
	PermHREmployeeSensitive  = "hr.employee.sensitive"
	PermHRScheduleAdmin      = "hr.schedule.admin"
	PermHROvertimeRequest    = "hr.overtime.request"
	PermHROvertimeAdmin      = "hr.overtime.admin"
	PermHRExpenseAdmin       = "hr.expense.admin"
	PermHRProfileChangeAdmin = "hr.profile_change.admin"
	PermHRChecklistAdmin     = "hr.checklist.admin"
	PermHRPerformanceView    = "hr.performance.view"
	PermHRPerformanceAdmin   = "hr.performance.admin"
	PermHRReportsView        = "hr.reports.view"
)

// HRScopes lists the HRIS-expansion permissions.
func HRScopes() []string {
	return []string{
		PermHRCompensationView,
		PermHRCompensationAdmin,
		PermHREmployeeSensitive,
		PermHRScheduleAdmin,
		PermHROvertimeRequest,
		PermHROvertimeAdmin,
		PermHRExpenseAdmin,
		PermHRProfileChangeAdmin,
		PermHRChecklistAdmin,
		PermHRPerformanceView,
		PermHRPerformanceAdmin,
		PermHRReportsView,
	}
}
