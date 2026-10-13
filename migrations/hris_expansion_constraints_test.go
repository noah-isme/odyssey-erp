package migrations_test

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

const hrCompanyClause = "company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE"

// readMigrationPair returns the up and down SQL for a migration base name.
func readMigrationPair(t *testing.T, base string) (up, down string) {
	t.Helper()
	u, err := os.ReadFile(base + ".up.sql")
	if err != nil {
		t.Fatal(err)
	}
	d, err := os.ReadFile(base + ".down.sql")
	if err != nil {
		t.Fatal(err)
	}
	return string(u), string(d)
}

var hrCreateTableRe = regexp.MustCompile(`(?is)CREATE TABLE (?:IF NOT EXISTS )?(hr_\w+)\s*\((.*?)\n\);`)

// hrCreateTableBlocks maps each `CREATE TABLE hr_...` name to its column body.
func hrCreateTableBlocks(up string) map[string]string {
	out := map[string]string{}
	for _, m := range hrCreateTableRe.FindAllStringSubmatch(up, -1) {
		out[m[1]] = m[2]
	}
	return out
}

// assertHRTablesHaveCompanyClause asserts the exact company_id clause in the
// named tables (or in every hr_ table when names is empty), requiring at least
// the expected tables to exist.
func assertHRTablesHaveCompanyClause(t *testing.T, up string, names ...string) {
	t.Helper()
	blocks := hrCreateTableBlocks(up)
	for _, n := range names {
		if _, ok := blocks[n]; !ok {
			t.Errorf("up file has no CREATE TABLE %s block", n)
		}
	}
	for n, body := range blocks {
		if !strings.Contains(body, hrCompanyClause) {
			t.Errorf("table %s missing %q", n, hrCompanyClause)
		}
	}
}

func assertContainsAll(t *testing.T, label, content string, fragments ...string) {
	t.Helper()
	norm := strings.Join(strings.Fields(content), " ")
	for _, f := range fragments {
		if !strings.Contains(norm, f) {
			t.Errorf("%s missing %q", label, f)
		}
	}
}

// assertPermissionsInBoth asserts each permission name appears in up and down.
func assertPermissionsInBoth(t *testing.T, up, down string, perms ...string) {
	t.Helper()
	for _, p := range perms {
		q := "'" + p + "'"
		if !strings.Contains(up, q) {
			t.Errorf("up missing permission %s", p)
		}
		if !strings.Contains(down, q) {
			t.Errorf("down missing permission %s", p)
		}
	}
}

func assertNoLike(t *testing.T, down string) {
	t.Helper()
	if regexp.MustCompile(`(?i)\bLIKE\b`).MatchString(down) {
		t.Error("down file must not use LIKE")
	}
}

func TestMigration000130HRISProfileHistory(t *testing.T) {
	up, down := readMigrationPair(t, "000130_hris_profile_history")

	assertHRTablesHaveCompanyClause(t, up, "hr_employee_emergency_contacts", "hr_employee_job_history")

	assertContainsAll(t, "up", up,
		"CHECK (gender IN ('MALE','FEMALE','OTHER'))",
		"CHECK (marital_status IN ('SINGLE','MARRIED','DIVORCED','WIDOWED'))",
		"CHECK (dependents_count >= 0)",
		"CHECK (employment_type IN ('PERMANENT','CONTRACT','PROBATION','INTERN'))",
		"CHECK (status IN ('ACTIVE','INACTIVE'))",
		"contract_end_date >= contract_start_date",
		"CHECK (valid_to IS NULL OR valid_to >= valid_from)",
		"UNIQUE (employee_id, valid_from)",
		"INSERT INTO hr_employee_job_history",
		"e.hire_date",
		"FROM hr_employees e",
		"ALTER TABLE payroll_compensation_assignments",
		"ADD COLUMN reason TEXT NOT NULL DEFAULT ''",
		"ADD COLUMN created_by BIGINT NULL REFERENCES users(id) ON DELETE SET NULL",
		"(employee_id, valid_from DESC)",
	)

	perms := []string{"hr.compensation.view", "hr.compensation.admin", "hr.employee.sensitive"}
	assertPermissionsInBoth(t, up, down, perms...)

	assertContainsAll(t, "down", down,
		"DROP TABLE hr_employee_job_history",
		"DROP TABLE hr_employee_emergency_contacts",
		"DROP COLUMN created_by",
		"DROP COLUMN reason",
		"DROP COLUMN date_of_birth",
		"DROP COLUMN termination_reason",
		"DELETE FROM role_permissions WHERE permission_id IN ( SELECT id FROM permissions WHERE name IN (",
		"DELETE FROM permissions WHERE name IN (",
		"-- WARNING: this is an irreversible PII loss",
	)
	assertNoLike(t, down)
}

