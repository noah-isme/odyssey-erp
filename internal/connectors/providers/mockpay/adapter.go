package mockpay

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/odyssey-erp/odyssey-erp/internal/connectors"
)

// Adapter implements the connectors.ProviderAdapter interface for a mock payment gateway.
type Adapter struct {
	logger *slog.Logger
}

// NewAdapter creates a new mock payment gateway adapter.
func NewAdapter(logger *slog.Logger) *Adapter {
	return &Adapter{logger: logger}
}

// ValidateConnection simulates connection validation.
func (a *Adapter) ValidateConnection(ctx context.Context, conn *connectors.Connection) error {
	a.logger.Info("validating mockpay connection", slog.String("name", conn.Name))
	if conn.SecretRef == "" {
		return fmt.Errorf("mockpay: secret reference is required")
	}
	return nil
}

// CheckHealth simulates a health check, always returning healthy.
func (a *Adapter) CheckHealth(ctx context.Context, conn *connectors.Connection) (connectors.ConnectionStatus, error) {
	a.logger.Info("checking health for mockpay", slog.String("name", conn.Name))
	return connectors.StatusHealthy, nil
}

// RefreshToken is a no-op since mockpay doesn't use OAuth.
func (a *Adapter) RefreshToken(ctx context.Context, conn *connectors.Connection) error {
	return nil
}

// VerifyCallbackSignature simulates signature validation.
func (a *Adapter) VerifyCallbackSignature(ctx context.Context, payload []byte, signature string) error {
	if signature == "" {
		return fmt.Errorf("mockpay: missing signature")
	}
	return nil
}

// PaymentChargePayload defines the canonical schema for a charge command.
type PaymentChargePayload struct {
	Amount   float64 `json:"amount"`
	Currency string  `json:"currency"`
	Source   string  `json:"source"`
}

// ExecuteCommand routes outbound commands to the correct mock simulation.
func (a *Adapter) ExecuteCommand(ctx context.Context, conn *connectors.Connection, cmd *connectors.OutboxCommand) error {
	a.logger.Info("mockpay executing command",
		slog.String("command", cmd.CommandType),
		slog.String("correlation_id", cmd.CorrelationID),
	)

	switch cmd.CommandType {
	case "payment.charge":
		var payload PaymentChargePayload
		if err := json.Unmarshal(cmd.Payload, &payload); err != nil {
			return fmt.Errorf("mockpay: failed to parse charge payload: %w", err)
		}

		a.logger.Info("mockpay processing charge",
			slog.Float64("amount", payload.Amount),
			slog.String("currency", payload.Currency),
		)

		// Simulating successful network execution
		return nil

	default:
		return fmt.Errorf("mockpay: unsupported command type: %s", cmd.CommandType)
	}
}

// TranslateWebhook parses a raw provider webhook into zero or more canonical events.
func (a *Adapter) TranslateWebhook(ctx context.Context, conn *connectors.Connection, providerEventID string, payload []byte) ([]*connectors.CanonicalEvent, error) {
	// For mockpay, we just simulate translating a successful charge event
	evt := &connectors.CanonicalEvent{
		CompanyID:     conn.CompanyID,
		ConnectionID:  conn.ID,
		EventType:     "payment.captured",
		EventTime:     time.Now(),
		CorrelationID: providerEventID, // Usually a provider's ID or metadata
		CausationID:   providerEventID,
		Payload:       payload, // Just pass the raw payload through for the stub
	}
	return []*connectors.CanonicalEvent{evt}, nil
}
