//go:build staging
// +build staging

package staging

// TestStagingHRCoreJourney is the executable end-to-end scenario for HR Core
// (employees/org, leave with manager approval, attendance). It drives the
// running application over HTTP exactly like TestStagingCoreJourneys and uses
// the database only for read-only verification.
//
// Scenario steps (each is a named t.Run; a failure in setup steps 1-6 stops
// the journey, steps 7-10 are independent and always all run):
//
//  1. route-smoke: admin GETs /hr/employees, /hr/leave, /hr/attendance,
//     /approvals and /approvals/policies (200) and the active company matches.
//  2. rbac-denied: the no-access user is denied the HR list pages, the
//     approvals inbox, and a mutating HR POST (no department is created).
//  3. org-setup: admin creates a run-unique department and position; both are
//     persisted under ODYSSEY_E2E_COMPANY_ID.
//  4. employees: admin ensures a manager employee linked to the manager user
//     and an employee linked to the employee user whose manager is the manager
//     employee. Existing rows (hr_employees.user_id is UNIQUE) are reused via
//     POST /hr/employees/{id}/update, so reruns never create duplicates.
//     The employee and manager users then log in.
//  5. leave-setup: admin creates a run-unique leave type and seeds a 12-day
//     balance for every year the scenario's leave dates fall in. The LEAVE
//     approval policy the engine will resolve (same precedence as
//     approvals.Repository.ResolvePolicy) must be a single employee-manager
//     step; if no LEAVE policy resolves at all, one is created through
//     /approvals/policies, otherwise an unsuitable policy fails the step.
//  6. leave-validation: the employee submits an end date before the start
//     date; the request is rejected and no hr_leave_requests row is written.
//  7. leave-approve: the employee submits future working days; the request is
//     PENDING, balance.pending grows by the inclusive calendar-day count, and
//     one approval assignment targets the manager user. The employee cannot
//     approve it. The manager sees it in /approvals and approves it: request
//     APPROVED, pending moves to used, an audit_logs row and an in-app
//     notification for the employee are written. A duplicate approve is inert.
//  8. leave-reject: a second request is rejected by the manager; pending is
//     released and used is unchanged.
//  9. leave-cancel: a third request is cancelled by the employee; pending is
//     released, the approval request is CANCELLED and leaves the inbox.
//  10. attendance: admin downloads the CSV template (header check), imports a
//     CSV with valid rows, an unknown employee_number and an invalid status
//     (batch counts and errors match, valid rows upserted), re-imports the
//     valid rows with changed times (no duplicates, values updated), records a
//     manual entry, and has a check-out-before-check-in manual entry rejected.
//
// Required environment (no defaults; the test SKIPs when all four
// ODYSSEY_E2E_HR_* variables are unset and FAILs when only some are set):
//
//	ODYSSEY_E2E_URL, PG_DSN,
//	ODYSSEY_E2E_ADMIN_EMAIL, ODYSSEY_E2E_ADMIN_PASSWORD,
//	ODYSSEY_E2E_NO_ACCESS_EMAIL, ODYSSEY_E2E_NO_ACCESS_PASSWORD,
//	ODYSSEY_E2E_COMPANY_ID,
//	ODYSSEY_E2E_HR_EMPLOYEE_EMAIL, ODYSSEY_E2E_HR_EMPLOYEE_PASSWORD
//	  (existing active user holding hr.leave.request),
//	ODYSSEY_E2E_HR_MANAGER_EMAIL, ODYSSEY_E2E_HR_MANAGER_PASSWORD
//	  (existing active user holding approvals.inbox).
//
// This test does not emit certification evidence IDs. When it is run on its
// own, set ODYSSEY_EVIDENCE_STRICT=0 because TestMain otherwise fails the run
// for the missing v0.10-core evidence rows.

import (
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

const (
	envHREmployeeEmail    = "ODYSSEY_E2E_HR_EMPLOYEE_EMAIL"
	envHREmployeePassword = "ODYSSEY_E2E_HR_EMPLOYEE_PASSWORD"
	envHRManagerEmail     = "ODYSSEY_E2E_HR_MANAGER_EMAIL"
	envHRManagerPassword  = "ODYSSEY_E2E_HR_MANAGER_PASSWORD"

	hrLeaveEntitlement   = 12.0
	hrAttendanceTemplate = "employee_number,date,check_in,check_out,status"
)

type hrConfig struct {
	baseURL   string
	pgDSN     string
	companyID int64

	admin    stagingCredentials
	noAccess stagingCredentials
	employee stagingCredentials
	manager  stagingCredentials
}

// loadHRConfig loads only what the HR journey needs, so the journey can run
// without the unrelated core certification fixtures.
func loadHRConfig(t *testing.T) hrConfig {
	t.Helper()
	hrKeys := []string{envHREmployeeEmail, envHREmployeePassword, envHRManagerEmail, envHRManagerPassword}
	var missing []string
	for _, key := range hrKeys {
		if strings.TrimSpace(os.Getenv(key)) == "" {
			missing = append(missing, key)
		}
	}
	if len(missing) == len(hrKeys) {
		t.Skipf("HR Core journey not configured: set %s to run it", strings.Join(hrKeys, ", "))
	}
	if len(missing) > 0 {
		t.Fatalf("HR Core journey is partially configured; missing %s", strings.Join(missing, ", "))
	}

	baseURL := strings.TrimRight(requiredValue(t, envURL), "/")
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		t.Fatalf("%s must be an absolute http(s) URL, got %q", envURL, baseURL)
	}
	return hrConfig{
		baseURL:   baseURL,
		pgDSN:     requiredValue(t, envPGDSN),
		companyID: requiredID(t, envCompanyID),
		admin:     stagingCredentials{email: requiredValue(t, envAdminEmail), password: requiredValue(t, envAdminPassword)},
		noAccess:  stagingCredentials{email: requiredValue(t, envNoAccessEmail), password: requiredValue(t, envNoAccessPassword)},
		employee:  stagingCredentials{email: requiredValue(t, envHREmployeeEmail), password: requiredValue(t, envHREmployeePassword)},
		manager:   stagingCredentials{email: requiredValue(t, envHRManagerEmail), password: requiredValue(t, envHRManagerPassword)},
	}
}

