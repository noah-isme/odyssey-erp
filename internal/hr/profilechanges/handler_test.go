package profilechanges

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/odyssey-erp/odyssey-erp/internal/hr/compensation"
	"github.com/odyssey-erp/odyssey-erp/internal/hr/employees"
	"github.com/odyssey-erp/odyssey-erp/internal/rbac"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/view"
)

type permissionReader struct{ permissions []string }

func (p permissionReader) EffectivePermissions(context.Context, int64) ([]string, error) {
	return p.permissions, nil
}

// fakeProfileChanges returns unmasked payloads regardless of the filter so
// the tests also cover the handler's own masking.
type fakeProfileChanges struct {
	filters  []Filter
	retried  []int64
	retryErr error
}

func (f *fakeProfileChanges) ListAll(_ context.Context, _ int64, filter Filter) ([]Request, error) {
	f.filters = append(f.filters, filter)
	approval := int64(61)
	created := time.Date(2026, 10, 3, 9, 0, 0, 0, time.UTC)
	return []Request{
		{ID: 11, EmployeeID: 7, EmployeeName: "Ada Lovelace", EmployeeNumber: "E-007", ChangeType: TypeAddress,
			Payload: map[string]string{"address_line1": "Jl. Baru 9", "city": "Bandung", "phone": "0812-NEW"},
			Status:  StatusPending, ApprovalRequestID: &approval, ApplyError: "employee row locked", CreatedAt: created},
		{ID: 12, EmployeeID: 7, EmployeeName: "Ada Lovelace", EmployeeNumber: "E-007", ChangeType: TypeBank,
			Payload: map[string]string{"bank_code": "BCA", "bank_account_number": "9988776655", "bank_account_name": "Ada L"},
			Status:  StatusPending, ApprovalRequestID: &approval, CreatedAt: created},
		{ID: 13, EmployeeID: 7, EmployeeName: "Ada Lovelace", EmployeeNumber: "E-007", ChangeType: TypeEmergencyContact,
			Payload: map[string]string{"contact_id": "5", "name": "Byron", "relationship": "Father", "phone": "0811-555", "is_primary": "1"},
			Status:  StatusPending, ApprovalRequestID: &approval, CreatedAt: created},
		{ID: 14, EmployeeID: 8, EmployeeName: "Alan Turing", EmployeeNumber: "E-008", ChangeType: TypeAddress,
			Payload: map[string]string{"city": "Surabaya"}, Status: StatusApproved, CreatedAt: created},
	}, nil
}

func (f *fakeProfileChanges) RetryApply(_ context.Context, _, id, _ int64) error {
	f.retried = append(f.retried, id)
	return f.retryErr
}

type fakeEmployees struct{ getCalls, contactCalls int }

func (f *fakeEmployees) GetByID(_ context.Context, _, id int64) (employees.Employee, error) {
	f.getCalls++
	dob := time.Date(1990, 1, 2, 0, 0, 0, 0, time.UTC)
	return employees.Employee{ID: id, Name: "Ada Lovelace", Profile: employees.Profile{
		NationalID: "3201010101900001", DateOfBirth: &dob, Phone: "0812-OLD",
		AddressLine1: "Jl. Lama 1", City: "Jakarta",
	}}, nil
}

func (f *fakeEmployees) ListContacts(_ context.Context, _, employeeID int64) ([]employees.EmergencyContact, error) {
	f.contactCalls++
	return []employees.EmergencyContact{{ID: 5, EmployeeID: employeeID, Name: "Byron", Relationship: "Parent", Phone: "0811-555"}}, nil
}

type fakeBank struct{ calls int }

func (f *fakeBank) AsOf(_ context.Context, _, employeeID int64, _ time.Time) (compensation.Assignment, error) {
	f.calls++
	return compensation.Assignment{EmployeeID: employeeID, BankCode: "MANDIRI", BankAccountNumber: "1122334455", BankAccountName: "Ada Lovelace"}, nil
}

func profileRequest(t *testing.T, method, path string, form url.Values) (*http.Request, *shared.Session) {
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
	return req.WithContext(shared.ContextWithSession(req.Context(), session)), session
}

type profileFixture struct {
	svc  *fakeProfileChanges
	emp  *fakeEmployees
	bank *fakeBank
}

func newProfileRouter(t *testing.T, perms []string, fx profileFixture, withTemplates bool) http.Handler {
	t.Helper()
	var engine *view.Engine
	if withTemplates {
		var err error
		engine, err = view.NewEngine()
		if err != nil {
			t.Fatal(err)
		}
	}
	h := NewHandler(nil, fx.svc, fx.emp, fx.bank, engine, shared.NewCSRFManager("secret"), rbac.Middleware{Service: permissionReader{perms}})
	router := chi.NewRouter()
	router.Route("/hr/profile-changes", h.MountRoutes)
	return router
}

func newFixture() profileFixture {
	return profileFixture{svc: &fakeProfileChanges{}, emp: &fakeEmployees{}, bank: &fakeBank{}}
}

