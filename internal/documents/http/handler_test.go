package documentshttp

import (
	"context"
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

	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/documents/library/"},
		{http.MethodGet, "/documents/search/"},
		{http.MethodGet, "/documents/categories/"},
		{http.MethodGet, "/documents/classifications/"},
		{http.MethodGet, "/documents/library/1/acl"},
		{http.MethodPost, "/documents/library/1/acl"},
		{http.MethodGet, "/documents/library/1/versions/2/download"},
		{http.MethodPost, "/documents/library/1/versions/2/submit-review"},
		{http.MethodPost, "/documents/library/1/versions/2/review"},
	} {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(tc.method, tc.path, nil))
			if rec.Code != http.StatusForbidden {
				t.Fatalf("status=%d want %d", rec.Code, http.StatusForbidden)
			}
		})
	}
}

// A user with documents.view but not documents.admin can read the ACL page
// route but not create entries.
func TestACLCreateRequiresAdminPermission(t *testing.T) {
	router := chi.NewRouter()
	h := NewHandler(nil, nil, nil, nil, rbac.Middleware{Service: staticPerms{shared.PermDocumentsView}}, nil, nil)
	router.Route("/documents", h.MountRoutes)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, documentsRequestWithSession(t, http.MethodPost, "/documents/library/1/acl", "7", "3"))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d want %d", rec.Code, http.StatusForbidden)
	}
}

type staticPerms []string

func (p staticPerms) EffectivePermissions(context.Context, int64) ([]string, error) {
	return p, nil
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