// postMultipartFile posts a single file plus form fields. postMultipart in the
// core harness is fixed to the documents upload shape.
func (c *stagingClient) postMultipartFile(t *testing.T, path string, fields map[string]string, fileField, filename string, content []byte) httpResult {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if err := writer.WriteField(key, fields[key]); err != nil {
			t.Fatalf("multipart field %s: %v", key, err)
		}
	}
	part, err := writer.CreateFormFile(fileField, filename)
	if err != nil {
		t.Fatalf("multipart file: %v", err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatalf("multipart content: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("multipart close: %v", err)
	}
	headers := make(http.Header)
	headers.Set("Content-Type", writer.FormDataContentType())
	return c.do(t, http.MethodPost, path, &body, headers)
}

type hrJourney struct {
	cfg hrConfig
	db  *sql.DB
	run string

	admin, noAccess, employee, manager *stagingClient

	employeeUserID, managerUserID int64
	departmentID, positionID      int64
	managerEmployeeID             int64
	employeeID                    int64
	employeeNumber                string
	leaveTypeID                   int64
	leavePolicyID                 int64

	approveStart, approveEnd time.Time
	rejectStart, rejectEnd   time.Time
	cancelStart, cancelEnd   time.Time
}

type hrBalance struct{ entitled, used, pending float64 }

type hrEmployeeRow struct {
	id, companyID                            int64
	number, name, email, hireDate, status    string
	managerID, departmentID, positionID, uid sql.NullInt64
}

type hrAttendanceRow struct {
	status, source, checkIn, checkOut string
	importID                          sql.NullInt64
}

func TestStagingHRCoreJourney(t *testing.T) {
	cfg := loadHRConfig(t)
	db := openReadOnlyDB(t, cfg.pgDSN)
	defer db.Close()

	now := time.Now().UTC()
	j := &hrJourney{
		cfg: cfg,
		db:  db,
		run: strings.ToUpper(strconv.FormatInt(now.UnixNano(), 36)),
	}
	j.admin = setupClient(t, cfg.baseURL, cfg.admin)

	// Future working days: Monday at least two weeks out, so every request
	// covers Mon-Fri only and the inclusive calendar-day count the service
	// uses equals the working-day count.
	monday := hrNextMonday(now, 14)
	j.approveStart, j.approveEnd = monday, monday.AddDate(0, 0, 2)
	j.rejectStart, j.rejectEnd = monday.AddDate(0, 0, 7), monday.AddDate(0, 0, 8)
	j.cancelStart, j.cancelEnd = monday.AddDate(0, 0, 14), monday.AddDate(0, 0, 14)

	// Steps 01-06 build the fixtures every later step needs, so a failure there
	// stops the journey. Steps 07-10 only depend on that setup (each snapshots
	// the balance it asserts against), so they all run and report separately.
	steps := []struct {
		name  string
		fn    func(*testing.T)
		setup bool
	}{
		{"01-route-smoke", j.stepRouteSmoke, true},
		{"02-rbac-denied", j.stepRBACDenied, true},
		{"03-org-setup", j.stepOrgSetup, true},
		{"04-employees", j.stepEmployees, true},
		{"05-leave-setup", j.stepLeaveSetup, true},
		{"06-leave-validation", j.stepLeaveValidation, true},
		{"07-leave-approve", j.stepLeaveApprove, false},
		{"08-leave-reject", j.stepLeaveReject, false},
		{"09-leave-cancel", j.stepLeaveCancel, false},
		{"10-attendance", j.stepAttendance, false},
	}
	for _, step := range steps {
		if !t.Run(step.name, step.fn) && step.setup {
			t.Fatalf("HR Core journey stopped: setup step %s failed and later steps depend on it", step.name)
		}
	}
}

func (j *hrJourney) stepRouteSmoke(t *testing.T) {
	activeCompanyID, profile, _ := apiMe(t, j.admin)
	if activeCompanyID != j.cfg.companyID {
		t.Fatalf("admin active company = %d, want %d (%s)", activeCompanyID, j.cfg.companyID, envCompanyID)
	}
	for _, path := range []string{"/hr/employees", "/hr/leave", "/hr/attendance", "/approvals", "/approvals/policies"} {
		assertRoute(t, j.admin, path)
	}
	t.Logf("admin HR/approval routes returned 200; release profile=%s company_id=%d run=%s", profile, activeCompanyID, j.run)
}

func (j *hrJourney) stepRBACDenied(t *testing.T) {
	j.noAccess = setupClient(t, j.cfg.baseURL, j.cfg.noAccess)
	for _, path := range []string{"/hr/employees", "/hr/leave", "/hr/attendance", "/approvals"} {
		assertDenied(t, j.noAccess.get(t, path), http.MethodGet, path+" [no-access]")
	}
	code := "E2E-DENIED-" + j.run
	denied := j.noAccess.postForm(t, "/hr/employees/departments", url.Values{
		"csrf_token": {fetchCSRF(t, j.noAccess, "/")},
		"code":       {code},
		"name":       {"E2E denied department " + j.run},
	})
	assertDenied(t, denied, http.MethodPost, "/hr/employees/departments [no-access]")
	if count := queryCount(t, j.db, "SELECT COUNT(*) FROM hr_departments WHERE code=$1", code); count != 0 {
		t.Fatalf("no-access department POST persisted %d rows for code %q, want 0", count, code)
	}
}

func (j *hrJourney) stepOrgSetup(t *testing.T) {
	deptCode, posCode := "E2E-DEPT-"+j.run, "E2E-POS-"+j.run
	dept := j.admin.postForm(t, "/hr/employees/departments", url.Values{
		"csrf_token": {fetchCSRF(t, j.admin, "/hr/employees")},
		"code":       {deptCode},
		"name":       {"E2E Department " + j.run},
	})
	hrAssertRedirect(t, dept, "/hr/employees?tab=departments", http.MethodPost, "/hr/employees/departments")
	j.departmentID = queryInt64(t, j.db, "SELECT id FROM hr_departments WHERE company_id=$1 AND code=$2", j.cfg.companyID, deptCode)

	pos := j.admin.postForm(t, "/hr/employees/positions", url.Values{
		"csrf_token": {fetchCSRF(t, j.admin, "/hr/employees")},
		"code":       {posCode},
		"name":       {"E2E Position " + j.run},
	})
	hrAssertRedirect(t, pos, "/hr/employees?tab=positions", http.MethodPost, "/hr/employees/positions")
	j.positionID = queryInt64(t, j.db, "SELECT id FROM hr_positions WHERE company_id=$1 AND code=$2", j.cfg.companyID, posCode)

	if count := queryCount(t, j.db, "SELECT COUNT(*) FROM hr_departments WHERE code=$1", deptCode); count != 1 {
		t.Fatalf("department %q rows = %d, want 1", deptCode, count)
	}
	if count := queryCount(t, j.db, "SELECT COUNT(*) FROM hr_positions WHERE code=$1", posCode); count != 1 {
		t.Fatalf("position %q rows = %d, want 1", posCode, count)
	}
	t.Logf("department_id=%d position_id=%d company_id=%d", j.departmentID, j.positionID, j.cfg.companyID)
}

func (j *hrJourney) stepEmployees(t *testing.T) {
	j.managerUserID = hrUserID(t, j.db, j.cfg.manager.email, envHRManagerEmail)
	j.employeeUserID = hrUserID(t, j.db, j.cfg.employee.email, envHREmployeeEmail)
	if j.managerUserID == j.employeeUserID {
		t.Fatalf("%s and %s resolve to the same user %d; the scenario needs two users", envHRManagerEmail, envHREmployeeEmail, j.managerUserID)
	}

	j.managerEmployeeID = j.ensureEmployee(t, "MGR", j.managerUserID, j.cfg.manager.email, nil)
	j.employeeID = j.ensureEmployee(t, "EMP", j.employeeUserID, j.cfg.employee.email, &j.managerEmployeeID)
	if j.employeeID == j.managerEmployeeID {
		t.Fatalf("employee and manager resolved to the same hr_employees row %d", j.employeeID)
	}

	linked := queryCount(t, j.db, `
		SELECT COUNT(*)
		FROM hr_employees e
		JOIN hr_employees m ON m.id=e.manager_id
		WHERE e.id=$1 AND e.user_id=$2 AND e.company_id=$3 AND e.status='ACTIVE'
		  AND e.department_id=$4 AND e.position_id=$5
		  AND m.id=$6 AND m.user_id=$7 AND m.company_id=$3 AND m.status='ACTIVE'`,
		j.employeeID, j.employeeUserID, j.cfg.companyID, j.departmentID, j.positionID, j.managerEmployeeID, j.managerUserID)
	if linked != 1 {
		t.Fatalf("employee %d is not linked to user %d with manager employee %d (user %d) in company %d", j.employeeID, j.employeeUserID, j.managerEmployeeID, j.managerUserID, j.cfg.companyID)
	}
	for _, userID := range []int64{j.employeeUserID, j.managerUserID} {
		if count := queryCount(t, j.db, "SELECT COUNT(*) FROM hr_employees WHERE user_id=$1", userID); count != 1 {
			t.Fatalf("user %d has %d hr_employees rows, want exactly 1", userID, count)
		}
	}
	j.employeeNumber = queryString(t, j.db, "SELECT employee_number FROM hr_employees WHERE id=$1", j.employeeID)
	listing := assertRoute(t, j.admin, "/hr/employees?q="+url.QueryEscape(j.employeeNumber))
	if !bytes.Contains(listing.body, []byte(j.employeeNumber)) {
		t.Fatalf("employee directory search did not render employee number %q", j.employeeNumber)
	}
	j.employee = setupClient(t, j.cfg.baseURL, j.cfg.employee)
	j.manager = setupClient(t, j.cfg.baseURL, j.cfg.manager)
	t.Logf("manager_employee_id=%d (user %d) employee_id=%d (user %d, number %s)", j.managerEmployeeID, j.managerUserID, j.employeeID, j.employeeUserID, j.employeeNumber)
}

// ensureEmployee creates the employee row for userID, or updates the existing
// one in place (hr_employees.user_id is UNIQUE). managerEmployeeID nil keeps
// the existing manager of a reused row.
func (j *hrJourney) ensureEmployee(t *testing.T, label string, userID int64, email string, managerEmployeeID *int64) int64 {
	t.Helper()
	existing, found := hrEmployeeByUser(t, j.db, userID)
	if found {
		if existing.companyID != j.cfg.companyID {
			t.Fatalf("precondition: user %d is already linked to hr_employees row %d in company %d, not %s=%d; relink or use another user", userID, existing.id, existing.companyID, envCompanyID, j.cfg.companyID)
		}
		manager := ""
		if managerEmployeeID != nil {
			manager = strconv.FormatInt(*managerEmployeeID, 10)
		} else if existing.managerID.Valid {
			manager = strconv.FormatInt(existing.managerID.Int64, 10)
		}
		path := "/hr/employees/" + strconv.FormatInt(existing.id, 10) + "/update"
		update := j.admin.postForm(t, path, url.Values{
			"csrf_token":      {fetchCSRF(t, j.admin, "/hr/employees")},
			"user_id":         {strconv.FormatInt(userID, 10)},
			"department_id":   {strconv.FormatInt(j.departmentID, 10)},
			"position_id":     {strconv.FormatInt(j.positionID, 10)},
			"manager_id":      {manager},
			"employee_number": {existing.number},
			"name":            {existing.name},
			"email":           {existing.email},
			"hire_date":       {existing.hireDate},
			"status":          {"ACTIVE"},
		})
		hrAssertRedirect(t, update, "/hr/employees?tab=employees", http.MethodPost, path)
		t.Logf("%s: reused hr_employees row %d for user %d", label, existing.id, userID)
	} else {
		number := "E2E-" + label + "-" + j.run
		manager := ""
		if managerEmployeeID != nil {
			manager = strconv.FormatInt(*managerEmployeeID, 10)
		}
		create := j.admin.postForm(t, "/hr/employees", url.Values{
			"csrf_token":      {fetchCSRF(t, j.admin, "/hr/employees")},
			"user_id":         {strconv.FormatInt(userID, 10)},
			"department_id":   {strconv.FormatInt(j.departmentID, 10)},
			"position_id":     {strconv.FormatInt(j.positionID, 10)},
			"manager_id":      {manager},
			"employee_number": {number},
			"name":            {"E2E HR " + label + " " + j.run},
			"email":           {email},
			"hire_date":       {time.Now().UTC().Format("2006-01-02")},
		})
		hrAssertRedirect(t, create, "/hr/employees?tab=employees", http.MethodPost, "/hr/employees")
		t.Logf("%s: created hr_employees row %s for user %d", label, number, userID)
	}

	row, found := hrEmployeeByUser(t, j.db, userID)
	if !found {
		t.Fatalf("%s: no hr_employees row linked to user %d after create/update; check the /hr/employees flash for the rejection", label, userID)
	}
	if row.companyID != j.cfg.companyID || row.status != "ACTIVE" ||
		row.departmentID.Int64 != j.departmentID || row.positionID.Int64 != j.positionID {
		t.Fatalf("%s: hr_employees row %d = company %d status %s department %v position %v; want company %d ACTIVE department %d position %d",
			label, row.id, row.companyID, row.status, row.departmentID, row.positionID, j.cfg.companyID, j.departmentID, j.positionID)
	}
	if managerEmployeeID != nil && (!row.managerID.Valid || row.managerID.Int64 != *managerEmployeeID) {
		t.Fatalf("%s: hr_employees row %d manager_id = %v, want %d", label, row.id, row.managerID, *managerEmployeeID)
	}
	return row.id
}

func (j *hrJourney) stepLeaveSetup(t *testing.T) {
	typeCode := "E2E-LT-" + j.run
	createType := j.admin.postForm(t, "/hr/leave/types", url.Values{
		"csrf_token":   {fetchCSRF(t, j.admin, "/hr/leave")},
		"code":         {typeCode},
		"name":         {"E2E Leave " + j.run},
		"default_days": {strconv.FormatFloat(hrLeaveEntitlement, 'f', 2, 64)},
	})
	hrAssertRedirect(t, createType, "/hr/leave?tab=balances", http.MethodPost, "/hr/leave/types")
	j.leaveTypeID = queryInt64(t, j.db, "SELECT id FROM hr_leave_types WHERE company_id=$1 AND code=$2 AND is_active", j.cfg.companyID, typeCode)

	// Balances are keyed by the request start year, so seed every year used.
	years := map[int]struct{}{}
	for _, start := range []time.Time{j.approveStart, j.rejectStart, j.cancelStart} {
		years[start.Year()] = struct{}{}
	}
	for year := range years {
		seed := j.admin.postForm(t, "/hr/leave/balances", url.Values{
			"csrf_token":    {fetchCSRF(t, j.admin, "/hr/leave")},
			"employee_id":   {strconv.FormatInt(j.employeeID, 10)},
			"leave_type_id": {strconv.FormatInt(j.leaveTypeID, 10)},
			"year":          {strconv.Itoa(year)},
			"days":          {strconv.FormatFloat(hrLeaveEntitlement, 'f', 2, 64)},
		})
		hrAssertRedirect(t, seed, "/hr/leave?tab=balances", http.MethodPost, "/hr/leave/balances")
		balance := j.balance(t, year)
		if !hrSame(balance.entitled, hrLeaveEntitlement) || !hrSame(balance.used, 0) || !hrSame(balance.pending, 0) {
			t.Fatalf("seeded balance for employee %d type %d year %d = %+v, want entitled=%.2f used=0 pending=0", j.employeeID, j.leaveTypeID, year, balance, hrLeaveEntitlement)
		}
	}

	if delegations := queryCount(t, j.db, `
		SELECT COUNT(*) FROM approval_delegations
		WHERE delegator_id=$1 AND is_active AND starts_at<=NOW() AND ends_at>=NOW() AND (module IS NULL OR module='LEAVE')`, j.managerUserID); delegations != 0 {
		t.Fatalf("precondition: manager user %d has %d active LEAVE delegations; assignments would go to the delegate", j.managerUserID, delegations)
	}
	for _, pref := range []struct {
		userID int64
		kind   string
	}{{j.managerUserID, "approval_assigned"}, {j.employeeUserID, "approval_approved"}, {j.employeeUserID, "approval_rejected"}} {
		if disabled := queryCount(t, j.db, "SELECT COUNT(*) FROM notification_preferences WHERE user_id=$1 AND notification_type=$2 AND in_app_enabled=FALSE", pref.userID, pref.kind); disabled != 0 {
			t.Fatalf("precondition: user %d disabled in-app %s notifications; the journey asserts them", pref.userID, pref.kind)
		}
	}

	amounts := []float64{hrDays(j.approveStart, j.approveEnd), hrDays(j.rejectStart, j.rejectEnd), hrDays(j.cancelStart, j.cancelEnd)}
	maxAmount := 0.0
	resolved := map[float64]int64{}
	for _, amount := range amounts {
		maxAmount = math.Max(maxAmount, amount)
		if id, ok := j.resolveLeavePolicy(t, amount); ok {
			resolved[amount] = id
		}
	}
	if len(resolved) == 0 {
		name := "E2E LEAVE manager approval " + j.run
		create := j.admin.postForm(t, "/approvals/policies", url.Values{
			"csrf_token":    {fetchCSRF(t, j.admin, "/approvals/policies")},
			"name":          {name},
			"module":        {"LEAVE"},
			"company_id":    {strconv.FormatInt(j.cfg.companyID, 10)},
			"min_amount":    {"0"},
			"max_amount":    {strconv.FormatFloat(maxAmount, 'f', 2, 64)},
			"step_name":     {"Employee manager"},
			"approver_kind": {"manager"},
		})
		hrAssertRedirect(t, create, "/approvals/policies", http.MethodPost, "/approvals/policies")
		created := queryInt64(t, j.db, "SELECT id FROM approval_policies WHERE name=$1 AND module='LEAVE' AND company_id=$2 AND is_active", name, j.cfg.companyID)
		t.Logf("no LEAVE policy resolved for company %d; created policy %d (%s)", j.cfg.companyID, created, name)
		for _, amount := range amounts {
			if id, ok := j.resolveLeavePolicy(t, amount); ok {
				resolved[amount] = id
			}
		}
	}
	for _, amount := range amounts {
		id, ok := resolved[amount]
		if !ok {
			t.Fatalf("precondition: no active LEAVE approval policy resolves for company %d and %.2f days while others do; configure a policy covering 1-%.0f days", j.cfg.companyID, amount, maxAmount)
		}
		steps := queryCount(t, j.db, "SELECT COUNT(*) FROM approval_policy_steps WHERE policy_id=$1", id)
		managerFirst := queryCount(t, j.db, "SELECT COUNT(*) FROM approval_policy_steps WHERE policy_id=$1 AND step_order=(SELECT MIN(step_order) FROM approval_policy_steps WHERE policy_id=$1) AND approver_manager", id)
		if steps != 1 || managerFirst != 1 {
			t.Fatalf("precondition: LEAVE policy %d resolves for company %d and %.2f days but has %d steps (first step employee-manager=%t); the journey needs a single employee-manager step so the manager's approval finalizes the request", id, j.cfg.companyID, amount, steps, managerFirst == 1)
		}
	}
	if resolved[amounts[0]] != resolved[amounts[1]] || resolved[amounts[0]] != resolved[amounts[2]] {
		t.Logf("note: scenario amounts resolve to different LEAVE policies %v; each is single-step employee-manager", resolved)
	}
	j.leavePolicyID = resolved[amounts[0]]
	t.Logf("leave_type_id=%d policy_id=%d balance_years=%v", j.leaveTypeID, j.leavePolicyID, years)
}

func (j *hrJourney) stepLeaveValidation(t *testing.T) {
	before := queryCount(t, j.db, "SELECT COUNT(*) FROM hr_leave_requests WHERE employee_id=$1", j.employeeID)
	reason := "E2E invalid range " + j.run
	start, end := j.approveStart.AddDate(0, 0, 2), j.approveStart
	result := j.employee.postForm(t, "/hr/leave", url.Values{
		"csrf_token":    {fetchCSRF(t, j.employee, "/hr/leave")},
		"leave_type_id": {strconv.FormatInt(j.leaveTypeID, 10)},
		"start_date":    {start.Format("2006-01-02")},
		"end_date":      {end.Format("2006-01-02")},
		"reason":        {reason},
	})
	hrAssertRedirect(t, result, "/hr/leave?tab=my-requests", http.MethodPost, "/hr/leave [end before start]")
	after := queryCount(t, j.db, "SELECT COUNT(*) FROM hr_leave_requests WHERE employee_id=$1", j.employeeID)
	if after != before || queryCount(t, j.db, "SELECT COUNT(*) FROM hr_leave_requests WHERE reason=$1", reason) != 0 {
		t.Fatalf("invalid date range persisted a leave request: rows %d -> %d", before, after)
	}
	balance := j.balance(t, j.approveStart.Year())
	if !hrSame(balance.pending, 0) || !hrSame(balance.used, 0) {
		t.Fatalf("invalid date range changed balance to %+v", balance)
	}
}

func (j *hrJourney) stepLeaveApprove(t *testing.T) {
	days := hrDays(j.approveStart, j.approveEnd)
	year := j.approveStart.Year()
	before := j.balance(t, year)
	leaveID, approvalID := j.submitLeave(t, "approve", j.approveStart, j.approveEnd)
	j.assertPendingSubmission(t, leaveID, approvalID, days, year, before)

	// The employee must not be able to decide their own request.
	employeeAttempt := j.employee.postForm(t, fmt.Sprintf("/approvals/%d/approve", approvalID), url.Values{
		"csrf_token": {fetchCSRF(t, j.employee, "/hr/leave")},
		"note":       {"self approval attempt"},
	})
	switch employeeAttempt.status {
	case http.StatusForbidden, http.StatusNotFound:
	case http.StatusSeeOther:
		t.Logf("employee approve attempt returned 303 (employee holds approvals.inbox); asserting state is unchanged")
	default:
		t.Fatalf("employee approve attempt status = %d, want 403/404 or 303 with no state change; body=%q", employeeAttempt.status, strings.TrimSpace(string(employeeAttempt.body)))
	}
	j.assertLeaveStatus(t, leaveID, approvalID, "PENDING", "PENDING")
	if decisions := queryCount(t, j.db, "SELECT COUNT(*) FROM approval_decisions WHERE request_id=$1", approvalID); decisions != 0 {
		t.Fatalf("employee approve attempt recorded %d approval decisions, want 0", decisions)
	}

	inbox := assertRoute(t, j.manager, "/approvals")
	if !bytes.Contains(inbox.body, []byte(fmt.Sprintf(`action="/approvals/%d/approve"`, approvalID))) {
		t.Fatalf("manager /approvals inbox does not offer approval request %d", approvalID)
	}
	approvePath := fmt.Sprintf("/approvals/%d/approve", approvalID)
	approve := j.manager.postForm(t, approvePath, url.Values{
		"csrf_token": {csrfFromBody(t, inbox.body, "/approvals")},
		"note":       {"E2E approve " + j.run},
	})
	hrAssertRedirect(t, approve, "/approvals", http.MethodPost, approvePath)
	j.assertLeaveStatus(t, leaveID, approvalID, "APPROVED", "APPROVED")
	if count := queryCount(t, j.db, "SELECT COUNT(*) FROM approval_requests WHERE id=$1 AND completed_at IS NOT NULL", approvalID); count != 1 {
		t.Fatalf("approval request %d has no completed_at", approvalID)
	}
	if count := queryCount(t, j.db, "SELECT COUNT(*) FROM approval_decisions WHERE request_id=$1 AND actor_id=$2 AND decision='APPROVE'", approvalID, j.managerUserID); count != 1 {
		t.Fatalf("approval request %d manager APPROVE decisions = %d, want 1", approvalID, count)
	}
	after := j.balance(t, year)
	if !hrSame(after.pending, before.pending) || !hrSame(after.used, before.used+days) || !hrSame(after.entitled, before.entitled) {
		t.Fatalf("balance after approval = %+v, want pending %.2f used %.2f (before %+v, days %.2f)", after, before.pending, before.used+days, before, days)
	}
	j.assertAudit(t, leaveID, "LEAVE_APPROVED", j.managerUserID)
	j.assertNotification(t, j.employeeUserID, "approval_approved", fmt.Sprintf("request:%d:status:APPROVED", approvalID))

	// A duplicate decision must not move the balance a second time.
	retry := j.manager.postForm(t, approvePath, url.Values{"csrf_token": {fetchCSRF(t, j.manager, "/approvals")}, "note": {"duplicate"}})
	hrAssertRedirect(t, retry, "/approvals", http.MethodPost, approvePath+" [duplicate]")
	if again := j.balance(t, year); again != after {
		t.Fatalf("duplicate approve changed balance from %+v to %+v", after, again)
	}
	if count := queryCount(t, j.db, "SELECT COUNT(*) FROM approval_decisions WHERE request_id=$1", approvalID); count != 1 {
		t.Fatalf("duplicate approve recorded %d decisions, want 1", count)
	}
	t.Logf("leave_request_id=%d approval_request_id=%d days=%.2f used %.2f->%.2f", leaveID, approvalID, days, before.used, after.used)
}

func (j *hrJourney) stepLeaveReject(t *testing.T) {
	days := hrDays(j.rejectStart, j.rejectEnd)
	year := j.rejectStart.Year()
	before := j.balance(t, year)
	leaveID, approvalID := j.submitLeave(t, "reject", j.rejectStart, j.rejectEnd)
	j.assertPendingSubmission(t, leaveID, approvalID, days, year, before)

	inbox := assertRoute(t, j.manager, "/approvals")
	if !bytes.Contains(inbox.body, []byte(fmt.Sprintf(`action="/approvals/%d/reject"`, approvalID))) {
		t.Fatalf("manager /approvals inbox does not offer rejection of approval request %d", approvalID)
	}
	rejectPath := fmt.Sprintf("/approvals/%d/reject", approvalID)
	reject := j.manager.postForm(t, rejectPath, url.Values{
		"csrf_token": {csrfFromBody(t, inbox.body, "/approvals")},
		"note":       {"E2E reject " + j.run},
	})
	hrAssertRedirect(t, reject, "/approvals", http.MethodPost, rejectPath)
	j.assertLeaveStatus(t, leaveID, approvalID, "REJECTED", "REJECTED")
	after := j.balance(t, year)
	if !hrSame(after.pending, before.pending) || !hrSame(after.used, before.used) {
		t.Fatalf("balance after rejection = %+v, want pending %.2f used %.2f", after, before.pending, before.used)
	}
	j.assertAudit(t, leaveID, "LEAVE_REJECTED", j.managerUserID)
	j.assertNotification(t, j.employeeUserID, "approval_rejected", fmt.Sprintf("request:%d:status:REJECTED", approvalID))
}

func (j *hrJourney) stepLeaveCancel(t *testing.T) {
	days := hrDays(j.cancelStart, j.cancelEnd)
	year := j.cancelStart.Year()
	before := j.balance(t, year)
	leaveID, approvalID := j.submitLeave(t, "cancel", j.cancelStart, j.cancelEnd)
	j.assertPendingSubmission(t, leaveID, approvalID, days, year, before)

	cancelPath := fmt.Sprintf("/hr/leave/%d/cancel", leaveID)
	cancel := j.employee.postForm(t, cancelPath, url.Values{"csrf_token": {fetchCSRF(t, j.employee, "/hr/leave")}})
	hrAssertRedirect(t, cancel, "/hr/leave?tab=my-requests", http.MethodPost, cancelPath)
	j.assertLeaveStatus(t, leaveID, approvalID, "CANCELLED", "CANCELLED")
	after := j.balance(t, year)
	if !hrSame(after.pending, before.pending) || !hrSame(after.used, before.used) {
		t.Fatalf("balance after cancellation = %+v, want pending %.2f used %.2f", after, before.pending, before.used)
	}
	j.assertAudit(t, leaveID, "LEAVE_CANCELLED", j.employeeUserID)
	inbox := assertRoute(t, j.manager, "/approvals")
	if bytes.Contains(inbox.body, []byte(fmt.Sprintf(`action="/approvals/%d/approve"`, approvalID))) {
		t.Fatalf("cancelled approval request %d is still actionable in the manager inbox", approvalID)
	}
}

func (j *hrJourney) stepAttendance(t *testing.T) {
	template := j.admin.get(t, "/hr/attendance/template")
	assertStatus(t, template, http.StatusOK, http.MethodGet, "/hr/attendance/template")
	if ct := template.header.Get("Content-Type"); !strings.HasPrefix(ct, "text/csv") {
		t.Fatalf("attendance template Content-Type = %q, want text/csv", ct)
	}
	header := strings.TrimSpace(strings.SplitN(string(template.body), "\n", 2)[0])
	if header != hrAttendanceTemplate {
		t.Fatalf("attendance template header = %q, want %q", header, hrAttendanceTemplate)
	}

	// Far-future dates derived from the run time keep reruns apart; every
	// assertion below is also correct if a date was used by an earlier run,
	// because rows are upserted and checked by value and import batch.
	base := time.Date(2100, time.January, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, int((time.Now().Unix()/10)%9000)*3)
	d1, d2, d3 := base.Format("2006-01-02"), base.AddDate(0, 0, 1).Format("2006-01-02"), base.AddDate(0, 0, 2).Format("2006-01-02")
	unknown := "E2E-UNKNOWN-" + j.run

	first := strings.Join([]string{
		hrAttendanceTemplate,
		fmt.Sprintf("%s,%s,%s 08:00,%s 17:00,PRESENT", j.employeeNumber, d1, d1, d1),
		fmt.Sprintf("%s,%s,,,ABSENT", j.employeeNumber, d2),
		fmt.Sprintf("%s,%s,,,PRESENT", unknown, d1),
		fmt.Sprintf("%s,%s,,,HOLIDAY", j.employeeNumber, d3),
	}, "\n") + "\n"
	firstID := j.importAttendance(t, "e2e-attendance-"+j.run+".csv", first, 4, 2, 2)
	errorsJSON := queryString(t, j.db, "SELECT errors::text FROM hr_attendance_imports WHERE id=$1", firstID)
	for _, want := range []string{"line 4: employee not found", "line 5: invalid time or status"} {
		if !strings.Contains(errorsJSON, want) {
			t.Fatalf("attendance import %d errors = %s, want entry %q", firstID, errorsJSON, want)
		}
	}
	j.assertAttendance(t, d1, hrAttendanceRow{status: "PRESENT", source: "CSV", checkIn: d1 + " 08:00", checkOut: d1 + " 17:00", importID: sql.NullInt64{Int64: firstID, Valid: true}})
	j.assertAttendance(t, d2, hrAttendanceRow{status: "ABSENT", source: "CSV", importID: sql.NullInt64{Int64: firstID, Valid: true}})
	if count := queryCount(t, j.db, "SELECT COUNT(*) FROM hr_attendance WHERE import_id=$1", firstID); count != 2 {
		t.Fatalf("attendance import %d wrote %d rows, want 2 (rejected rows must not be written)", firstID, count)
	}
	daily := assertRoute(t, j.admin, "/hr/attendance?tab=daily&date="+d1)
	if !bytes.Contains(daily.body, []byte(j.employeeNumber)) {
		t.Fatalf("attendance daily view for %s does not show employee %s", d1, j.employeeNumber)
	}

	second := strings.Join([]string{
		hrAttendanceTemplate,
		fmt.Sprintf("%s,%s,%s 08:30,%s 17:30,PRESENT", j.employeeNumber, d1, d1, d1),
		fmt.Sprintf("%s,%s,,,ABSENT", j.employeeNumber, d2),
	}, "\n") + "\n"
	secondID := j.importAttendance(t, "e2e-attendance-"+j.run+"-rerun.csv", second, 2, 2, 0)
	j.assertAttendance(t, d1, hrAttendanceRow{status: "PRESENT", source: "CSV", checkIn: d1 + " 08:30", checkOut: d1 + " 17:30", importID: sql.NullInt64{Int64: secondID, Valid: true}})
	j.assertAttendance(t, d2, hrAttendanceRow{status: "ABSENT", source: "CSV", importID: sql.NullInt64{Int64: secondID, Valid: true}})
	for _, date := range []string{d1, d2} {
		if count := queryCount(t, j.db, "SELECT COUNT(*) FROM hr_attendance WHERE employee_id=$1 AND attendance_date=$2::date", j.employeeID, date); count != 1 {
			t.Fatalf("attendance rows for employee %d on %s = %d after re-import, want 1", j.employeeID, date, count)
		}
	}

	if count := queryCount(t, j.db, "SELECT COUNT(*) FROM hr_attendance WHERE employee_id=$1 AND attendance_date=$2::date AND import_id IN ($3,$4)", j.employeeID, d3, firstID, secondID); count != 0 {
		t.Fatalf("invalid-status CSV row for %s was written by this run's imports", d3)
	}
	manual := j.admin.postForm(t, "/hr/attendance/record", url.Values{
		"csrf_token":  {fetchCSRF(t, j.admin, "/hr/attendance")},
		"employee_id": {strconv.FormatInt(j.employeeID, 10)},
		"date":        {d3},
		"check_in":    {"08:00"},
		"check_out":   {"17:00"},
		"status":      {"PRESENT"},
	})
	hrAssertRedirect(t, manual, "/hr/attendance?tab=daily&date="+d3, http.MethodPost, "/hr/attendance/record")
	manualRow := hrAttendanceRow{status: "PRESENT", source: "MANUAL", checkIn: d3 + " 08:00", checkOut: d3 + " 17:00"}
	j.assertAttendance(t, d3, manualRow)

	invalid := j.admin.postForm(t, "/hr/attendance/record", url.Values{
		"csrf_token":  {fetchCSRF(t, j.admin, "/hr/attendance")},
		"employee_id": {strconv.FormatInt(j.employeeID, 10)},
		"date":        {d3},
		"check_in":    {"17:00"},
		"check_out":   {"08:00"},
		"status":      {"PRESENT"},
	})
	hrAssertRedirect(t, invalid, "/hr/attendance?tab=daily&date="+d3, http.MethodPost, "/hr/attendance/record [check_out before check_in]")
	j.assertAttendance(t, d3, manualRow)
	t.Logf("attendance imports %d (4 rows: 2 accepted, 2 rejected) and %d (idempotent re-import); manual record on %s", firstID, secondID, d3)
}

func (j *hrJourney) importAttendance(t *testing.T, filename, content string, total, accepted, rejected int64) int64 {
	t.Helper()
	result := j.admin.postMultipartFile(t, "/hr/attendance/import", map[string]string{
		"csrf_token": fetchCSRF(t, j.admin, "/hr/attendance"),
	}, "file", filename, []byte(content))
	hrAssertRedirect(t, result, "/hr/attendance?tab=import", http.MethodPost, "/hr/attendance/import "+filename)
	if count := queryCount(t, j.db, "SELECT COUNT(*) FROM hr_attendance_imports WHERE company_id=$1 AND filename=$2", j.cfg.companyID, filename); count != 1 {
		t.Fatalf("attendance import batches for %s in company %d = %d, want 1", filename, j.cfg.companyID, count)
	}
	var id, gotTotal, gotAccepted, gotRejected int64
	if err := j.db.QueryRow(`SELECT id,total_rows,accepted_rows,rejected_rows FROM hr_attendance_imports WHERE company_id=$1 AND filename=$2`, j.cfg.companyID, filename).Scan(&id, &gotTotal, &gotAccepted, &gotRejected); err != nil {
		t.Fatalf("load attendance import %s: %v", filename, err)
	}
	if gotTotal != total || gotAccepted != accepted || gotRejected != rejected {
		t.Fatalf("attendance import %d (%s) total/accepted/rejected = %d/%d/%d, want %d/%d/%d", id, filename, gotTotal, gotAccepted, gotRejected, total, accepted, rejected)
	}
	return id
}

func (j *hrJourney) assertAttendance(t *testing.T, date string, want hrAttendanceRow) {
	t.Helper()
	var got hrAttendanceRow
	err := j.db.QueryRow(`
		SELECT status, source,
		       COALESCE(to_char(check_in AT TIME ZONE 'UTC','YYYY-MM-DD HH24:MI'),''),
		       COALESCE(to_char(check_out AT TIME ZONE 'UTC','YYYY-MM-DD HH24:MI'),''),
		       import_id
		FROM hr_attendance WHERE employee_id=$1 AND attendance_date=$2::date`, j.employeeID, date).
		Scan(&got.status, &got.source, &got.checkIn, &got.checkOut, &got.importID)
	if errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("no attendance row for employee %d on %s", j.employeeID, date)
	}
	if err != nil {
		t.Fatalf("load attendance for employee %d on %s: %v", j.employeeID, date, err)
	}
	if want.source == "MANUAL" {
		// Manual upserts leave import_id untouched; it is not part of the contract.
		got.importID = want.importID
	}
	if got != want {
		t.Fatalf("attendance for employee %d on %s = %+v, want %+v", j.employeeID, date, got, want)
	}
}

