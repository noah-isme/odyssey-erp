package report

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

// Handler manages report endpoints.
type Handler struct {
	client *Client
	logger *slog.Logger
}

// NewHandler creates a report handler.
func NewHandler(client *Client, logger *slog.Logger) *Handler {
	return &Handler{client: client, logger: logger}
}

// MountRoutes registers report routes.
func (h *Handler) MountRoutes(r chi.Router) {
	r.Get("/ping", h.ping)
	r.Post("/sample", h.sample)
}

func (h *Handler) ping(w http.ResponseWriter, r *http.Request) {
	if err := h.client.Ping(r.Context()); err != nil {
		h.logger.Warn("gotenberg ping failed", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

func (h *Handler) sample(w http.ResponseWriter, r *http.Request) {
	html := "" +
		"<html><head><title>Odyssey Report</title></head><body>" +
		"<h1>Odyssey ERP</h1><p>Generated at " + time.Now().Format(time.RFC1123) + "</p>" +
		"</body></html>"
	pdf, err := h.client.RenderHTML(r.Context(), html)
	if err != nil {
		h.logger.Error("render sample pdf", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusBadGateway), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", "inline; filename=sample.pdf")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(pdf)
}
