package employees

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/odyssey-erp/odyssey-erp/internal/hr/compensation"
	"github.com/odyssey-erp/odyssey-erp/internal/rbac"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/view"
)

type permissionReader struct{ permissions []string }

func (p permissionReader) EffectivePermissions(context.Context, int64) ([]string, error) {
	return p.permissions, nil
}

const (
	testNationalID = "3171999900001111"
	testTaxID      = "09.254.294.3-407.000"
	testAddress    = "Jl. Sudirman Kav 52"
	testSalary     = "8500000.00"
)

// fakeEmployees is an in-memory EmployeeService.
type fakeEmployees struct {
	employee Employee
	updates  []UpdateInput
	created  []CreateInput
	changes  []JobChangeInput
	contacts []ContactInput
	deleted  []int64
	// history overrides ListJobHistory (newest first) when set.
	history []JobHistory
	// combined records UpdateWithJobChange calls; combinedErr fails them.
	combined    []combinedCall
	combinedErr error
}

type combinedCall struct {
	update UpdateInput
	job    *JobChangeInput
}

func newFakeEmployees() *fakeEmployees {
	dob := time.Date(1990, 4, 2, 0, 0, 0, 0, time.UTC)
	dept := int64(3)
	return &fakeEmployees{employee: Employee{
		ID: 8, CompanyID: 1, EmployeeNumber: "E-008", Name: "Ada Lovelace", Email: "ada@example.com",
		DepartmentID: &dept, Department: "Finance", Status: StatusActive, EmploymentType: EmploymentPermanent,
		HireDate: time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
		Profile: Profile{
			DateOfBirth: &dob, NationalID: testNationalID, TaxID: testTaxID, Phone: "0812-555",
			AddressLine1: testAddress, City: "Jakarta", Country: "ID", Gender: "FEMALE",
		},
	}}
}

func (f *fakeEmployees) List(context.Context, int64, ...EmployeeFilter) ([]Employee, error) {
	return []Employee{f.employee}, nil
}

func (f *fakeEmployees) GetByID(_ context.Context, companyID, id int64) (Employee, error) {
	if companyID != f.employee.CompanyID || id != f.employee.ID {
		return Employee{}, ErrEmployeeNotFound
	}
	return f.employee, nil
}

func (f *fakeEmployees) Create(_ context.Context, in CreateInput) (Employee, error) {
	f.created = append(f.created, in)
	return Employee{ID: 9}, nil
}

func (f *fakeEmployees) Update(_ context.Context, _, _ int64, in UpdateInput) (Employee, error) {
	f.updates = append(f.updates, in)
	return f.employee, nil
}

func (f *fakeEmployees) ListDepartments(context.Context, int64) ([]Department, error) {
	return []Department{{ID: 3, Code: "FIN", Name: "Finance"}}, nil
}

func (f *fakeEmployees) ListPositions(context.Context, int64) ([]Position, error) {
	return []Position{{ID: 4, Code: "CTRL", Name: "Controller"}}, nil
}

func (f *fakeEmployees) ListAvailableUsers(context.Context) ([]UserOption, error) { return nil, nil }

func (f *fakeEmployees) CreateDepartment(context.Context, int64, string, string) (Department, error) {
	return Department{}, nil
}

func (f *fakeEmployees) CreatePosition(context.Context, int64, string, string) (Position, error) {
	return Position{}, nil
}

func (f *fakeEmployees) ListContacts(context.Context, int64, int64) ([]EmergencyContact, error) {
	return []EmergencyContact{{ID: 5, EmployeeID: 8, Name: "Charles Babbage", Relationship: "Friend", Phone: "0813-777", IsPrimary: true}}, nil
}

func (f *fakeEmployees) UpsertContact(_ context.Context, _, _ int64, in ContactInput) (EmergencyContact, error) {
	f.contacts = append(f.contacts, in)
	return EmergencyContact{ID: 6}, nil
}

func (f *fakeEmployees) DeleteContact(_ context.Context, _, _, contactID, _ int64) error {
	f.deleted = append(f.deleted, contactID)
	return nil
}

