package masterdata

import (
	"log/slog"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/odyssey-erp/odyssey-erp/internal/masterdata/branches"
	"github.com/odyssey-erp/odyssey-erp/internal/masterdata/categories"
	"github.com/odyssey-erp/odyssey-erp/internal/masterdata/companies"
	"github.com/odyssey-erp/odyssey-erp/internal/masterdata/products"
	"github.com/odyssey-erp/odyssey-erp/internal/masterdata/suppliers"
	"github.com/odyssey-erp/odyssey-erp/internal/masterdata/taxes"
	"github.com/odyssey-erp/odyssey-erp/internal/masterdata/units"
	"github.com/odyssey-erp/odyssey-erp/internal/masterdata/warehouses"
	"github.com/odyssey-erp/odyssey-erp/internal/rbac"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/view"
)

// Handler manages master data endpoints.
type Handler struct {
	logger           *slog.Logger
	companiesHandler *companies.Handler
	branchesHandler  *branches.Handler
	warehousesHandler *warehouses.Handler
	unitsHandler     *units.Handler
	taxesHandler     *taxes.Handler
	categoriesHandler *categories.Handler
	suppliersHandler *suppliers.Handler
	productsHandler  *products.Handler
}

// NewHandler builds Handler instance.
func NewHandler(logger *slog.Logger, db *pgxpool.Pool, templates *view.Engine, csrf *shared.CSRFManager, sessions *shared.SessionManager, rbac rbac.Middleware) *Handler {
	// Repositories
	companyRepo := companies.NewRepository(db)
	branchRepo := branches.NewRepository(db)
	warehouseRepo := warehouses.NewRepository(db)
	unitRepo := units.NewRepository(db)
	taxRepo := taxes.NewRepository(db)
	categoryRepo := categories.NewRepository(db)
	supplierRepo := suppliers.NewRepository(db)
	productRepo := products.NewRepository(db)

	// Services
	companyService := companies.NewService(companyRepo)
	branchService := branches.NewService(branchRepo)
	warehouseService := warehouses.NewService(warehouseRepo)
	unitService := units.NewService(unitRepo)
	taxService := taxes.NewService(taxRepo)
	categoryService := categories.NewService(categoryRepo)
	supplierService := suppliers.NewService(supplierRepo)
	productService := products.NewService(productRepo)

	// Handlers
	companiesHandler := companies.NewHandler(logger, companyService, templates, csrf, sessions, rbac)
	branchesHandler := branches.NewHandler(logger, branchService, companyService, templates, csrf, sessions, rbac)
	warehousesHandler := warehouses.NewHandler(logger, warehouseService, branchService, templates, csrf, sessions, rbac)
	// (Note: warehouse handler needs branchService for list options)
	
	unitsHandler := units.NewHandler(logger, unitService, templates, csrf, sessions, rbac)
	taxesHandler := taxes.NewHandler(logger, taxService, templates, csrf, sessions, rbac)
	categoriesHandler := categories.NewHandler(logger, categoryService, templates, csrf, sessions, rbac)
	suppliersHandler := suppliers.NewHandler(logger, supplierService, templates, csrf, sessions, rbac)
	
	productsHandler := products.NewHandler(logger, productService, categoryService, unitService, taxService, templates, csrf, sessions, rbac)

	return &Handler{
		logger:           logger,
		companiesHandler: companiesHandler,
		branchesHandler:  branchesHandler,
		warehousesHandler: warehousesHandler,
		unitsHandler:     unitsHandler,
		taxesHandler:     taxesHandler,
		categoriesHandler: categoriesHandler,
		suppliersHandler: suppliersHandler,
		productsHandler:  productsHandler,
	}
}

// MountRoutes registers master data routes.
func (h *Handler) MountRoutes(r chi.Router) {
	r.Route("/companies", func(r chi.Router) {
		h.companiesHandler.MountRoutes(r)
	})
	r.Route("/branches", func(r chi.Router) {
		h.branchesHandler.MountRoutes(r)
	})
	r.Route("/warehouses", func(r chi.Router) {
		h.warehousesHandler.MountRoutes(r)
	})
	r.Route("/units", func(r chi.Router) {
		h.unitsHandler.MountRoutes(r)
	})
	r.Route("/taxes", func(r chi.Router) {
		h.taxesHandler.MountRoutes(r)
	})
	r.Route("/categories", func(r chi.Router) {
		h.categoriesHandler.MountRoutes(r)
	})
	r.Route("/suppliers", func(r chi.Router) {
		h.suppliersHandler.MountRoutes(r)
	})
	r.Route("/products", func(r chi.Router) {
		h.productsHandler.MountRoutes(r)
	})
}
