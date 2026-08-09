package qmshttp

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/odyssey-erp/odyssey-erp/internal/qms"
	"github.com/odyssey-erp/odyssey-erp/internal/rbac"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
)

func TestRoutesRequireAuthenticatedPermission(t *testing.T) {
	router := chi.NewRouter()
	h := NewHandler(nil, nil, nil, nil, rbac.Middleware{}, nil, nil)
	router.Route("/qms", h.MountRoutes)

	for _, path := range []string{"/qms/ncrs/", "/qms/capas/", "/qms/audits/", "/qms/supplier-quality/", "/qms/inspections/", "/qms/complaints/"} {
		t.Run(path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
			if rec.Code != http.StatusForbidden {
				t.Fatalf("status=%d want %d", rec.Code, http.StatusForbidden)
			}
		})
	}
}

func TestCreateNCRRedirectsForInvalidInput(t *testing.T) {
	h := NewHandler(slog.New(slog.NewTextHandler(io.Discard, nil)), qms.NewService(nil), nil, nil, rbac.Middleware{}, nil, nil)
	req := qmsRequestWithSession(t, http.MethodPost, "/qms/ncrs/", "7", "3")
	rec := httptest.NewRecorder()

	h.createNCR(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status=%d want %d", rec.Code, http.StatusSeeOther)
	}
	if got := rec.Header().Get("Location"); got != "/qms/ncrs/new" {
		t.Fatalf("redirect=%q", got)
	}
}

func qmsRequestWithSession(t *testing.T, method, path, userID, companyID string) *http.Request {
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