func (f *fakeEmployees) UpdateWithJobChange(_ context.Context, _, _ int64, in UpdateInput, job *JobChangeInput) (Employee, error) {
	if f.combinedErr != nil {
		return Employee{}, f.combinedErr
	}
	f.combined = append(f.combined, combinedCall{update: in, job: job})
	return f.employee, nil
}

func (f *fakeEmployees) ListJobHistory(context.Context, int64, int64) ([]JobHistory, error) {
	if f.history != nil {
		return f.history, nil
	}
	return []JobHistory{{ID: 1, EmployeeID: 8, Department: "Finance", Status: StatusActive, EmploymentType: EmploymentPermanent,
		ValidFrom: time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC), Reason: "Hired"}}, nil
}

func (f *fakeEmployees) AssignmentAsOf(context.Context, int64, int64, time.Time) (JobHistory, error) {
	return JobHistory{}, nil
}

func (f *fakeEmployees) RecordJobChange(_ context.Context, _, _ int64, in JobChangeInput) (JobHistory, error) {
	f.changes = append(f.changes, in)
	return JobHistory{}, nil
}

// fakeCompensation records calls so tests can prove it is never touched
// without permission.
type fakeCompensation struct {
	listCalls int
	changes   []compensation.ChangeInput
	banks     []compensation.BankDetails
}

func (f *fakeCompensation) ListForEmployee(context.Context, int64, int64) ([]compensation.Assignment, error) {
	f.listCalls++
	return []compensation.Assignment{{ID: 2, EmployeeID: 8, BaseSalary: testSalary, PTKPCode: "TK/0", BPJSHealth: true,
		BankCode: "BCA", BankAccountNumber: "1234567890", EffectiveFrom: time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC), Reason: "Hired"}}, nil
}

func (f *fakeCompensation) Change(_ context.Context, _, _ int64, in compensation.ChangeInput) (compensation.Assignment, error) {
	f.changes = append(f.changes, in)
	return compensation.Assignment{ID: 3}, nil
}

func (f *fakeCompensation) ChangeBank(_ context.Context, _, _ int64, bank compensation.BankDetails, _ time.Time, _ string, _ int64) (compensation.Assignment, error) {
	f.banks = append(f.banks, bank)
	return compensation.Assignment{ID: 4}, nil
}

func employeeRequest(t *testing.T, method, path string, form url.Values) *http.Request {
	t.Helper()
	manager := shared.NewSessionManager(nil, "session", "secret", time.Hour, false)
	body := strings.NewReader("")
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req := httptest.NewRequest(method, path, body)
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	session, err := manager.Load(req.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	session.SetUser("3")
	session.Set("company_id", "1")
	return req.WithContext(shared.ContextWithSession(req.Context(), session))
}

func newTestRouter(t *testing.T, perms []string, emp *fakeEmployees, comp *fakeCompensation, withTemplates bool) http.Handler {
	t.Helper()
	var engine *view.Engine
	if withTemplates {
		var err error
		engine, err = view.NewEngine()
		if err != nil {
			t.Fatal(err)
		}
	}
	h := NewHandler(nil, emp, comp, engine, shared.NewCSRFManager("secret"), rbac.Middleware{Service: permissionReader{perms}})
	router := chi.NewRouter()
	router.Route("/hr/employees", h.MountRoutes)
	return router
}

func TestEmployeeRoutePermissions(t *testing.T) {
	salaryForm := url.Values{"effective_from": {"2026-11-01"}, "base_salary": {"9000000"}, "ptkp_code": {"TK/0"}, "reason": {"Review"}}
	cases := []struct {
		name        string
		perms       []string
		method      string
		path        string
		form        url.Values
		wantStatus  int
		wantChanges int
	}{
		{"compensation POST without hr.compensation.admin is forbidden", []string{shared.PermHREmployeeAdmin, shared.PermHRCompensationView, shared.PermHREmployeeSensitive}, http.MethodPost, "/hr/employees/8/compensation", salaryForm, http.StatusForbidden, 0},
		{"compensation POST with hr.compensation.admin succeeds", []string{shared.PermHRCompensationAdmin}, http.MethodPost, "/hr/employees/8/compensation", salaryForm, http.StatusSeeOther, 1},
		{"profile POST requires hr.employee.admin", []string{shared.PermHREmployeeView}, http.MethodPost, "/hr/employees/8/profile", url.Values{"name": {"x"}}, http.StatusForbidden, 0},
		{"contact POST requires hr.employee.admin", []string{shared.PermHREmployeeView}, http.MethodPost, "/hr/employees/8/contacts", url.Values{"name": {"x"}}, http.StatusForbidden, 0},
		{"contact delete requires hr.employee.admin", []string{shared.PermHREmployeeView}, http.MethodPost, "/hr/employees/8/contacts/5/delete", url.Values{}, http.StatusForbidden, 0},
		{"job history POST requires hr.employee.admin", []string{shared.PermHREmployeeView, shared.PermHRCompensationAdmin}, http.MethodPost, "/hr/employees/8/job-history", url.Values{"valid_from": {"2026-11-01"}}, http.StatusForbidden, 0},
		{"detail GET requires employee view or admin", []string{shared.PermHRCompensationView}, http.MethodGet, "/hr/employees/8", nil, http.StatusForbidden, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			emp, comp := newFakeEmployees(), &fakeCompensation{}
			rec := httptest.NewRecorder()
			newTestRouter(t, tc.perms, emp, comp, false).ServeHTTP(rec, employeeRequest(t, tc.method, tc.path, tc.form))
			if rec.Code != tc.wantStatus {
				t.Fatalf("status=%d want %d body=%s", rec.Code, tc.wantStatus, rec.Body.String())
			}
			if len(comp.changes) != tc.wantChanges {
				t.Fatalf("compensation changes=%d want %d", len(comp.changes), tc.wantChanges)
			}
			if tc.wantStatus == http.StatusForbidden && (len(emp.updates)+len(emp.contacts)+len(emp.deleted)+len(emp.changes)) != 0 {
				t.Fatal("forbidden request reached the employee service")
			}
		})
	}
}

