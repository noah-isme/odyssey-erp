package insightshhtp

import "github.com/go-chi/chi/v5"

// MountRoutes mendaftarkan endpoint finance insights.
func (h *Handler) MountRoutes(r chi.Router) {
	if h == nil {
		return
	}
	r.Get("/insights", h.handleInsights)
}
