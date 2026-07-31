package shared

import "testing"

func TestPhase1To6PermissionsAreUniqueAndComplete(t *testing.T) {
	want := []string{
		"delivery.return.view", "delivery.return.create", "delivery.return.post", "delivery.return.void",
		"finance.ar.credit_note.view", "finance.ar.credit_note.create", "finance.ar.credit_note.post", "finance.ar.credit_note.void",
		"finance.ap.debit_note.view", "finance.ap.debit_note.create", "finance.ap.debit_note.post", "finance.ap.debit_note.void",
		"procurement.return.view", "procurement.return.create", "procurement.return.post", "procurement.return.void",
		"approvals.inbox", "approvals.policy.admin", "approvals.delegate",
		"hr.employee.view", "hr.employee.admin", "hr.leave.request", "hr.leave.admin", "hr.attendance.import",
		"payroll.view", "payroll.process", "payroll.post", "payroll.policy.admin", "payroll.payslip.own", "payroll.payslip.manager",
		"tax.view", "tax.config.manage", "tax.period.lock", "tax.document.correct", "tax.report.export",
		"crm.view", "crm.create", "crm.edit", "crm.convert", "crm.team.view", "crm.manage",
	}
	got := Phase1To6PermissionNames()
	if len(got) != len(want) {
		t.Fatalf("permission inventory has %d entries, want %d", len(got), len(want))
	}
	seen := make(map[string]bool, len(got))
	for _, name := range got {
		if seen[name] {
			t.Fatalf("duplicate permission %q", name)
		}
		seen[name] = true
	}
	for _, name := range want {
		if !seen[name] {
			t.Errorf("missing permission %q", name)
		}
	}
}