func TestEmployeeDetailMasksAndOmits(t *testing.T) {
	cases := []struct {
		name             string
		perms            []string
		wantSensitive    bool
		wantCompensation bool
	}{
		{"view only: sensitive masked, compensation omitted", []string{shared.PermHREmployeeView}, false, false},
		{"admin without sensitive or compensation", []string{shared.PermHREmployeeAdmin}, false, false},
		{"sensitive only", []string{shared.PermHREmployeeView, shared.PermHREmployeeSensitive}, true, false},
		{"compensation view only", []string{shared.PermHREmployeeView, shared.PermHRCompensationView}, false, true},
		{"full access", []string{shared.PermHREmployeeAdmin, shared.PermHREmployeeSensitive, shared.PermHRCompensationAdmin}, true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			emp, comp := newFakeEmployees(), &fakeCompensation{}
			rec := httptest.NewRecorder()
			newTestRouter(t, tc.perms, emp, comp, true).ServeHTTP(rec, employeeRequest(t, http.MethodGet, "/hr/employees/8", nil))
			if rec.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
			body := rec.Body.String()
			for _, secret := range []string{testNationalID, testTaxID, testAddress} {
				if got := strings.Contains(body, secret); got != tc.wantSensitive {
					t.Fatalf("body contains %q = %v, want %v", secret, got, tc.wantSensitive)
				}
			}
			// Date of birth renders as a form value (admin) or display date.
			dob := strings.Contains(body, "1990-04-02") || strings.Contains(body, "02 Apr 1990")
			if dob != tc.wantSensitive {
				t.Fatalf("body contains date of birth = %v, want %v", dob, tc.wantSensitive)
			}
			if got := strings.Contains(body, testSalary); got != tc.wantCompensation {
				t.Fatalf("body contains salary = %v, want %v", got, tc.wantCompensation)
			}
			if got := strings.Contains(body, `data-tab="compensation"`); got != tc.wantCompensation {
				t.Fatalf("compensation tab rendered = %v, want %v", got, tc.wantCompensation)
			}
			if !tc.wantCompensation && comp.listCalls != 0 {
				t.Fatal("compensation data loaded without permission")
			}
		})
	}
}