func TestMigration000131HRISTime(t *testing.T) {
	up, down := readMigrationPair(t, "000131_hris_time")

	assertHRTablesHaveCompanyClause(t, up,
		"hr_holidays", "hr_shifts", "hr_employee_shifts", "hr_overtime_requests")

	assertContainsAll(t, "up", up,
		"UNIQUE (company_id, holiday_date)",
		"UNIQUE (company_id, code)",
		"CHECK (break_minutes >= 0)",
		"work_days SMALLINT[] NOT NULL DEFAULT '{1,2,3,4,5}'",
		"shift_id BIGINT NOT NULL REFERENCES hr_shifts(id) ON DELETE RESTRICT",
		"CHECK (valid_to IS NULL OR valid_to >= valid_from)",
		"UNIQUE (employee_id, valid_from)",
		"CHECK (minutes > 0 AND minutes <= 720)",
		"CHECK (status IN ('PENDING','APPROVED','REJECTED','CANCELLED'))",
		"approval_request_id BIGINT NULL REFERENCES approval_requests(id) ON DELETE SET NULL",
		"apply_error TEXT NOT NULL DEFAULT ''",
		"decided_by BIGINT NULL REFERENCES users(id) ON DELETE SET NULL",
		"hr_overtime_requests(employee_id, worked_on)",
		"hr_overtime_requests(company_id, status)",
		"CREATE UNIQUE INDEX uq_hr_overtime_requests_active ON hr_overtime_requests(employee_id, worked_on) WHERE status IN ('PENDING','APPROVED')",
		"ALTER TABLE hr_leave_requests",
		"ADD COLUMN start_half_day BOOLEAN NOT NULL DEFAULT FALSE",
		"ADD COLUMN end_half_day BOOLEAN NOT NULL DEFAULT FALSE",
	)

	perms := []string{"hr.schedule.admin", "hr.overtime.request", "hr.overtime.admin"}
	assertPermissionsInBoth(t, up, down, perms...)

	assertContainsAll(t, "down", down,
		"DROP TABLE hr_overtime_requests",
		"DROP TABLE hr_employee_shifts",
		"DROP TABLE hr_shifts",
		"DROP TABLE hr_holidays",
		"DROP COLUMN start_half_day",
		"DROP COLUMN end_half_day",
		"DELETE FROM role_permissions WHERE permission_id IN ( SELECT id FROM permissions WHERE name IN (",
		"DELETE FROM permissions WHERE name IN (",
	)
	assertNoLike(t, down)
}

func TestMigration000132HRISSelfService(t *testing.T) {
	up, down := readMigrationPair(t, "000132_hris_self_service")

	assertHRTablesHaveCompanyClause(t, up, "hr_expense_claims", "hr_profile_change_requests")

	assertContainsAll(t, "up", up,
		"CHECK (amount > 0)",
		"currency CHAR(3) NOT NULL DEFAULT 'IDR'",
		"document_id BIGINT NULL REFERENCES documents(id) ON DELETE SET NULL",
		"CHECK (status IN ('PENDING','APPROVED','REJECTED','CANCELLED'))",
		"CHECK (change_type IN ('ADDRESS','BANK','EMERGENCY_CONTACT'))",
		"payload JSONB NOT NULL",
		"approval_request_id BIGINT NULL REFERENCES approval_requests(id) ON DELETE SET NULL",
		"apply_error TEXT NOT NULL DEFAULT ''",
		"applied_at TIMESTAMPTZ NULL",
		"decided_by BIGINT NULL REFERENCES users(id) ON DELETE SET NULL",
		"hr_expense_claims(company_id, status)",
		"hr_expense_claims(employee_id, claim_date)",
		"hr_profile_change_requests(company_id, status)",
	)

	assertPermissionsInBoth(t, up, down, "hr.expense.admin", "hr.profile_change.admin")

	assertContainsAll(t, "down", down,
		"DROP TABLE hr_profile_change_requests",
		"DROP TABLE hr_expense_claims",
		"DELETE FROM role_permissions WHERE permission_id IN ( SELECT id FROM permissions WHERE name IN (",
		"DELETE FROM permissions WHERE name IN (",
	)
	assertNoLike(t, down)
}

