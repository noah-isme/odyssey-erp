package bankfeeds

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
)

type Handler struct {
	service *Service
	logger  *slog.Logger
}

func NewHandler(service *Service, logger *slog.Logger) *Handler {
	return &Handler{
		service: service,
		logger:  logger,
	}
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

	var payload map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}
	
	eventType, _ := payload["type"].(string)
	if eventType == "" {
		eventType = "unknown"
	}

	payloadBytes, _ := json.Marshal(payload)

	err := h.service.SaveWebhookEvent(r.Context(), provider, eventType, payloadBytes)
	if err != nil {
		h.logger.Error("failed to save webhook event", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}