// TestDetailDataMasksServerSide checks the view model itself, independent of
// the template's own CanSeeSensitive/CanSeeCompensation guards.
func TestDetailDataMasksServerSide(t *testing.T) {
	cases := []struct {
		name             string
		perms            accessFlags
		wantSensitive    bool
		wantCompensation bool
	}{
		{"no sensitive, no compensation", accessFlags{employeeAdmin: true}, false, false},
		{"sensitive only", accessFlags{sensitive: true}, true, false},
		{"compensation view", accessFlags{compensationView: true}, false, true},
		{"compensation admin implies view", accessFlags{compensationAdmin: true}, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			emp, comp := newFakeEmployees(), &fakeCompensation{}
			h := NewHandler(nil, emp, comp, nil, nil, rbac.Middleware{})
			data := h.detailData(employeeRequest(t, http.MethodGet, "/hr/employees/8", nil), 1, tc.perms, emp.employee)
			got := data["Employee"].(Employee)
			hasSensitive := got.NationalID != "" || got.TaxID != "" || got.DateOfBirth != nil || got.AddressLine1 != "" || got.City != "" || got.Country != ""
			if hasSensitive != tc.wantSensitive {
				t.Fatalf("sensitive present = %v, want %v (%+v)", hasSensitive, tc.wantSensitive, got.Profile)
			}
			if got.Phone == "" || got.Gender == "" {
				t.Fatal("non-sensitive profile fields must not be masked")
			}
			_, hasComp := data["Compensation"]
			if hasComp != tc.wantCompensation || (comp.listCalls > 0) != tc.wantCompensation {
				t.Fatalf("compensation in model = %v (list calls %d), want %v", hasComp, comp.listCalls, tc.wantCompensation)
			}
		})
	}
}

func TestEmployeeDetailNotFound(t *testing.T) {
	rec := httptest.NewRecorder()
	newTestRouter(t, []string{shared.PermHREmployeeView}, newFakeEmployees(), &fakeCompensation{}, true).
		ServeHTTP(rec, employeeRequest(t, http.MethodGet, "/hr/employees/99", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status=%d want 404", rec.Code)
	}
}

func TestUpdateProfile(t *testing.T) {
	form := url.Values{
		"employee_number": {"E-008"}, "name": {"Ada King"}, "email": {"ada@example.com"}, "hire_date": {"2024-01-15"},
		"phone": {"0812-000"}, "gender": {"FEMALE"}, "marital_status": {"MARRIED"}, "dependents_count": {"2"},
		"contract_start_date": {"2024-01-15"}, "national_id": {"NEW-ID"}, "date_of_birth": {"1990-04-02"},
	}
	cases := []struct {
		name          string
		perms         []string
		wantSensitive bool
	}{
		{"admin without sensitive keeps stored sensitive values", []string{shared.PermHREmployeeAdmin}, false},
		{"admin with sensitive writes sensitive values", []string{shared.PermHREmployeeAdmin, shared.PermHREmployeeSensitive}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			emp := newFakeEmployees()
			rec := httptest.NewRecorder()
			newTestRouter(t, tc.perms, emp, &fakeCompensation{}, false).ServeHTTP(rec, employeeRequest(t, http.MethodPost, "/hr/employees/8/profile", form))
			if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/hr/employees/8?tab=profile" {
				t.Fatalf("status=%d location=%q", rec.Code, rec.Header().Get("Location"))
			}
			if len(emp.updates) != 1 {
				t.Fatalf("updates=%d want 1", len(emp.updates))
			}
			in := emp.updates[0]
			if in.Name != "Ada King" || in.Phone != "0812-000" || in.MaritalStatus != "MARRIED" || in.DependentsCount != 2 || in.ContractStartDate == nil || in.ActorID != 3 {
				t.Fatalf("unexpected update input: %+v", in)
			}
			if in.IncludeSensitive != tc.wantSensitive {
				t.Fatalf("IncludeSensitive=%v want %v", in.IncludeSensitive, tc.wantSensitive)
			}
		})
	}
}

