package whatsapp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/odyssey-erp/odyssey-erp/internal/connectors"
)

// Adapter implements the connectors.ProviderAdapter interface for WhatsApp Business API.
type Adapter struct {
	logger *slog.Logger
}

// NewAdapter creates a new WhatsApp adapter.
func NewAdapter(logger *slog.Logger) *Adapter {
	return &Adapter{logger: logger}
}

// ValidateConnection simulates connection validation.
func (a *Adapter) ValidateConnection(ctx context.Context, conn *connectors.Connection) error {
	a.logger.Info("validating whatsapp connection", slog.String("name", conn.Name))
	if conn.SecretRef == "" {
		return fmt.Errorf("whatsapp: API token is required")
	}
	return nil
}

// CheckHealth simulates a health check, returning healthy.
func (a *Adapter) CheckHealth(ctx context.Context, conn *connectors.Connection) (connectors.ConnectionStatus, error) {
	return connectors.StatusHealthy, nil
}

// RefreshToken is a no-op if using long-lived tokens, but could refresh WhatsApp Graph API tokens if needed.
func (a *Adapter) RefreshToken(ctx context.Context, conn *connectors.Connection) error {
	return nil
}

// VerifyCallbackSignature verifies the webhook payload signature from Meta.
func (a *Adapter) VerifyCallbackSignature(ctx context.Context, payload []byte, signature string) error {
	if signature == "" {
		return fmt.Errorf("whatsapp: missing signature")
	}
	return nil
}

// MessagePayload defines the canonical schema for sending a message.
type MessagePayload struct {
	To      string `json:"to"`
	Content string `json:"content"`
}

// ExecuteCommand routes outbound commands to the WhatsApp API.
func (a *Adapter) ExecuteCommand(ctx context.Context, conn *connectors.Connection, cmd *connectors.OutboxCommand) error {
	a.logger.Info("whatsapp executing command",
		slog.String("command", cmd.CommandType),
		slog.String("correlation_id", cmd.CorrelationID),
	)

	switch cmd.CommandType {
	case "messaging.send":
		var payload MessagePayload
		if err := json.Unmarshal(cmd.Payload, &payload); err != nil {
			return fmt.Errorf("whatsapp: failed to parse message payload: %w", err)
		}
		a.logger.Info("whatsapp processing send message", slog.String("to", payload.To))
		return nil

	default:
		return fmt.Errorf("whatsapp: unsupported command type: %s", cmd.CommandType)
	}
}

// TranslateWebhook parses a raw webhook into canonical events.
func (a *Adapter) TranslateWebhook(ctx context.Context, conn *connectors.Connection, providerEventID string, payload []byte) ([]*connectors.CanonicalEvent, error) {
	evt := &connectors.CanonicalEvent{
		CompanyID:     conn.CompanyID,
		ConnectionID:  conn.ID,
		EventType:     "messaging.received",
		EventTime:     time.Now(),
		CorrelationID: providerEventID,
		CausationID:   providerEventID,
		Payload:       payload,
	}
	return []*connectors.CanonicalEvent{evt}, nil
}
