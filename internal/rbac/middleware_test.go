package rbac

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/odyssey-erp/odyssey-erp/internal/shared"
)

type stubPermissionReader struct {
	permissions []string
	err         error
}

func (s stubPermissionReader) EffectivePermissions(context.Context, int64) ([]string, error) {
	return s.permissions, s.err
}

type stubScopedPermissionReader struct {
	scopedPermissions []string
	scopedErr         error
	userID            int64
	scope             AccessScope
	at                time.Time
	calls             int
}

func (s *stubScopedPermissionReader) EffectivePermissions(context.Context, int64) ([]string, error) {
	return nil, nil
}

func (s *stubScopedPermissionReader) EffectivePermissionsInScope(_ context.Context, userID int64, scope AccessScope, at time.Time) ([]string, error) {
	s.userID = userID
	s.scope = scope
	s.at = at
	s.calls++
	return s.scopedPermissions, s.scopedErr
}

func authorizedRequest(t *testing.T, userID string) *http.Request {
	return requestWithSession(t, userID, "", "")
}

func requestWithSession(t *testing.T, userID, companyID, branchID string) *http.Request {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Errorf("close redis client: %v", err)
		}
	})
	manager := shared.NewSessionManager(client, "test", "secret", time.Hour, false)
	session, err := manager.Load(context.Background(), httptest.NewRequest(http.MethodGet, "/", nil))
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	session.SetUser(userID)
	if companyID != "" {
		session.Set("company_id", companyID)
	}
	if branchID != "" {
		session.Set("branch_id", branchID)
	}
	request := httptest.NewRequest(http.MethodGet, "/protected", nil)
	return request.WithContext(shared.ContextWithSession(request.Context(), session))
}

func TestRequireAny(t *testing.T) {
	tests := []struct {
		name       string
		reader     stubPermissionReader
		request    *http.Request
		wantStatus int
	}{
		{name: "allows granted permission", reader: stubPermissionReader{permissions: []string{"users.view"}}, request: authorizedRequest(t, "42"), wantStatus: http.StatusOK},
		{name: "denies missing permission", reader: stubPermissionReader{}, request: authorizedRequest(t, "42"), wantStatus: http.StatusForbidden},
		{name: "fails closed on service error", reader: stubPermissionReader{err: errors.New("db unavailable")}, request: authorizedRequest(t, "42"), wantStatus: http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			middleware := Middleware{Service: tt.reader}.RequireAny("users.view")
			recorder := httptest.NewRecorder()
			middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })).ServeHTTP(recorder, tt.request)
			if recorder.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", recorder.Code, tt.wantStatus)
			}
		})
	}
}

func TestRequireAllDeniesWhenOnePermissionMissing(t *testing.T) {
	middleware := Middleware{Service: stubPermissionReader{permissions: []string{"users.view"}}}.RequireAll("users.view", "roles.view")
	recorder := httptest.NewRecorder()
	middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })).ServeHTTP(recorder, authorizedRequest(t, "42"))
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusForbidden)
	}
}

func TestRequireAnyDeniesWithoutSession(t *testing.T) {
	middleware := Middleware{Service: stubPermissionReader{permissions: []string{"users.view"}}}.RequireAny("users.view")
	recorder := httptest.NewRecorder()
	middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/protected", nil))
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusForbidden)
	}
}

func TestRequireAnyInScopeUsesCompanyScope(t *testing.T) {
	reader := &stubScopedPermissionReader{scopedPermissions: []string{"inventory.view"}}
	middleware := Middleware{Service: reader}.RequireAnyInScope("inventory.view")
	recorder := httptest.NewRecorder()
	middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })).ServeHTTP(recorder, requestWithSession(t, "42", "7", ""))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if reader.calls != 1 || reader.userID != 42 || reader.scope.CompanyID != 7 || reader.scope.BranchID != nil {
		t.Fatalf("scoped lookup = calls:%d user:%d scope:%+v", reader.calls, reader.userID, reader.scope)
	}
	if reader.at.IsZero() || reader.at.Location() != time.UTC {
		t.Fatalf("evaluation time = %v, want a UTC timestamp", reader.at)
	}
}

