package audithttp

import (
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/httprate"
)

const rateLimit = 10
const rateWindow = time.Minute

// MountRoutes mendaftarkan endpoint audit timeline dan ekspor CSV.
func (h *Handler) MountRoutes(r chi.Router) {
	if h == nil {
		return
	}
	limiter := httprate.Limit(rateLimit, rateWindow)
	r.Get("/finance/audit/timeline", h.handleTimeline)
	r.Group(func(gr chi.Router) {
		gr.Use(limiter)
		gr.Get("/finance/audit/timeline/export.csv", h.handleExport)
	})
}
