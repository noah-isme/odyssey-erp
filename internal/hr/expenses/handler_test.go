package expenses

import (
	"context"
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

type fakeExpenses struct {
	filters  []Filter
	retried  []int64
	retryErr error
}

func (f *fakeExpenses) ListAll(_ context.Context, _ int64, filter Filter) ([]Claim, error) {
	f.filters = append(f.filters, filter)
	approval := int64(91)
	doc := int64(555)
	return []Claim{
		{ID: 41, EmployeeName: "Ada Lovelace", EmployeeNumber: "E-007", Department: "Finance",
			ClaimDate: time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC), Category: "Travel", Description: "Taxi to client",
			Amount: "125000.50", Currency: "IDR", DocumentID: &doc, Status: StatusPending, ApprovalRequestID: &approval,
			ApplyError: "approval company mismatch"},
		{ID: 42, EmployeeName: "Alan Turing", EmployeeNumber: "E-008",
			ClaimDate: time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC), Category: "Meals",
			Amount: "80000.00", Currency: "IDR", Status: StatusApproved},
	}, nil
}

func (f *fakeExpenses) RetryApply(_ context.Context, _, id, _ int64) error {
	f.retried = append(f.retried, id)
	return f.retryErr
}

func expenseRequest(t *testing.T, method, path string, form url.Values) (*http.Request, *shared.Session) {
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

func newExpenseRouter(t *testing.T, perms []string, svc *fakeExpenses, withTemplates bool) http.Handler {
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
	router.Route("/hr/expenses", h.MountRoutes)
	return router
}

func TestExpenseAdminRoutePermissions(t *testing.T) {
	cases := []struct {
		name         string
		perms        []string
		method, path string
		form         url.Values
	}{
		{"page requires hr.expense.admin", []string{shared.PermHREmployeeAdmin, shared.PermHRProfileChangeAdmin}, http.MethodGet, "/hr/expenses", nil},
		{"retry requires hr.expense.admin", []string{shared.PermHREmployeeAdmin, shared.PermHROvertimeAdmin}, http.MethodPost, "/hr/expenses/41/retry", url.Values{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &fakeExpenses{}
			req, _ := expenseRequest(t, tc.method, tc.path, tc.form)
			rec := httptest.NewRecorder()
			newExpenseRouter(t, tc.perms, svc, false).ServeHTTP(rec, req)
			if rec.Code != http.StatusForbidden {
				t.Fatalf("status=%d want 403", rec.Code)
			}
			if len(svc.filters) != 0 || len(svc.retried) != 0 {
				t.Fatal("forbidden request reached the expense service")
			}
		})
	}
}

func TestExpenseRetryApply(t *testing.T) {
	cases := []struct {
		name     string
		retryErr error
		wantKind string
		wantMsg  string
	}{
		{"success", nil, "success", "Expense claim decision applied"},
		{"domain error is shown", ErrApprovalNotDecided, "error", "Approval is not decided yet"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &fakeExpenses{retryErr: tc.retryErr}
			req, session := expenseRequest(t, http.MethodPost, "/hr/expenses/41/retry", url.Values{})
			rec := httptest.NewRecorder()
			newExpenseRouter(t, []string{shared.PermHRExpenseAdmin}, svc, false).ServeHTTP(rec, req)
			if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/hr/expenses" {
				t.Fatalf("status=%d location=%q", rec.Code, rec.Header().Get("Location"))
			}
			if len(svc.retried) != 1 || svc.retried[0] != 41 {
				t.Fatalf("retried=%v", svc.retried)
			}
			f := session.PopFlash()
			if f == nil || f.Kind != tc.wantKind || !strings.HasPrefix(f.Message, tc.wantMsg) {
				t.Fatalf("flash=%+v", f)
			}
		})
	}
}

func TestExpenseAdminPageRenders(t *testing.T) {
	svc := &fakeExpenses{}
	req, _ := expenseRequest(t, http.MethodGet, "/hr/expenses?status=pending&q=ada&from=2026-10-01&to=bad", nil)
	rec := httptest.NewRecorder()
	newExpenseRouter(t, []string{shared.PermHRExpenseAdmin}, svc, true).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if len(svc.filters) != 1 {
		t.Fatalf("ListAll calls=%d", len(svc.filters))
	}
	f := svc.filters[0]
	if f.Status != StatusPending || f.Search != "ada" || f.From == nil || !f.From.Equal(time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)) || f.To != nil {
		t.Fatalf("filter=%+v", f)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"Ada Lovelace", "Taxi to client", "IDR 125000.50", "approval company mismatch",
		`href="/documents/library/555"`, `action="/hr/expenses/41/retry"`, `href="/hr/expenses"`,
		`value="2026-10-01"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %q", want)
		}
	}
	if strings.Contains(body, `action="/hr/expenses/42/retry"`) {
		t.Fatal("retry rendered for a decided claim")
	}
	parts := strings.Split(body, `<form method="post"`)
	for _, part := range parts[1:] {
		end := strings.Index(part, "</form>")
		if end < 0 || !strings.Contains(part[:end], `name="csrf_token"`) {
			t.Fatalf("POST form without csrf_token: %.120s", part)
		}
	}
}
