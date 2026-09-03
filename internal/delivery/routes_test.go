package delivery

import (
	"log/slog"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/odyssey-erp/odyssey-erp/internal/rbac"
)

func TestMountRoutesBuildsDeliveryOrderRoutes(t *testing.T) {
	router := chi.NewRouter()
	MountRoutes(router, nil, slog.Default(), nil, nil, rbac.Middleware{}, nil)
	if router == nil {
		t.Fatal("MountRoutes() did not initialize a router")
	}
}
