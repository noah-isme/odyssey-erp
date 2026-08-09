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

func authorizedRequest(t *testing.T, userID string) *http.Request {
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