func TestProfileChangeAdminRoutePermissions(t *testing.T) {
	cases := []struct {
		name         string
		perms        []string
		method, path string
		form         url.Values
	}{
		{"page requires hr.profile_change.admin", []string{shared.PermHREmployeeAdmin, shared.PermHREmployeeSensitive, shared.PermHRExpenseAdmin}, http.MethodGet, "/hr/profile-changes", nil},
		{"retry requires hr.profile_change.admin", []string{shared.PermHREmployeeAdmin, shared.PermHREmployeeSensitive}, http.MethodPost, "/hr/profile-changes/11/retry", url.Values{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fx := newFixture()
			req, _ := profileRequest(t, tc.method, tc.path, tc.form)
			rec := httptest.NewRecorder()
			newProfileRouter(t, tc.perms, fx, false).ServeHTTP(rec, req)
			if rec.Code != http.StatusForbidden {
				t.Fatalf("status=%d want 403", rec.Code)
			}
			if len(fx.svc.filters) != 0 || len(fx.svc.retried) != 0 || fx.emp.getCalls != 0 || fx.bank.calls != 0 {
				t.Fatal("forbidden request reached a service")
			}
		})
	}
}

func TestProfileChangeRetryApply(t *testing.T) {
	cases := []struct {
		name     string
		retryErr error
		wantKind string
		wantMsg  string
	}{
		{"success", nil, "success", "Profile change decision applied"},
		{"domain error is shown", ErrNotRetryable, "error", "Only pending profile change requests can be retried"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fx := newFixture()
			fx.svc.retryErr = tc.retryErr
			req, session := profileRequest(t, http.MethodPost, "/hr/profile-changes/11/retry", url.Values{})
			rec := httptest.NewRecorder()
			newProfileRouter(t, []string{shared.PermHRProfileChangeAdmin}, fx, false).ServeHTTP(rec, req)
			if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/hr/profile-changes" {
				t.Fatalf("status=%d location=%q", rec.Code, rec.Header().Get("Location"))
			}
			if len(fx.svc.retried) != 1 || fx.svc.retried[0] != 11 {
				t.Fatalf("retried=%v", fx.svc.retried)
			}
			f := session.PopFlash()
			if f == nil || f.Kind != tc.wantKind || !strings.HasPrefix(f.Message, tc.wantMsg) {
				t.Fatalf("flash=%+v", f)
			}
		})
	}
}

func TestProfileChangeAdminPageRenders(t *testing.T) {
	cases := []struct {
		name          string
		perms         []string
		wantSensitive bool
		present       []string
		absent        []string
		bankCalls     int
	}{
		{
			name:          "without hr.employee.sensitive values are masked",
			perms:         []string{shared.PermHRProfileChangeAdmin},
			wantSensitive: false,
			present: []string{
				"data-masked-notice", MaskedValue,
				// phone is not sensitive: the diff shows current and proposed
				"0812-OLD", "0812-NEW",
				// emergency contact diff
				"Parent", "Father",
			},
			absent: []string{
				"Jl. Lama 1", "Jl. Baru 9", "Jakarta", "Bandung",
				"9988776655", "1122334455", "MANDIRI", "BCA",
				"3201010101900001", "Surabaya",
			},
			bankCalls: 0,
		},
		{
			name:          "with hr.employee.sensitive values are shown",
			perms:         []string{shared.PermHRProfileChangeAdmin, shared.PermHREmployeeSensitive},
			wantSensitive: true,
			present: []string{
				"Jl. Lama 1", "Jl. Baru 9", "Jakarta", "Bandung",
				"1122334455", "9988776655", "MANDIRI", "BCA",
				"0812-OLD", "0812-NEW", "Parent", "Father",
				// decided request: payload only
				"Surabaya",
			},
			absent:    []string{"data-masked-notice", MaskedValue, "3201010101900001"},
			bankCalls: 1,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fx := newFixture()
			req, _ := profileRequest(t, http.MethodGet, "/hr/profile-changes?status=pending&type=bogus&q=ada", nil)
			rec := httptest.NewRecorder()
			newProfileRouter(t, tc.perms, fx, true).ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
			if len(fx.svc.filters) != 1 {
				t.Fatalf("ListAll calls=%d", len(fx.svc.filters))
			}
			f := fx.svc.filters[0]
			if f.Status != StatusPending || f.ChangeType != "" || f.Search != "ada" || f.ShowSensitive != tc.wantSensitive {
				t.Fatalf("filter=%+v", f)
			}
			body := rec.Body.String()
			for _, want := range tc.present {
				if !strings.Contains(body, want) {
					t.Errorf("body missing %q", want)
				}
			}
			for _, unwanted := range tc.absent {
				if strings.Contains(body, unwanted) {
					t.Errorf("body unexpectedly contains %q", unwanted)
				}
			}
			for _, want := range []string{`action="/hr/profile-changes/11/retry"`, `action="/hr/profile-changes/12/retry"`, "employee row locked", `href="/hr/profile-changes"`} {
				if !strings.Contains(body, want) {
					t.Errorf("body missing %q", want)
				}
			}
			if strings.Contains(body, `action="/hr/profile-changes/14/retry"`) {
				t.Error("retry rendered for a decided request")
			}
			if fx.emp.getCalls != 1 {
				t.Errorf("GetByID calls=%d want 1 (cached per employee, pending only)", fx.emp.getCalls)
			}
			if fx.bank.calls != tc.bankCalls {
				t.Errorf("AsOf calls=%d want %d", fx.bank.calls, tc.bankCalls)
			}
			parts := strings.Split(body, `<form method="post"`)
			for _, part := range parts[1:] {
				end := strings.Index(part, "</form>")
				if end < 0 || !strings.Contains(part[:end], `name="csrf_token"`) {
					t.Fatalf("POST form without csrf_token: %.120s", part)
				}
			}
		})
	}
}