func (j *hrJourney) submitLeave(t *testing.T, label string, start, end time.Time) (int64, int64) {
	t.Helper()
	reason := "E2E " + label + " " + j.run
	result := j.employee.postForm(t, "/hr/leave", url.Values{
		"csrf_token":    {fetchCSRF(t, j.employee, "/hr/leave")},
		"leave_type_id": {strconv.FormatInt(j.leaveTypeID, 10)},
		"start_date":    {start.Format("2006-01-02")},
		"end_date":      {end.Format("2006-01-02")},
		"reason":        {reason},
	})
	hrAssertRedirect(t, result, "/hr/leave?tab=my-requests", http.MethodPost, "/hr/leave ["+label+"]")
	if count := queryCount(t, j.db, "SELECT COUNT(*) FROM hr_leave_requests WHERE employee_id=$1 AND reason=$2", j.employeeID, reason); count != 1 {
		t.Fatalf("leave request %q rows = %d, want 1; check the /hr/leave flash for the rejection", reason, count)
	}
	var leaveID int64
	var approvalID sql.NullInt64
	if err := j.db.QueryRow("SELECT id, approval_request_id FROM hr_leave_requests WHERE employee_id=$1 AND reason=$2", j.employeeID, reason).Scan(&leaveID, &approvalID); err != nil {
		t.Fatalf("load leave request %q: %v", reason, err)
	}
	if !approvalID.Valid {
		status := queryString(t, j.db, "SELECT status FROM hr_leave_requests WHERE id=$1", leaveID)
		t.Fatalf("leave request %d has status %s and no approval_request_id; approval submission failed", leaveID, status)
	}
	return leaveID, approvalID.Int64
}

