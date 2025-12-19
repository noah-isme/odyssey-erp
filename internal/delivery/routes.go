// Package delivery provides delivery management functionality.
package delivery

import (
	"log/slog"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/odyssey-erp/odyssey-erp/internal/delivery/orders"
	"github.com/odyssey-erp/odyssey-erp/internal/rbac"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/view"
)

// MountRoutes wires all delivery domain routes.
func MountRoutes(
	r chi.Router,
	pool *pgxpool.Pool,
	logger *slog.Logger,
	templates *view.Engine,
	csrf *shared.CSRFManager,
	rbacMW rbac.Middleware,
) {
	// Orders entity
	ordersRepo := orders.NewRepository(pool)
	ordersSvc := orders.NewService(ordersRepo)
	ordersHandler := orders.NewHandler(logger, ordersSvc, templates, csrf, rbacMW)

	r.Route("/orders", func(r chi.Router) {
		ordersHandler.MountRoutes(r)
	})
}