func TestDetailWritesRedirectToTabs(t *testing.T) {
	perms := []string{shared.PermHREmployeeAdmin, shared.PermHRCompensationAdmin}
	cases := []struct {
		path     string
		form     url.Values
		location string
		check    func(*fakeEmployees, *fakeCompensation) bool
	}{
		{"/hr/employees/8/contacts", url.Values{"name": {"Mary"}, "is_primary": {"on"}}, "/hr/employees/8?tab=contacts",
			func(e *fakeEmployees, _ *fakeCompensation) bool {
				return len(e.contacts) == 1 && e.contacts[0].IsPrimary && e.contacts[0].Name == "Mary"
			}},
		{"/hr/employees/8/contacts/5/delete", url.Values{}, "/hr/employees/8?tab=contacts",
			func(e *fakeEmployees, _ *fakeCompensation) bool { return len(e.deleted) == 1 && e.deleted[0] == 5 }},
		{"/hr/employees/8/job-history", url.Values{"valid_from": {"2026-11-01"}, "department_id": {"3"}, "status": {"ACTIVE"}, "reason": {"Promotion"}}, "/hr/employees/8?tab=job",
			func(e *fakeEmployees, _ *fakeCompensation) bool {
				return len(e.changes) == 1 && e.changes[0].Reason == "Promotion" && e.changes[0].DepartmentID != nil
			}},
		{"/hr/employees/8/job-history", url.Values{"reason": {"no date"}}, "/hr/employees/8?tab=job",
			func(e *fakeEmployees, _ *fakeCompensation) bool { return len(e.changes) == 0 }},
		{"/hr/employees/8/compensation", url.Values{"change_type": {"bank"}, "effective_from": {"2026-11-01"}, "bank_code": {"BNI"}, "bank_account_number": {"99"}}, "/hr/employees/8?tab=compensation",
			func(_ *fakeEmployees, c *fakeCompensation) bool {
				return len(c.banks) == 1 && c.banks[0].BankCode == "BNI" && len(c.changes) == 0
			}},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			emp, comp := newFakeEmployees(), &fakeCompensation{}
			rec := httptest.NewRecorder()
			newTestRouter(t, perms, emp, comp, false).ServeHTTP(rec, employeeRequest(t, http.MethodPost, tc.path, tc.form))
			if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != tc.location {
				t.Fatalf("status=%d location=%q", rec.Code, rec.Header().Get("Location"))
			}
			if !tc.check(emp, comp) {
				t.Fatalf("service calls not as expected: emp=%+v comp=%+v", emp, comp)
			}
		})
	}
}

func TestCreateInitialCompensationRequiresPermission(t *testing.T) {
	form := url.Values{"employee_number": {"E-009"}, "name": {"Grace"}, "email": {"g@example.com"}, "hire_date": {"2026-11-01"},
		"employment_type": {"CONTRACT"}, "contract_start_date": {"2026-11-01"}, "contract_end_date": {"2027-10-31"},
		"base_salary": {"7000000"}, "ptkp_code": {"TK/0"}}
	cases := []struct {
		name        string
		perms       []string
		wantCreated int
		wantComp    bool
	}{
		{"without hr.compensation.admin the create is refused", []string{shared.PermHREmployeeAdmin}, 0, false},
		{"with hr.compensation.admin the initial compensation is passed", []string{shared.PermHREmployeeAdmin, shared.PermHRCompensationAdmin}, 1, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			emp := newFakeEmployees()
			rec := httptest.NewRecorder()
			newTestRouter(t, tc.perms, emp, &fakeCompensation{}, false).ServeHTTP(rec, employeeRequest(t, http.MethodPost, "/hr/employees/", form))
			if rec.Code != http.StatusSeeOther {
				t.Fatalf("status=%d", rec.Code)
			}
			if len(emp.created) != tc.wantCreated {
				t.Fatalf("created=%d want %d", len(emp.created), tc.wantCreated)
			}
			if tc.wantCreated == 1 {
				in := emp.created[0]
				if (in.Compensation != nil) != tc.wantComp || in.Compensation.BaseSalary != "7000000" || in.EmploymentType != "CONTRACT" || in.ContractEndDate == nil {
					t.Fatalf("unexpected create input: %+v", in)
				}
			}
		})
	}
}