func (j *hrJourney) assertPendingSubmission(t *testing.T, leaveID, approvalID int64, days float64, year int, before hrBalance) {
	t.Helper()
	j.assertLeaveStatus(t, leaveID, approvalID, "PENDING", "PENDING")
	if got := queryFloat64(t, j.db, "SELECT days::double precision FROM hr_leave_requests WHERE id=$1", leaveID); !hrSame(got, days) {
		t.Fatalf("leave request %d days = %.2f, want %.2f (inclusive calendar days)", leaveID, got, days)
	}
	var module string
	var documentID, companyID, requesterID, policyID int64
	var amount float64
	if err := j.db.QueryRow(`SELECT module, document_id, COALESCE(company_id,0), requester_id, policy_id, amount::double precision FROM approval_requests WHERE id=$1`, approvalID).
		Scan(&module, &documentID, &companyID, &requesterID, &policyID, &amount); err != nil {
		t.Fatalf("load approval request %d: %v", approvalID, err)
	}
	if module != "LEAVE" || documentID != leaveID || companyID != j.cfg.companyID || requesterID != j.employeeUserID || !hrSame(amount, days) {
		t.Fatalf("approval request %d = module %s document %d company %d requester %d amount %.2f; want LEAVE/%d/%d/%d/%.2f", approvalID, module, documentID, companyID, requesterID, amount, leaveID, j.cfg.companyID, j.employeeUserID, days)
	}
	if expected, ok := j.resolveLeavePolicy(t, days); !ok || policyID != expected {
		t.Fatalf("approval request %d used policy %d, want resolved policy %d", approvalID, policyID, expected)
	}
	if count := queryCount(t, j.db, "SELECT COUNT(*) FROM approval_assignments WHERE request_id=$1", approvalID); count != 1 {
		t.Fatalf("approval request %d has %d assignments, want 1", approvalID, count)
	}
	if count := queryCount(t, j.db, "SELECT COUNT(*) FROM approval_assignments WHERE request_id=$1 AND approver_id=$2 AND step_order=1 AND status='PENDING' AND delegated_from IS NULL", approvalID, j.managerUserID); count != 1 {
		t.Fatalf("approval request %d has no pending step-1 assignment for manager user %d", approvalID, j.managerUserID)
	}
	after := j.balance(t, year)
	if !hrSame(after.pending, before.pending+days) || !hrSame(after.used, before.used) {
		t.Fatalf("balance after submission = %+v, want pending %.2f used %.2f", after, before.pending+days, before.used)
	}
	j.assertAudit(t, leaveID, "LEAVE_REQUEST", j.employeeUserID)
	j.assertNotification(t, j.managerUserID, "approval_assigned", fmt.Sprintf("request:%d:step:1:user:%d", approvalID, j.managerUserID))
}

