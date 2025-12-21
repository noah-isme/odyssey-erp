package sales

import (
	"log/slog"

	"github.com/go-chi/chi/v5"
	"github.com/odyssey-erp/odyssey-erp/internal/rbac"
	"github.com/odyssey-erp/odyssey-erp/internal/sales/customers"
	"github.com/odyssey-erp/odyssey-erp/internal/sales/orders"
	"github.com/odyssey-erp/odyssey-erp/internal/sales/quotations"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/view"
)

type Handler struct {
	customers  *customers.Handler
	quotations *quotations.Handler
	orders     *orders.Handler
}

func NewHandler(
	logger *slog.Logger,
	service *Service,
	templates *view.Engine,
	csrf *shared.CSRFManager,
	sessions *shared.SessionManager,
	rbac rbac.Middleware,
) *Handler {
	// Initializing sub-handlers
	// Note: sessions manager is not passed to sub-handlers as they use request context,
	// but kept in signature to match main.go
	
	h := &Handler{
		customers: customers.NewHandler(
			logger,
			service.Customers,
			templates,
			csrf,
			rbac,
		),
		quotations: quotations.NewHandler(
			logger,
			service.Quotations,
			service.Customers,
			service.Products,
			templates,
			csrf,
			rbac,
		),
		orders: orders.NewHandler(
			logger,
			service.Orders,
			service.Customers,
			service.Quotations,
			service.Products,
			templates,
			csrf,
			rbac,
		),
	}
	return h
}

func (h *Handler) MountRoutes(r chi.Router) {
	// Mount sub-routes
	h.customers.MountRoutes(r)
	h.quotations.MountRoutes(r)
	h.orders.MountRoutes(r)
}