func TestMigration000133HRISChecklists(t *testing.T) {
	up, down := readMigrationPair(t, "000133_hris_checklists")

	assertHRTablesHaveCompanyClause(t, up,
		"hr_checklist_templates",
		"hr_checklist_template_items",
		"hr_employee_checklists",
		"hr_employee_checklist_tasks",
	)

	assertContainsAll(t, "up", up,
		"CHECK (kind IN ('ONBOARDING','OFFBOARDING'))",
		"CHECK (assignee_role IN ('HR','IT','MANAGER'))",
		"CHECK (action_key IN ('DEACTIVATE_USER','RETURN_ASSETS','CLOSE_LEAVE','CLOSE_BENEFITS'))",
		"CHECK (status IN ('OPEN','COMPLETED','CANCELLED'))",
		"CHECK (status IN ('OPEN','DONE','SKIPPED'))",
		"UNIQUE (company_id, code)",
		"UNIQUE (template_id, seq)",
		"default_assignee_user_id BIGINT NULL REFERENCES users(id) ON DELETE SET NULL",
		"template_id BIGINT NOT NULL REFERENCES hr_checklist_templates(id) ON DELETE RESTRICT",
		"CREATE UNIQUE INDEX uq_hr_employee_checklists_open ON hr_employee_checklists(employee_id, kind) WHERE status = 'OPEN'",
		"hr_employee_checklist_tasks(assignee_user_id, status)",
		"hr_employee_checklist_tasks(status, due_date)",
		"ALTER TABLE fixed_assets ADD COLUMN custodian_employee_id BIGINT NULL REFERENCES hr_employees(id) ON DELETE SET NULL",
		"ON fixed_assets(custodian_employee_id)",
	)

	assertPermissionsInBoth(t, up, down, "hr.checklist.admin")

	assertContainsAll(t, "down", down,
		"ALTER TABLE fixed_assets DROP COLUMN custodian_employee_id",
		"DROP TABLE hr_employee_checklist_tasks",
		"DROP TABLE hr_employee_checklists",
		"DROP TABLE hr_checklist_template_items",
		"DROP TABLE hr_checklist_templates",
		"DELETE FROM role_permissions WHERE permission_id IN ( SELECT id FROM permissions WHERE name IN (",
		"DELETE FROM permissions WHERE name IN (",
	)
	assertNoLike(t, down)
}

func TestMigration000134HRISPerformanceCycles(t *testing.T) {
	up, down := readMigrationPair(t, "000134_hris_performance_cycles")

	assertHRTablesHaveCompanyClause(t, up,
		"hr_performance_cycles",
		"hr_performance_participants",
		"hr_performance_goals",
	)

	assertContainsAll(t, "up", up,
		"CHECK (status IN ('DRAFT','ACTIVE','CLOSED'))",
		"CHECK (status IN ('ENROLLED','SELF_SUBMITTED','MANAGER_SUBMITTED','COMPLETED'))",
		"CHECK (period_end >= period_start)",
		"UNIQUE (company_id, name)",
		"UNIQUE (cycle_id, employee_id)",
		"UNIQUE (participant_id, seq)",
		"reviewer_employee_id BIGINT NULL REFERENCES hr_employees(id) ON DELETE SET NULL",
		"review_id BIGINT NULL REFERENCES hr_performance_reviews(id) ON DELETE SET NULL",
		"CHECK (self_rating BETWEEN 1 AND 5)",
		"CHECK (manager_rating BETWEEN 1 AND 5)",
		"CHECK (final_rating BETWEEN 1 AND 5)",
		"CHECK (self_score BETWEEN 1 AND 5)",
		"CHECK (manager_score BETWEEN 1 AND 5)",
		"weight NUMERIC(5,2) NOT NULL DEFAULT 0 CHECK (weight BETWEEN 0 AND 100)",
		"ALTER TABLE hr_performance_reviews ADD COLUMN company_id BIGINT NULL REFERENCES companies(id)",
		"ON hr_performance_reviews(company_id)",
	)

	// The backfill must run before the column becomes NOT NULL.
	norm := strings.Join(strings.Fields(up), " ")
	backfill := strings.Index(norm, "UPDATE hr_performance_reviews")
	notNull := strings.Index(norm, "ALTER COLUMN company_id SET NOT NULL")
	if backfill < 0 || notNull < 0 || backfill > notNull {
		t.Errorf("backfill UPDATE (%d) must appear before SET NOT NULL (%d)", backfill, notNull)
	}
	if !strings.Contains(norm, "SET company_id = e.company_id FROM hr_employees e WHERE e.id = pr.employee_id") {
		t.Error("backfill must derive company_id from hr_employees")
	}

	assertPermissionsInBoth(t, up, down,
		"hr.performance.view", "hr.performance.admin", "hr.reports.view")

	assertContainsAll(t, "down", down,
		"ALTER TABLE hr_performance_reviews DROP COLUMN company_id",
		"DROP TABLE hr_performance_goals",
		"DROP TABLE hr_performance_participants",
		"DROP TABLE hr_performance_cycles",
		"DELETE FROM role_permissions WHERE permission_id IN ( SELECT id FROM permissions WHERE name IN (",
		"DELETE FROM permissions WHERE name IN (",
	)
	dn := strings.Join(strings.Fields(down), " ")
	if strings.Index(dn, "DROP TABLE hr_performance_goals") > strings.Index(dn, "DROP TABLE hr_performance_participants") ||
		strings.Index(dn, "DROP TABLE hr_performance_participants") > strings.Index(dn, "DROP TABLE hr_performance_cycles") {
		t.Error("down must drop tables in reverse dependency order")
	}
	assertNoLike(t, down)
}
