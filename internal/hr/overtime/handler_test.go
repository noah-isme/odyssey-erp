package overtime

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/odyssey-erp/odyssey-erp/internal/rbac"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/view"
)

type permissionReader struct{ permissions []string }

func (p permissionReader) EffectivePermissions(context.Context, int64) ([]string, error) {
	return p.permissions, nil
}

// fakeOvertime is an in-memory HandlerService that records calls.
type fakeOvertime struct {
	submitted    []SubmitInput
	cancelled    []int64
	retried      []int64
	listAllCalls int
	retryErr     error
}

func (f *fakeOvertime) writes() int { return len(f.submitted) + len(f.cancelled) + len(f.retried) }

func (f *fakeOvertime) Submit(_ context.Context, in SubmitInput) (Request, error) {
	f.submitted = append(f.submitted, in)
	return Request{ID: 1}, nil
}

func (f *fakeOvertime) Cancel(_ context.Context, _, id, _ int64) error {
	f.cancelled = append(f.cancelled, id)
	return nil
}

func (f *fakeOvertime) ListOwn(context.Context, int64, int64) ([]Request, error) {
	return []Request{{ID: 21, WorkedOn: time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC), Minutes: 90, Reason: "Month-end close", Status: StatusPending}}, nil
}

func (f *fakeOvertime) ListAll(context.Context, int64, Filter) ([]Request, error) {
	f.listAllCalls++
	approval := int64(77)
	return []Request{{ID: 31, EmployeeName: "Grace Hopper", EmployeeNumber: "E-042", Department: "Ops",
		WorkedOn: time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC), Minutes: 120, Status: StatusPending,
		ApprovalRequestID: &approval, ApplyError: ApplyErrorPayrollApproval}}, nil
}

func (f *fakeOvertime) RetryApply(_ context.Context, _, id, _ int64) error {
	f.retried = append(f.retried, id)
	return f.retryErr
}

func overtimeRequest(t *testing.T, method, path string, form url.Values) (*http.Request, *shared.Session) {
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

func newOvertimeRouter(t *testing.T, perms []string, svc *fakeOvertime, withTemplates bool) http.Handler {
	t.Helper()
	var engine *view.Engine
	if withTemplates {
		var err error
		engine, err = view.NewEngine()
		if err != nil {
			t.Fatal(err)
		}
	}
	h := NewHandler(nil, svc, engine, shared.NewCSRFManager("secret"), rbac.Middleware{Service: permissionReader{perms}})
	router := chi.NewRouter()
	router.Route("/hr/overtime", h.MountRoutes)
	return router
}

func TestOvertimeRoutePermissions(t *testing.T) {
	submitForm := url.Values{"worked_on": {"2026-10-05"}, "hours": {"1.5"}}
	cases := []struct {
		name         string
		perms        []string
		method, path string
		form         url.Values
	}{
		{"page requires an overtime permission", []string{shared.PermHRLeaveRequest}, http.MethodGet, "/hr/overtime", nil},
		{"submit requires hr.overtime.request", []string{shared.PermHROvertimeAdmin}, http.MethodPost, "/hr/overtime", submitForm},
		{"cancel requires hr.overtime.request", []string{shared.PermHROvertimeAdmin}, http.MethodPost, "/hr/overtime/21/cancel", url.Values{}},
		{"retry requires hr.overtime.admin", []string{shared.PermHROvertimeRequest, shared.PermHRScheduleAdmin}, http.MethodPost, "/hr/overtime/31/retry", url.Values{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &fakeOvertime{}
			req, _ := overtimeRequest(t, tc.method, tc.path, tc.form)
			rec := httptest.NewRecorder()
			newOvertimeRouter(t, tc.perms, svc, false).ServeHTTP(rec, req)
			if rec.Code != http.StatusForbidden {
				t.Fatalf("status=%d want 403", rec.Code)
			}
			if svc.writes() != 0 {
				t.Fatal("forbidden request reached the overtime service")
			}
		})
	}
}

func TestSubmitOvertimeHappyPath(t *testing.T) {
	svc := &fakeOvertime{}
	req, session := overtimeRequest(t, http.MethodPost, "/hr/overtime", url.Values{"worked_on": {"2026-10-05"}, "hours": {"1.5"}, "reason": {"Close"}})
	rec := httptest.NewRecorder()
	newOvertimeRouter(t, []string{shared.PermHROvertimeRequest}, svc, false).ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/hr/overtime?tab=my-requests" {
		t.Fatalf("status=%d location=%q", rec.Code, rec.Header().Get("Location"))
	}
	want := SubmitInput{UserID: 3, CompanyID: 1, WorkedOn: time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC), Minutes: 90, Reason: "Close"}
	if len(svc.submitted) != 1 || svc.submitted[0] != want {
		t.Fatalf("submitted=%+v want %+v", svc.submitted, want)
	}
	if f := session.PopFlash(); f == nil || f.Kind != "success" {
		t.Fatalf("flash=%+v", f)
	}
}