func (j *hrJourney) assertLeaveStatus(t *testing.T, leaveID, approvalID int64, leaveStatus, approvalStatus string) {
	t.Helper()
	if got := queryString(t, j.db, "SELECT status FROM hr_leave_requests WHERE id=$1", leaveID); got != leaveStatus {
		t.Fatalf("leave request %d status = %s, want %s", leaveID, got, leaveStatus)
	}
	if got := queryString(t, j.db, "SELECT status FROM approval_requests WHERE id=$1", approvalID); got != approvalStatus {
		t.Fatalf("approval request %d status = %s, want %s", approvalID, got, approvalStatus)
	}
}

func (j *hrJourney) assertAudit(t *testing.T, leaveID int64, action string, actorID int64) {
	t.Helper()
	count := queryCount(t, j.db, "SELECT COUNT(*) FROM audit_logs WHERE entity='hr_leave_request' AND entity_id=$1 AND action=$2 AND actor_id=$3", strconv.FormatInt(leaveID, 10), action, actorID)
	if count != 1 {
		t.Fatalf("audit_logs %s rows for leave request %d by user %d = %d, want 1", action, leaveID, actorID, count)
	}
}

func (j *hrJourney) assertNotification(t *testing.T, recipientID int64, kind, dedupeKey string) {
	t.Helper()
	if count := queryCount(t, j.db, "SELECT COUNT(*) FROM notifications WHERE recipient_id=$1 AND type=$2 AND dedupe_key=$3", recipientID, kind, dedupeKey); count != 1 {
		t.Fatalf("notifications %s for user %d with dedupe key %q = %d, want 1", kind, recipientID, dedupeKey, count)
	}
}

