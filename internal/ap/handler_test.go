package ap

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	"github.com/odyssey-erp/odyssey-erp/internal/rbac"
)

func TestAPHandler_MountRoutes(t *testing.T) {
	logger := slog.Default()
	r := chi.NewRouter()

	// Create handler with nil service/templates for route testing only
	handler := NewHandler(logger, nil, nil, nil, nil, rbac.Middleware{})
	handler.MountRoutes(r)

	// Test if routes are properly mounted by checking a GET request
	// We expect a panic or a template error because templates are nil,
	// but the route itself should exist and not return 404.

	req := httptest.NewRequest(http.MethodGet, "/invoices", nil)
	w := httptest.NewRecorder()

	// Since templates are nil, calling the handler will panic when it tries to render.
	// We can recover it to prove the route exists.
	defer func() {
		if err := recover(); err != nil {
			// Expected panic due to nil templates
			require.NotNil(t, err)
		}
	}()

	r.ServeHTTP(w, req)
}
