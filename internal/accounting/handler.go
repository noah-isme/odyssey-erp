package accounting

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// Handler wires finance ledger endpoints.
type Handler struct {
	logger  *slog.Logger
	service *Service
}

// NewHandler builds a Handler instance.
func NewHandler(logger *slog.Logger, service *Service) *Handler {
	return &Handler{logger: logger, service: service}
}

// MountRoutes registers HTTP routes for the ledger module.
func (h *Handler) MountRoutes(r chi.Router) {
	r.Get("/finance/journals", h.handleNotImplemented)
	r.Post("/finance/journals", h.handleNotImplemented)
	r.Post("/finance/journals/{id}/void", h.handleNotImplemented)
	r.Post("/finance/journals/{id}/reverse", h.handleNotImplemented)
	r.Get("/finance/gl", h.handleNotImplemented)
	r.Get("/finance/reports/trial-balance", h.handleNotImplemented)
	r.Get("/finance/reports/pl", h.handleNotImplemented)
	r.Get("/finance/reports/bs", h.handleNotImplemented)
	r.Get("/finance/reports/trial-balance/pdf", h.handleNotImplemented)
	r.Get("/finance/reports/pl/pdf", h.handleNotImplemented)
	r.Get("/finance/reports/bs/pdf", h.handleNotImplemented)
}

func (h *Handler) handleNotImplemented(w http.ResponseWriter, _ *http.Request) {
	h.logger.Info("ledger handler invoked", slog.String("path", "finance"))
	http.Error(w, http.StatusText(http.StatusNotImplemented), http.StatusNotImplemented)
}