func (j *hrJourney) balance(t *testing.T, year int) hrBalance {
	t.Helper()
	var b hrBalance
	err := j.db.QueryRow(`SELECT entitled::double precision, used::double precision, pending::double precision FROM hr_leave_balances WHERE employee_id=$1 AND leave_type_id=$2 AND year=$3`, j.employeeID, j.leaveTypeID, year).Scan(&b.entitled, &b.used, &b.pending)
	if err != nil {
		t.Fatalf("load leave balance employee %d type %d year %d: %v", j.employeeID, j.leaveTypeID, year, err)
	}
	return b
}

// resolveLeavePolicy mirrors approvals.Repository.ResolvePolicy: exact company
// before company-neutral, then the highest matching minimum, then newest.
func (j *hrJourney) resolveLeavePolicy(t *testing.T, amount float64) (int64, bool) {
	t.Helper()
	var id int64
	err := j.db.QueryRow(`
		SELECT id FROM approval_policies
		WHERE module='LEAVE' AND is_active=TRUE
		  AND (company_id=$1 OR company_id IS NULL)
		  AND min_amount <= $2::numeric AND (max_amount IS NULL OR max_amount >= $2::numeric)
		ORDER BY (company_id IS NOT NULL) DESC, min_amount DESC, id DESC
		LIMIT 1`, j.cfg.companyID, strconv.FormatFloat(amount, 'f', 2, 64)).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false
	}
	if err != nil {
		t.Fatalf("resolve LEAVE policy for %.2f days: %v", amount, err)
	}
	return id, true
}

