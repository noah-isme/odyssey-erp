package dashboard

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/view"
)

// Handler serves dashboard routes.
type Handler struct {
	logger    *slog.Logger
	service   *Service
	templates *view.Engine
	csrf      *shared.CSRFManager
}

// NewHandler creates a dashboard handler.
func NewHandler(logger *slog.Logger, service *Service, templates *view.Engine, csrf *shared.CSRFManager) *Handler {
	return &Handler{
		logger:    logger,
		service:   service,
		templates: templates,
		csrf:      csrf,
	}
}

// MountRoutes mounts dashboard API routes.
func (h *Handler) MountRoutes(r chi.Router) {
	r.Get("/api/dashboard/kpis", h.handleGetKPIs)
	r.Get("/api/dashboard/activity", h.handleGetActivity)
}

// handleGetKPIs returns KPI data as JSON.
func (h *Handler) handleGetKPIs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	companyID := int64(1)

	kpis, err := h.service.GetKPIs(ctx, companyID)
	if err != nil {
		h.logger.Error("failed to get KPIs", slog.Any("error", err))
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(kpis); err != nil {
		h.logger.Error("encode kpis", slog.Any("error", err))
	}
}

// handleGetActivity returns recent activity as JSON.
func (h *Handler) handleGetActivity(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	activities, err := h.service.GetRecentActivity(ctx, 10)
	if err != nil {
		h.logger.Error("failed to get activity", slog.Any("error", err))
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(activities); err != nil {
		h.logger.Error("encode activities", slog.Any("error", err))
	}
}
