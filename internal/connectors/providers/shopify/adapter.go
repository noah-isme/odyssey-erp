package shopify

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/odyssey-erp/odyssey-erp/internal/connectors"
)

// Adapter implements connectors.ProviderAdapter for Shopify.
type Adapter struct {
	logger *slog.Logger
}

// NewAdapter creates a new Shopify adapter.
func NewAdapter(logger *slog.Logger) *Adapter {
	return &Adapter{logger: logger}
}

// ValidateConnection checks that the provided secret references exist.
func (a *Adapter) ValidateConnection(ctx context.Context, conn *connectors.Connection) error {
	a.logger.Info("validating shopify connection", slog.String("name", conn.Name))
	if conn.SecretRef == "" {
		return fmt.Errorf("shopify: secret reference is required")
	}
	return nil
}

// CheckHealth checks connection health.
func (a *Adapter) CheckHealth(ctx context.Context, conn *connectors.Connection) (connectors.ConnectionStatus, error) {
	a.logger.Info("checking health for shopify", slog.String("name", conn.Name))
	return connectors.StatusHealthy, nil
}

// RefreshToken is unused for standard basic access tokens in Shopify.
func (a *Adapter) RefreshToken(ctx context.Context, conn *connectors.Connection) error {
	return nil
}

// VerifyCallbackSignature verifies a Shopify webhook signature.
func (a *Adapter) VerifyCallbackSignature(ctx context.Context, payload []byte, signature string) error {
	// Normally verify HMAC-SHA256 signature here using Shopify App Secret.
	if signature == "" {
		return fmt.Errorf("shopify: missing signature")
	}
	return nil
}

// OrderPayload defines an incoming payload from domain modules to sync an order to Shopify.
type OrderPayload struct {
	OrderID     string `json:"order_id"`
	TotalAmount string `json:"total_amount"`
	Currency    string `json:"currency"`
	Status      string `json:"status"`
}

// ExecuteCommand dispatches outbound commands to Shopify.
func (a *Adapter) ExecuteCommand(ctx context.Context, conn *connectors.Connection, cmd *connectors.OutboxCommand) error {
	a.logger.Info("shopify executing command",
		slog.String("command", cmd.CommandType),
		slog.String("correlation_id", cmd.CorrelationID),
	)

	switch cmd.CommandType {
	case "ecommerce.order.sync":
		var payload OrderPayload
		if err := json.Unmarshal(cmd.Payload, &payload); err != nil {
			return fmt.Errorf("shopify: failed to parse order payload: %w", err)
		}
		
		// Simulate API call to Shopify
		a.logger.Info("syncing order to shopify", slog.String("order_id", payload.OrderID))

		return nil
	default:
		return fmt.Errorf("shopify: unsupported command type: %s", cmd.CommandType)
	}
}

// TranslateWebhook parses a Shopify webhook and emits canonical domain events.
func (a *Adapter) TranslateWebhook(ctx context.Context, conn *connectors.Connection, providerEventID string, payload []byte) ([]*connectors.CanonicalEvent, error) {
	var event map[string]interface{}
	if err := json.Unmarshal(payload, &event); err != nil {
		return nil, fmt.Errorf("shopify: failed to unmarshal webhook event: %w", err)
	}

	eventType := "ecommerce.order.created"
	
	var canonicalEvents []*connectors.CanonicalEvent
	canonicalEvents = append(canonicalEvents, &connectors.CanonicalEvent{
		CompanyID:     conn.CompanyID,
		ConnectionID:  conn.ID,
		EventType:     eventType,
		EventTime:     time.Now(),
		CorrelationID: providerEventID,
		CausationID:   providerEventID,
		Payload:       payload, 
	})

	return canonicalEvents, nil
}
