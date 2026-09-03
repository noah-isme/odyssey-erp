package masterdata

import (
	"log/slog"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/odyssey-erp/odyssey-erp/internal/rbac"
)

func TestNewHandlerBuildsAllMasterDataHandlers(t *testing.T) {
	h := NewHandler(slog.Default(), nil, nil, nil, nil, rbac.Middleware{})
	if h.companiesHandler == nil || h.branchesHandler == nil || h.warehousesHandler == nil ||
		h.unitsHandler == nil || h.taxesHandler == nil || h.categoriesHandler == nil ||
		h.suppliersHandler == nil || h.productsHandler == nil {
		t.Fatalf("NewHandler() did not build all child handlers: %#v", h)
	}
	router := chi.NewRouter()
	h.MountRoutes(router)
	if router == nil {
		t.Fatal("MountRoutes() returned an invalid router")
	}
}