func TestEmployeeDetailTemplateRenders(t *testing.T) {
	emp, comp := newFakeEmployees(), &fakeCompensation{}
	rec := httptest.NewRecorder()
	perms := []string{shared.PermHREmployeeAdmin, shared.PermHREmployeeSensitive, shared.PermHRCompensationAdmin}
	newTestRouter(t, perms, emp, comp, true).ServeHTTP(rec, employeeRequest(t, http.MethodGet, "/hr/employees/8?tab=job", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		"Ada Lovelace", "E-008", "Profile", "Emergency contacts", "Job history", "Compensation",
		"Charles Babbage", "Hired", testSalary, testNationalID,
		`action="/hr/employees/8/profile"`, `action="/hr/employees/8/contacts"`, `action="/hr/employees/8/contacts/5/delete"`,
		`action="/hr/employees/8/job-history"`, `action="/hr/employees/8/compensation"`, `name="csrf_token"`,
		`data-tab-panel="job" class="tab-panel active"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("rendered page missing %q", want)
		}
	}
}

func TestEmployeeListTemplateLinksAndCompensationFields(t *testing.T) {
	cases := []struct {
		name     string
		perms    []string
		wantComp bool
	}{
		{"employee admin only", []string{shared.PermHREmployeeAdmin}, false},
		{"with compensation admin", []string{shared.PermHREmployeeAdmin, shared.PermHRCompensationAdmin}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			newTestRouter(t, tc.perms, newFakeEmployees(), &fakeCompensation{}, true).ServeHTTP(rec, employeeRequest(t, http.MethodGet, "/hr/employees/", nil))
			if rec.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
			body := rec.Body.String()
			for _, want := range []string{`href="/hr/employees/8"`, `name="employment_type"`, `name="contract_start_date"`, `name="probation_end_date"`} {
				if !strings.Contains(body, want) {
					t.Fatalf("list page missing %q", want)
				}
			}
			if got := strings.Contains(body, `name="base_salary"`); got != tc.wantComp {
				t.Fatalf("initial compensation fields rendered = %v, want %v", got, tc.wantComp)
			}
			if strings.Contains(body, testNationalID) {
				t.Fatal("directory must never render sensitive fields")
			}
		})
	}
}

func directoryUpdateForm(extra url.Values) url.Values {
	form := url.Values{"employee_number": {"E-008"}, "name": {"Ada Lovelace"}, "email": {"ada@example.com"}, "hire_date": {"2024-01-15"}}
	for k, v := range extra {
		form[k] = v
	}
	return form
}

func TestDirectoryUpdateJobChangeDefaultsEffectiveDate(t *testing.T) {
	today := businessToday()
	future := today.AddDate(0, 0, 20)
	tests := []struct {
		name    string
		history []JobHistory
		form    url.Values
		want    time.Time
	}{
		{"defaults to today when the latest record started earlier", []JobHistory{{ValidFrom: day(2024, 1, 15)}}, url.Values{"department_id": {"4"}}, today},
		{"never before a pre-boarded hire's future record", []JobHistory{{ValidFrom: future}}, url.Values{"department_id": {"4"}}, future},
		{"same-day record keeps today", []JobHistory{{ValidFrom: today}}, url.Values{"department_id": {"4"}}, today},
		{"explicit effective date wins", []JobHistory{{ValidFrom: day(2024, 1, 15)}}, url.Values{"department_id": {"4"}, "effective_date": {"2030-02-01"}}, day(2030, 2, 1)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			emp := newFakeEmployees()
			emp.history = tc.history
			rec := httptest.NewRecorder()
			newTestRouter(t, []string{shared.PermHREmployeeAdmin}, emp, &fakeCompensation{}, false).
				ServeHTTP(rec, employeeRequest(t, http.MethodPost, "/hr/employees/8/update", directoryUpdateForm(tc.form)))
			if rec.Code != http.StatusSeeOther {
				t.Fatalf("status=%d", rec.Code)
			}
			if len(emp.combined) != 1 || emp.combined[0].job == nil {
				t.Fatalf("combined calls=%+v, want one with a job change", emp.combined)
			}
			if got := emp.combined[0].job.ValidFrom; !got.Equal(tc.want) {
				t.Fatalf("ValidFrom=%s want %s", got, tc.want)
			}
			if len(emp.updates) != 0 || len(emp.changes) != 0 {
				t.Fatalf("profile update and job change must go through the single atomic call, got updates=%d changes=%d", len(emp.updates), len(emp.changes))
			}
		})
	}
}

func TestDirectoryUpdateRefusesToOverwriteScheduledChange(t *testing.T) {
	today := businessToday()
	scheduled := today.AddDate(0, 0, 20)
	history := []JobHistory{
		{ID: 2, ValidFrom: scheduled, Reason: "Promotion"},           // scheduled, not yet in effect
		{ID: 1, ValidFrom: today.AddDate(-1, 0, 0), Reason: "Hired"}, // current
	}
	run := func(form url.Values) (*fakeEmployees, string) {
		emp := newFakeEmployees()
		emp.history = history
		req := employeeRequest(t, http.MethodPost, "/hr/employees/8/update", directoryUpdateForm(form))
		rec := httptest.NewRecorder()
		newTestRouter(t, []string{shared.PermHREmployeeAdmin}, emp, &fakeCompensation{}, false).ServeHTTP(rec, req)
		if rec.Code != http.StatusSeeOther {
			t.Fatalf("status=%d", rec.Code)
		}
		msg := ""
		if f := shared.SessionFromContext(req.Context()).PopFlash(); f != nil {
			msg = f.Kind + ": " + f.Message
		}
		return emp, msg
	}

	emp, msg := run(url.Values{"department_id": {"4"}})
	want := "error: A job change is scheduled for " + scheduled.Format("2006-01-02") + "; edit it from the employee's Job tab"
	if msg != want {
		t.Fatalf("flash=%q want %q", msg, want)
	}
	if len(emp.combined) != 0 || len(emp.updates) != 0 || len(emp.changes) != 0 {
		t.Fatalf("nothing may be written: combined=%d updates=%d changes=%d", len(emp.combined), len(emp.updates), len(emp.changes))
	}

	// Without job-field changes the profile edit still saves.
	emp, msg = run(url.Values{"department_id": {"3"}})
	if len(emp.updates) != 1 || msg != "success: Employee updated successfully" {
		t.Fatalf("updates=%d flash=%q", len(emp.updates), msg)
	}

	// An explicit date equal to the scheduled start would correct it in place
	// with mirror values, so it is refused too.
	emp, msg = run(url.Values{"department_id": {"4"}, "effective_date": {scheduled.Format("2006-01-02")}})
	if msg != want || len(emp.combined) != 0 || len(emp.updates) != 0 || len(emp.changes) != 0 {
		t.Fatalf("flash=%q combined=%d updates=%d changes=%d", msg, len(emp.combined), len(emp.updates), len(emp.changes))
	}

	// Any other explicit effective date is an informed choice and is passed through.
	emp, _ = run(url.Values{"department_id": {"4"}, "effective_date": {today.Format("2006-01-02")}})
	if len(emp.combined) != 1 || !emp.combined[0].job.ValidFrom.Equal(today) {
		t.Fatalf("combined=%+v", emp.combined)
	}
}

func TestDirectoryUpdateWithoutJobFieldChangesOnlyUpdates(t *testing.T) {
	emp := newFakeEmployees()
	rec := httptest.NewRecorder()
	newTestRouter(t, []string{shared.PermHREmployeeAdmin}, emp, &fakeCompensation{}, false).
		ServeHTTP(rec, employeeRequest(t, http.MethodPost, "/hr/employees/8/update", directoryUpdateForm(url.Values{"department_id": {"3"}})))
	if rec.Code != http.StatusSeeOther || len(emp.updates) != 1 || len(emp.combined) != 0 {
		t.Fatalf("status=%d updates=%d combined=%d", rec.Code, len(emp.updates), len(emp.combined))
	}
}

func TestDirectoryUpdateFailureIsReportedWithoutSeparateCommit(t *testing.T) {
	emp := newFakeEmployees()
	emp.combinedErr = errors.New("hr: job change must start after 2026-01-01 (the latest job record)")
	rec := httptest.NewRecorder()
	newTestRouter(t, []string{shared.PermHREmployeeAdmin}, emp, &fakeCompensation{}, false).
		ServeHTTP(rec, employeeRequest(t, http.MethodPost, "/hr/employees/8/update", directoryUpdateForm(url.Values{"department_id": {"4"}})))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status=%d", rec.Code)
	}
	if len(emp.updates) != 0 {
		t.Fatalf("a failed combined update must not have saved the profile separately, updates=%d", len(emp.updates))
	}
}
