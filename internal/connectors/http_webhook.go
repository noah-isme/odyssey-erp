package connectors

import (
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
)

// WebhookHandler exposes the HTTP endpoint for incoming webhooks.
type WebhookHandler struct {
	processor *InboxProcessor
}

// NewWebhookHandler creates a new handler.
func NewWebhookHandler(processor *InboxProcessor) *WebhookHandler {
	return &WebhookHandler{
		processor: processor,
	}
}

// MountRoutes attaches the webhook receiving routes.
func (h *WebhookHandler) MountRoutes(r chi.Router) {
	// The route includes the companyID and connectionID to map the request directly to a tenant connection.
	r.Post("/{companyID}/{connectionID}", h.handleWebhook)
}

func (h *WebhookHandler) handleWebhook(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	companyIDStr := chi.URLParam(r, "companyID")
	connectionIDStr := chi.URLParam(r, "connectionID")

	companyID, err := strconv.ParseInt(companyIDStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid company ID", http.StatusBadRequest)
		return
	}

	connectionID, err := strconv.ParseInt(connectionIDStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid connection ID", http.StatusBadRequest)
		return
	}

	// For a real provider, we might extract the providerEventID from headers (like Stripe-Signature or a specific ID header)
	// or parse it from the body. Since this handler is generic and we haven't read the body yet,
	// we will rely on a generic header like "X-Provider-Event-Id" or assume the payload is hashed for a unique ID if missing.
	providerEventID := r.Header.Get("X-Provider-Event-Id")
	if providerEventID == "" {
		// Fallback for providers that don't send explicit event IDs in headers
		providerEventID = "req-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	}

	// Read the raw payload for signature verification
	payload, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	headers := make(map[string]string)
	for k, v := range r.Header {
		if len(v) > 0 {
			headers[k] = v[0]
		}
	}

	// Process the webhook
	err = h.processor.ProcessWebhook(ctx, connectionID, companyID, headers, payload)
	if err != nil {
		h.processor.logger.Error("webhook processing failed", "error", err)
		// We return a 400 or 500. Usually returning 400 prevents providers from aggressively retrying
		// bad signatures, but we want to retry on transient database errors.
		http.Error(w, "webhook processing failed", http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusAccepted)
	_, _ = w.Write([]byte(`{"status":"accepted"}`))
}
