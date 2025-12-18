package audithttp

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/httprate"

	"github.com/odyssey-erp/odyssey-erp/internal/shared"
)

const rateLimit = 10
const rateWindow = time.Minute

// MountRoutes mendaftarkan endpoint audit timeline dan ekspor CSV.
func (h *Handler) MountRoutes(r chi.Router) {
	if h == nil {
		return
	}
	limiter := httprate.Limit(rateLimit, rateWindow,
		httprate.WithKeyFuncs(rateLimitKey),
		httprate.WithLimitHandler(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, http.StatusText(http.StatusTooManyRequests), http.StatusTooManyRequests)
		}),
	)
	r.Get("/audit", h.handleTimeline)
	r.Group(func(gr chi.Router) {
		gr.Use(limiter)
		gr.Get("/audit/export.csv", h.handleExport)
		gr.Get("/audit/pdf", h.handlePDF)
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