func hrEmployeeByUser(t *testing.T, db *sql.DB, userID int64) (hrEmployeeRow, bool) {
	t.Helper()
	var row hrEmployeeRow
	err := db.QueryRow(`
		SELECT id, company_id, employee_number, name, email, to_char(hire_date,'YYYY-MM-DD'), status,
		       manager_id, department_id, position_id, user_id
		FROM hr_employees WHERE user_id=$1`, userID).
		Scan(&row.id, &row.companyID, &row.number, &row.name, &row.email, &row.hireDate, &row.status,
			&row.managerID, &row.departmentID, &row.positionID, &row.uid)
	if errors.Is(err, sql.ErrNoRows) {
		return hrEmployeeRow{}, false
	}
	if err != nil {
		t.Fatalf("load hr_employees for user %d: %v", userID, err)
	}
	return row, true
}

func hrUserID(t *testing.T, db *sql.DB, email, envKey string) int64 {
	t.Helper()
	var id int64
	err := db.QueryRow("SELECT id FROM users WHERE lower(email)=lower($1) AND is_active", email).Scan(&id)
	if err != nil {
		t.Fatalf("precondition: %s does not resolve to an active user: %v", envKey, err)
	}
	return id
}

func hrAssertRedirect(t *testing.T, result httpResult, wantLocation, method, path string) {
	t.Helper()
	assertStatus(t, result, http.StatusSeeOther, method, path)
	if result.location != wantLocation {
		t.Fatalf("%s %s redirected to %q, want %q", method, path, result.location, wantLocation)
	}
}

// hrDays matches leave.Service.Submit: inclusive calendar days.
func hrDays(start, end time.Time) float64 {
	return end.Sub(start).Hours()/24 + 1
}

func hrNextMonday(from time.Time, minDays int) time.Time {
	day := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, minDays)
	for day.Weekday() != time.Monday {
		day = day.AddDate(0, 0, 1)
	}
	return day
}

func hrSame(a, b float64) bool { return math.Abs(a-b) < 0.005 }
