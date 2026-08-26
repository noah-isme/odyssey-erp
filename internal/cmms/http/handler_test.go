package cmmshttp

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
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

	for _, path := range []string{"/cmms/work-orders/", "/cmms/assets/", "/cmms/pm-schedules/", "/cmms/spare-parts/", "/cmms/iot/sensors"} {
		t.Run(path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
			if rec.Code != http.StatusForbidden {
				t.Fatalf("status=%d want %d", rec.Code, http.StatusForbidden)
			}
		})
	}
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

func TestValidLifecycleTransition(t *testing.T) {
	tests := []struct {
		from cmms.Status
		to   cmms.Status
		want bool
	}{
		{cmms.WorkOrderStatusDraft, cmms.WorkOrderStatusInProgress, true},
		{cmms.WorkOrderStatusInProgress, cmms.WorkOrderStatusCompleted, true},
		{cmms.WorkOrderStatusCompleted, cmms.WorkOrderStatusClosed, true},
		{cmms.WorkOrderStatusClosed, cmms.WorkOrderStatusInProgress, false},
		{cmms.WorkOrderStatusDraft, cmms.WorkOrderStatusClosed, false},
		{cmms.WorkOrderStatusOnHold, cmms.WorkOrderStatusCompleted, false},
	}
	for _, tt := range tests {
		t.Run(string(tt.from)+"-"+string(tt.to), func(t *testing.T) {
			if got := validLifecycleTransition(tt.from, tt.to); got != tt.want {
				t.Fatalf("validLifecycleTransition(%q, %q)=%t want %t", tt.from, tt.to, got, tt.want)
			}
		})
	}
}

func TestMutationKeyUsesExplicitRequestKey(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/cmms/work-orders/4/status", strings.NewReader(url.Values{
		"idempotency_key": {" form-key "},
	}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Idempotency-Key", " header-key ")
	if got := mutationKey(req, "fallback"); got != "header-key" {
		t.Fatalf("header key=%q want header-key", got)
	}

	req = httptest.NewRequest(http.MethodPost, "/cmms/work-orders/4/status", strings.NewReader(url.Values{
		"idempotency_key": {" form-key "},
	}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if got := mutationKey(req, "fallback"); got != "form-key" {
		t.Fatalf("form key=%q want form-key", got)
	}

	req = httptest.NewRequest(http.MethodPost, "/cmms/work-orders/4/status", nil)
	if got := mutationKey(req, "fallback"); got != "fallback" {
		t.Fatalf("fallback key=%q want fallback", got)
	}
}

func TestVerifyCSRFRequiresSessionToken(t *testing.T) {
	manager := shared.NewCSRFManager("csrfsecret")
	h := &Handler{csrf: manager}
	req := cmmsRequestWithSession(t, http.MethodPost, "/cmms/work-orders/1/status", "7", "3")
	if err := h.verifyCSRF(req); err == nil {
		t.Fatal("verifyCSRF without a session token returned nil")
	}
	sess := shared.SessionFromContext(req.Context())
	token, err := manager.EnsureToken(context.Background(), sess)
	if err != nil {
		t.Fatal(err)
	}
	req = httptest.NewRequest(http.MethodPost, "/cmms/work-orders/1/status", strings.NewReader(url.Values{
		"csrf_token": {token},
	}.Encode())).WithContext(shared.ContextWithSession(req.Context(), sess))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if err := h.verifyCSRF(req); err != nil {
		t.Fatalf("verifyCSRF(valid token)=%v", err)
	}
}

func TestMaintenanceMutationRoutesExist(t *testing.T) {
	router := chi.NewRouter()
	h := NewHandler(nil, nil, nil, nil, rbac.Middleware{}, nil)
	router.Route("/cmms", h.MountRoutes)
	paths := []string{
		"/cmms/work-orders/1/complete",
		"/cmms/work-orders/1/close",
		"/cmms/work-orders/1/spare-parts",
		"/cmms/work-order-spare-parts/1/issue",
		"/cmms/assets/1/meter-readings",
		"/cmms/pm-schedules/run-due",
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			ctx := chi.NewRouteContext()
			if !router.Match(ctx, http.MethodPost, path) {
				t.Fatalf("POST route %s was not registered", path)
			}
		})
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
