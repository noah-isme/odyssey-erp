package documentshttp

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/odyssey-erp/odyssey-erp/internal/documents"
	"github.com/odyssey-erp/odyssey-erp/internal/rbac"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
)

func TestRoutesRequireAuthenticatedPermission(t *testing.T) {
	router := chi.NewRouter()
	h := NewHandler(nil, nil, nil, nil, rbac.Middleware{}, nil, nil)
	router.Route("/documents", h.MountRoutes)

	for _, path := range []string{
		"/documents/library/", "/documents/search/", "/documents/categories/", "/documents/classifications/",
		"/documents/library/1/acl", "/documents/library/1/versions/1/submit-review",
		"/documents/library/1/versions/1/review", "/documents/library/1/versions/1/challenge",
		"/documents/library/1/versions/1/download",
	} {
		t.Run(path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
			if rec.Code != http.StatusForbidden {
				t.Fatalf("status=%d want %d", rec.Code, http.StatusForbidden)
			}
		})
	}
}

func TestDocumentActionRoutesRequireAuthenticatedPermission(t *testing.T) {
	router := chi.NewRouter()
	h := NewHandler(nil, nil, nil, nil, rbac.Middleware{}, nil, nil)
	router.Route("/documents", h.MountRoutes)

	paths := []string{
		"/documents/library/1/acl",
		"/documents/library/1/acl/2/delete",
		"/documents/library/1/versions/1/submit-review",
		"/documents/library/1/versions/1/review",
		"/documents/library/1/versions/1/sign",
		"/documents/library/1/versions/1/retention",
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, path, nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != http.StatusForbidden {
				t.Fatalf("status=%d want %d", rec.Code, http.StatusForbidden)
			}
		})
	}
}

func TestValidACLPermission(t *testing.T) {
	for _, permission := range []string{"READ", "WRITE", "ADMIN", "APPROVE", "SIGN"} {
		if !validACLPermission(permission) {
			t.Errorf("permission %q should be valid", permission)
		}
	}
	for _, permission := range []string{"", "VIEW", "documents.view", "DELETE"} {
		if validACLPermission(permission) {
			t.Errorf("permission %q should be rejected", permission)
		}
	}
}

func TestDocumentOwnershipHelpersRejectInvalidScope(t *testing.T) {
	h := NewHandler(nil, documents.NewService(nil, nil), nil, nil, rbac.Middleware{}, nil, nil)
	if _, err := h.documentForCompany(nil, 1, 0); !errors.Is(err, documents.ErrDocumentNotFound) {
		t.Fatalf("document scope error=%v", err)
	}
	if _, _, err := h.documentVersionForCompany(nil, 1, 0, 1); !errors.Is(err, documents.ErrDocumentVersionNotFound) {
		t.Fatalf("version scope error=%v", err)
	}
}

func TestCreateDocumentRedirectsForInvalidInput(t *testing.T) {
	h := NewHandler(slog.New(slog.NewTextHandler(io.Discard, nil)), documents.NewService(nil, nil), nil, nil, rbac.Middleware{}, nil, nil)
	req := documentsRequestWithSession(t, http.MethodPost, "/documents/library/", "7", "3")
	rec := httptest.NewRecorder()

	h.createDocument(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status=%d want %d", rec.Code, http.StatusSeeOther)
	}
	if got := rec.Header().Get("Location"); got != "/documents/library/new" {
		t.Fatalf("redirect=%q", got)
	}
}

func documentsRequestWithSession(t *testing.T, method, path, userID, companyID string) *http.Request {
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