func TestScopedRouteMakesLegacyPermissionHelpersTenantAware(t *testing.T) {
	reader := &stubScopedPermissionReader{scopedPermissions: []string{"inventory.view"}}
	legacyCheck := Middleware{Service: reader}.RequireAny("inventory.edit")
	handler := ScopedRoute(legacyCheck(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, requestWithSession(t, "42", "7", ""))

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusForbidden)
	}
	if reader.calls != 1 || reader.scope.CompanyID != 7 {
		t.Fatalf("scoped lookup = calls:%d scope:%+v, want one lookup in company 7", reader.calls, reader.scope)
	}
}

func TestRequireAnyInScopeUsesOptionalBranchScope(t *testing.T) {
	reader := &stubScopedPermissionReader{scopedPermissions: []string{"inventory.view"}}
	middleware := Middleware{Service: reader}.RequireAnyInScope("inventory.view")
	recorder := httptest.NewRecorder()
	middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })).ServeHTTP(recorder, requestWithSession(t, "42", "7", "11"))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if reader.scope.CompanyID != 7 || reader.scope.BranchID == nil || *reader.scope.BranchID != 11 {
		t.Fatalf("scope = %+v, want company 7 and branch 11", reader.scope)
	}
}

func TestScopedMiddlewareRejectsMissingOrInvalidTenant(t *testing.T) {
	tests := []struct {
		name    string
		request *http.Request
	}{
		{name: "missing session", request: httptest.NewRequest(http.MethodGet, "/protected", nil)},
		{name: "missing company", request: authorizedRequest(t, "42")},
		{name: "invalid company", request: requestWithSession(t, "42", "0", "")},
		{name: "invalid branch", request: requestWithSession(t, "42", "7", "-1")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := &stubScopedPermissionReader{scopedPermissions: []string{"inventory.view"}}
			middleware := Middleware{Service: reader}.RequireAnyInScope("inventory.view")
			recorder := httptest.NewRecorder()
			middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })).ServeHTTP(recorder, tt.request)
			if recorder.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusForbidden)
			}
			if reader.calls != 0 {
				t.Fatalf("scoped lookup calls = %d, want 0", reader.calls)
			}
		})
	}
}

func TestScopedMiddlewareRejectsUnavailableReader(t *testing.T) {
	tests := []struct {
		name   string
		reader PermissionReader
	}{
		{name: "legacy reader", reader: stubPermissionReader{permissions: []string{"inventory.view"}}},
		{name: "service without scoped repository", reader: NewService(legacyRepositoryFake{})},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			middleware := Middleware{Service: tt.reader}.RequireAnyInScope("inventory.view")
			recorder := httptest.NewRecorder()
			middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })).ServeHTTP(recorder, requestWithSession(t, "42", "7", ""))
			if recorder.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusForbidden)
			}
		})
	}
}

func TestScopedMiddlewareReturns500OnRepositoryError(t *testing.T) {
	reader := &stubScopedPermissionReader{scopedErr: errors.New("db unavailable")}
	middleware := Middleware{Service: reader}.RequireAnyInScope("inventory.view")
	recorder := httptest.NewRecorder()
	middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })).ServeHTTP(recorder, requestWithSession(t, "42", "7", ""))
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
}

func TestScopedMiddlewareAnyAndAll(t *testing.T) {
	tests := []struct {
		name       string
		middleware func(Middleware) func(http.Handler) http.Handler
		granted    []string
		wantStatus int
	}{
		{name: "any allows one", middleware: func(m Middleware) func(http.Handler) http.Handler {
			return m.RequireAnyInScope("users.view", "users.edit")
		}, granted: []string{"users.view"}, wantStatus: http.StatusOK},
		{name: "any denies none", middleware: func(m Middleware) func(http.Handler) http.Handler {
			return m.RequireAnyInScope("users.view", "users.edit")
		}, granted: []string{"users.list"}, wantStatus: http.StatusForbidden},
		{name: "all allows both", middleware: func(m Middleware) func(http.Handler) http.Handler {
			return m.RequireAllInScope("users.view", "users.edit")
		}, granted: []string{"users.view", "users.edit"}, wantStatus: http.StatusOK},
		{name: "all denies one", middleware: func(m Middleware) func(http.Handler) http.Handler {
			return m.RequireAllInScope("users.view", "users.edit")
		}, granted: []string{"users.view"}, wantStatus: http.StatusForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := &stubScopedPermissionReader{scopedPermissions: tt.granted}
			recorder := httptest.NewRecorder()
			tt.middleware(Middleware{Service: reader})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })).ServeHTTP(recorder, requestWithSession(t, "42", "7", ""))
			if recorder.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", recorder.Code, tt.wantStatus)
			}
		})
	}
}