func TestCancelOwnOvertime(t *testing.T) {
	svc := &fakeOvertime{}
	req, _ := overtimeRequest(t, http.MethodPost, "/hr/overtime/21/cancel", url.Values{})
	rec := httptest.NewRecorder()
	newOvertimeRouter(t, []string{shared.PermHROvertimeRequest}, svc, false).ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther || len(svc.cancelled) != 1 || svc.cancelled[0] != 21 {
		t.Fatalf("status=%d cancelled=%v", rec.Code, svc.cancelled)
	}
}

func TestRetryApplySurfacesFinalizerError(t *testing.T) {
	svc := &fakeOvertime{retryErr: fmt.Errorf("%w: overtime request #31 stays pending; retry apply once the payroll run is posted or returned to draft", ErrPayrollAwaitingApproval)}
	req, session := overtimeRequest(t, http.MethodPost, "/hr/overtime/31/retry", url.Values{})
	rec := httptest.NewRecorder()
	newOvertimeRouter(t, []string{shared.PermHROvertimeAdmin}, svc, false).ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/hr/overtime?tab=all-requests" {
		t.Fatalf("status=%d location=%q", rec.Code, rec.Header().Get("Location"))
	}
	if len(svc.retried) != 1 || svc.retried[0] != 31 {
		t.Fatalf("retried=%v", svc.retried)
	}
	f := session.PopFlash()
	if f == nil || f.Kind != "error" || !strings.HasPrefix(f.Message, "Payroll run awaiting approval: overtime request #31 stays pending") {
		t.Fatalf("flash=%+v", f)
	}
}

func TestParseHours(t *testing.T) {
	cases := map[string]int{"1.5": 90, "0.25": 15, "12": 720, "": 0, "abc": 0, "-1": 0, "NaN": 0, "25": 0}
	for in, want := range cases {
		if got := parseHours(in); got != want {
			t.Errorf("parseHours(%q)=%d want %d", in, got, want)
		}
	}
}

func TestOvertimePageRenders(t *testing.T) {
	cases := []struct {
		name        string
		perms       []string
		tab         string
		wantDialog  bool
		wantAllTab  bool
		wantListAll int
	}{
		{"requester sees own requests and request dialog", []string{shared.PermHROvertimeRequest}, "", true, false, 0},
		{"requester cannot open the all tab", []string{shared.PermHROvertimeRequest}, "?tab=all-requests", true, false, 0},
		{"admin sees all requests with retry", []string{shared.PermHROvertimeAdmin}, "?tab=all-requests", false, true, 1},
		{"requester and admin", []string{shared.PermHROvertimeRequest, shared.PermHROvertimeAdmin}, "", true, true, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &fakeOvertime{}
			req, _ := overtimeRequest(t, http.MethodGet, "/hr/overtime"+tc.tab, nil)
			rec := httptest.NewRecorder()
			newOvertimeRouter(t, tc.perms, svc, true).ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
			body := rec.Body.String()
			if !strings.Contains(body, "Month-end close") || !strings.Contains(body, "1.50") {
				t.Fatal("own requests not rendered")
			}
			if got := strings.Contains(body, `id="request-overtime-dialog"`); got != tc.wantDialog {
				t.Fatalf("request dialog rendered=%v want %v", got, tc.wantDialog)
			}
			if got := strings.Contains(body, `action="/hr/overtime/21/cancel"`); got != tc.wantDialog {
				t.Fatalf("cancel form rendered=%v want %v", got, tc.wantDialog)
			}
			if got := strings.Contains(body, `data-tab="all-requests"`); got != tc.wantAllTab {
				t.Fatalf("all tab rendered=%v want %v", got, tc.wantAllTab)
			}
			if got := strings.Contains(body, `action="/hr/overtime/31/retry"`); got != tc.wantAllTab {
				t.Fatalf("retry form rendered=%v want %v", got, tc.wantAllTab)
			}
			if tc.wantAllTab && !strings.Contains(body, ApplyErrorPayrollApproval) {
				t.Fatal("apply error not rendered")
			}
			if svc.listAllCalls != tc.wantListAll {
				t.Fatalf("ListAll calls=%d want %d", svc.listAllCalls, tc.wantListAll)
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
