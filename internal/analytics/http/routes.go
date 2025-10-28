package analytichttp

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/httprate"

	"github.com/odyssey-erp/odyssey-erp/internal/shared"
)

// MountRoutes registers finance analytics endpoints onto the router.
func (h *Handler) MountRoutes(r chi.Router) {
	if h == nil {
		return
	}
	limiter := httprate.Limit(10, time.Minute,
		httprate.WithKeyFuncs(rateLimitKey),
		httprate.WithLimitHandler(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, http.StatusText(http.StatusTooManyRequests), http.StatusTooManyRequests)
		}),
	)

	r.Get("/finance/analytics", h.handleDashboard)
	r.Group(func(gr chi.Router) {
		gr.Use(limiter)
		gr.Get("/finance/analytics/pdf", h.handlePDF)
		gr.Get("/finance/analytics/export.csv", h.handleCSV)
	})
}

func rateLimitKey(r *http.Request) (string, error) {
	sess := shared.SessionFromContext(r.Context())
	if sess != nil {
		if user := strings.TrimSpace(sess.User()); user != "" {
			return "user:" + user, nil
		}
	}
	key, err := httprate.KeyByIP(r)
	if err != nil {
		return "", err
	}
	return "ip:" + key, nil
}
