package bankfeeds

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
)

type Handler struct {
	service *Service
	logger  *slog.Logger
	enqueue func(context.Context, int64) error
}

func NewHandler(service *Service, logger *slog.Logger) *Handler {
	return &Handler{
		service: service,
		logger:  logger,
	}
}

// SetEventEnqueuer connects the HTTP inbox to the worker queue after both
// application dependencies have been initialized.
func (h *Handler) SetEventEnqueuer(enqueue func(context.Context, int64) error) {
	h.enqueue = enqueue
}

func (h *Handler) MountRoutes(r chi.Router) {
	r.Post("/webhooks/{provider}", h.handleWebhook)
}

func (h *Handler) handleWebhook(w http.ResponseWriter, r *http.Request) {
	provider := chi.URLParam(r, "provider")
	if provider == "" {
		http.Error(w, "missing provider", http.StatusBadRequest)
		return
	}

	connectionID, err := webhookConnectionID(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	payloadBytes, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 2<<20))
	if err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}
	var payload struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}
	eventType := payload.Type
	if eventType == "" {
		eventType = "unknown"
	}

	headers := make(map[string]string, len(r.Header))
	for key, values := range r.Header {
		if len(values) > 0 {
			headers[key] = values[0]
		}
	}
	event, err := h.service.SaveWebhookEvent(r.Context(), connectionID, provider, eventType, headers, payloadBytes)
	if err != nil {
		if h.logger != nil {
			h.logger.Error("failed to save webhook event", "error", err)
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if h.enqueue != nil {
		if err := h.enqueue(r.Context(), event.ID); err != nil {
			if h.logger != nil {
				h.logger.Error("failed to enqueue webhook event", "event_id", event.ID, "error", err)
			}
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
	}

	w.WriteHeader(http.StatusAccepted)
}

func webhookConnectionID(r *http.Request) (int64, error) {
	value := r.Header.Get("X-Bank-Connection-ID")
	if value == "" {
		value = r.URL.Query().Get("connection_id")
	}
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("valid connection_id is required")
	}
	return id, nil
}
