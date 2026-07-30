package crm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/odyssey-erp/odyssey-erp/internal/rbac"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/view"
)

type crmPermissionReader struct{ permissions []string }

func (p crmPermissionReader) EffectivePermissions(context.Context, int64) ([]string, error) {
	return p.permissions, nil
}

func crmRequest(t *testing.T, path string) *http.Request {
	t.Helper()
	manager := shared.NewSessionManager(nil, "session", "secret", time.Hour, false)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	session, err := manager.Load(req.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	session.SetUser("7")
	session.Set("company_id", "3")
	return req.WithContext(shared.ContextWithSession(req.Context(), session))
}

func TestPipelineEnforcesPermissionAndTeamScope(t *testing.T) {
	tests := []struct {
		name        string
		permissions []string
		status      int
		viewAll     bool
	}{
		{"denied", nil, http.StatusForbidden, false},
		{"rep", []string{"crm.view"}, http.StatusOK, false},
		{"manager", []string{"crm.team.view"}, http.StatusOK, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := newStoreFake()
			templates, err := view.NewEngine()
			if err != nil {
				t.Fatal(err)
			}
			middleware := rbac.Middleware{Service: crmPermissionReader{tc.permissions}}
			handler := NewHandler(nil, NewService(store, nil, nil, nil), templates, nil, middleware)
			router := chi.NewRouter()
			router.Route("/crm", handler.MountRoutes)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, crmRequest(t, "/crm/"))
			if rec.Code != tc.status {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
			if tc.status == http.StatusOK && (store.lastScope.CompanyID != 3 || store.lastScope.UserID != 7 || store.lastScope.ViewAll != tc.viewAll) {
				t.Fatalf("scope=%+v", store.lastScope)
			}
		})
	}
}
