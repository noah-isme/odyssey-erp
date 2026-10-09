package cmmshttp

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/odyssey-erp/odyssey-erp/internal/cmms"
	"github.com/odyssey-erp/odyssey-erp/internal/rbac"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
)

func TestRoutesRequireAuthenticatedPermission(t *testing.T) {
	router := chi.NewRouter()
	h := NewHandler(nil, nil, nil, nil, rbac.Middleware{}, nil)
	router.Route("/cmms", h.MountRoutes)

	routes := []struct{ method, path string }{
		{http.MethodGet, "/cmms/work-orders/"},
		{http.MethodGet, "/cmms/assets/"},
		{http.MethodGet, "/cmms/pm-schedules/"},
		{http.MethodGet, "/cmms/spare-parts/"},
		{http.MethodGet, "/cmms/iot/sensors"},
		{http.MethodPost, "/cmms/work-orders/1/status"},
		{http.MethodPost, "/cmms/work-orders/1/complete"},
		{http.MethodPost, "/cmms/work-orders/1/close"},
		{http.MethodGet, "/cmms/assets/1/meter-readings"},
		{http.MethodPost, "/cmms/assets/1/meter-readings"},
	}
	for _, route := range routes {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(route.method, route.path, nil))
			if rec.Code != http.StatusForbidden {
				t.Fatalf("status=%d want %d", rec.Code, http.StatusForbidden)
			}
		})
	}
}

// A user holding only the view permissions reaches the CMMS pages but none
// of the write routes.
func TestWriteRoutesRequireWritePermission(t *testing.T) {
	router := chi.NewRouter()
	view := staticPerms{shared.PermCMMSWorkOrderView, shared.PermCMMSAssetView, shared.PermCMMSPlanView}
	h := NewHandler(nil, nil, nil, nil, rbac.Middleware{Service: view}, nil)
	router.Route("/cmms", h.MountRoutes)

	for _, path := range []string{
		"/cmms/work-orders/1/status",
		"/cmms/work-orders/1/complete",
		"/cmms/work-orders/1/close",
		"/cmms/assets/1/meter-readings",
	} {
		t.Run(path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, cmmsRequestWithSession(t, http.MethodPost, path, "7", "3"))
			if rec.Code != http.StatusForbidden {
				t.Fatalf("status=%d want %d", rec.Code, http.StatusForbidden)
			}
		})
	}
}

type staticPerms []string

func (p staticPerms) EffectivePermissions(context.Context, int64) ([]string, error) {
	return p, nil
}

func TestCreateWorkOrderRedirectsForInvalidInput(t *testing.T) {
	h := NewHandler(slog.New(slog.NewTextHandler(io.Discard, nil)), cmms.NewService(nil), nil, nil, rbac.Middleware{}, nil)
	req := cmmsRequestWithSession(t, http.MethodPost, "/cmms/work-orders/", "7", "3")
	rec := httptest.NewRecorder()

	h.createWorkOrder(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status=%d want %d", rec.Code, http.StatusSeeOther)
	}
	if got := rec.Header().Get("Location"); got != "/cmms/work-orders/new" {
		t.Fatalf("redirect=%q", got)
	}
}

func cmmsRequestWithSession(t *testing.T, method, path, userID, companyID string) *http.Request {
	t.Helper()
	manager := shared.NewSessionManager(nil, "session", "secret", time.Hour, false)
	req := httptest.NewRequest(method, path, nil)
	session, err := manager.Load(req.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	session.SetUser(userID)
	session.Set("company_id", companyID)
	return req.WithContext(shared.ContextWithSession(req.Context(), session))
}
