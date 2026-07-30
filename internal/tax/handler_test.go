package tax

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/odyssey-erp/odyssey-erp/internal/rbac"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
)

type permissionReader struct{ permissions []string }

func (p permissionReader) EffectivePermissions(context.Context, int64) ([]string, error) {
	return p.permissions, nil
}

func taxRequest(t *testing.T, path string) *http.Request {
	t.Helper()
	manager := shared.NewSessionManager(nil, "session", "secret", time.Hour, false)
	req := httptest.NewRequest(http.MethodGet, path, nil)
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
			router.ServeHTTP(rec, taxRequest(t, "/tax/periods/2/export"))
			if rec.Code != tc.status {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
			if tc.status == http.StatusOK && rec.Header().Get("Content-Type") != "application/xml" {
				t.Fatal("missing XML content type")
			}
		})
	}
}
