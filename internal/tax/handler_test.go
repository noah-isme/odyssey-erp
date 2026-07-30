package tax

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

func taxRequest(t *testing.T, method, path string, form url.Values) *http.Request {
	t.Helper()
	manager := shared.NewSessionManager(nil, "session", "secret", time.Hour, false)
	var body *strings.Reader
	if form == nil {
		body = strings.NewReader("")
	} else {
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

func TestExportRouteRequiresPermissionAndReturnsXML(t *testing.T) {
	f := &fakeStore{schema: reviewedSchema(), exportID: 1, records: []ExportRecord{{TaxNumber: "010", DocumentNumber: "INV", CounterpartyName: "Buyer", CounterpartyTaxID: "123", IssueDate: time.Now(), TaxableBase: 100, TaxAmount: 11, Sign: 1}}}
	for _, tc := range []struct {
		name        string
		permissions []string
		status      int
	}{{"denied", nil, http.StatusForbidden}, {"allowed", []string{"tax.report.export"}, http.StatusOK}} {
		t.Run(tc.name, func(t *testing.T) {
			router := chi.NewRouter()
			mw := rbac.Middleware{Service: permissionReader{tc.permissions}}
			h := NewHandler(nil, NewService(f, ReviewedSchemaValidator{}), nil, nil, mw)
			router.Route("/tax", h.MountRoutes)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, taxRequest(t, http.MethodPost, "/tax/periods/2/export", nil))
			if rec.Code != tc.status {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
			if tc.status == http.StatusOK && rec.Header().Get("Content-Type") != "application/xml" {
				t.Fatal("missing XML content type")
			}
		})
	}
}

func TestExportRouteDoesNotMutateOnGET(t *testing.T) {
	router := chi.NewRouter()
	h := NewHandler(nil, NewService(&fakeStore{}, nil), nil, nil, rbac.Middleware{Service: permissionReader{[]string{"tax.report.export"}}})
	router.Route("/tax", h.MountRoutes)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, taxRequest(t, http.MethodGet, "/tax/periods/2/export", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status=%d want 405", rec.Code)
	}
}

func TestDashboardWithoutSessionDoesNotPanic(t *testing.T) {
	templates, err := view.NewEngine()
	if err != nil {
		t.Fatal(err)
	}
	h := NewHandler(nil, NewService(&fakeStore{}, nil), templates, nil, rbac.Middleware{Service: permissionReader{[]string{"tax.view"}}})
	rec := httptest.NewRecorder()
	h.dashboard(rec, httptest.NewRequest(http.MethodGet, "/tax/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestTaxMutationRoutes(t *testing.T) {
	f := &fakeStore{}
	router := chi.NewRouter()
	h := NewHandler(nil, NewService(f, nil), nil, nil, rbac.Middleware{Service: permissionReader{[]string{"tax.period.lock", "tax.config.manage", "tax.document.correct"}}})
	router.Route("/tax", h.MountRoutes)
	requests := []struct {
		path string
		form url.Values
	}{
		{"/tax/periods/2/lock", nil},
		{"/tax/periods/2/build", nil},
		{"/tax/documents/4/cancel", url.Values{"reason": {"wrong buyer"}}},
		{"/tax/documents/4/replace", url.Values{"replacement_id": {"5"}, "reason": {"correction"}}},
	}
	for _, tc := range requests {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, taxRequest(t, http.MethodPost, tc.path, tc.form))
		if rec.Code != http.StatusSeeOther {
			t.Fatalf("%s status=%d body=%s", tc.path, rec.Code, rec.Body.String())
		}
	}
	if !f.locked || !f.built || len(f.cancelled) != 1 || len(f.replaced) != 1 || f.replaced[0] != [2]int64{4, 5} {
		t.Fatalf("actions not recorded: locked=%v built=%v cancelled=%v replaced=%v", f.locked, f.built, f.cancelled, f.replaced)
	}
}
