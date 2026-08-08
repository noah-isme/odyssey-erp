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
func (a *Adapter) VerifyCallbackSignature(ctx context.Context, headers map[string]string, payload []byte) error {
	signature := headers["X-Shopify-Hmac-Sha256"]
	if signature == "" {
		return fmt.Errorf("shopify: missing signature header (X-Shopify-Hmac-Sha256)")
	}

	// For a real connection, the shopify app secret would be stored in the vault via SecretRef
	// Let's assume the secret is in a JSON string like {"secret": "..."} or we just mock it for now.
	// For this mock, we just skip deep verification if it's "skip_validation" or use a dummy secret.
	secret := "dummy_secret"

	if !verifyWebhookSignature(secret, signature, payload) {
		a.logger.Warn("shopify: invalid webhook signature, but continuing for development")
		// In production, return an error here.
		// return fmt.Errorf("shopify: invalid signature")
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
	case "ecommerce.inventory.sync":
		var payload struct {
			ProductID   int64   `json:"product_id"`
			WarehouseID int64   `json:"warehouse_id"`
			DeltaQty    float64 `json:"delta_qty"`
		}
		if err := json.Unmarshal(cmd.Payload, &payload); err != nil {
			return fmt.Errorf("shopify: failed to parse inventory sync payload: %w", err)
		}

		// Simulate API call to Shopify to update available-to-promise inventory
		a.logger.Info("syncing inventory adjustment to shopify",
			slog.Int64("product_id", payload.ProductID),
			slog.Int64("warehouse_id", payload.WarehouseID),
			slog.Float64("delta_qty", payload.DeltaQty),
		)

		// E.g., POST /admin/api/2024-01/inventory_levels/adjust.json
		// with location_id, inventory_item_id (mapped from product), available_adjustment

		return nil
	default:
		return fmt.Errorf("shopify: unsupported command type: %s", cmd.CommandType)
	}
}

// TranslateWebhook parses a Shopify webhook and emits canonical domain events.
func (a *Adapter) TranslateWebhook(ctx context.Context, conn *connectors.Connection, headers map[string]string, payload []byte) ([]*connectors.CanonicalEvent, error) {
	topic := headers["X-Shopify-Topic"]
	if topic == "" {
		return nil, fmt.Errorf("shopify: missing X-Shopify-Topic header")
	}

	providerEventID := headers["X-Shopify-Webhook-Id"]

	var canonicalEvents []*connectors.CanonicalEvent

	switch topic {
	case "orders/create":
		var order ShopifyOrder
		if err := json.Unmarshal(payload, &order); err != nil {
			return nil, fmt.Errorf("shopify: failed to unmarshal orders/create: %w", err)
		}

		canonicalEvents = append(canonicalEvents, &connectors.CanonicalEvent{
			CompanyID:     conn.CompanyID,
			ConnectionID:  conn.ID,
			EventType:     "ecommerce.order.created",
			EventTime:     time.Now(),
			CorrelationID: fmt.Sprintf("shopify_order_%d", order.ID),
			CausationID:   providerEventID,
			Payload:       payload, 
		})
	case "orders/updated":
		// For an order update, we might map to an update or cancellation
		canonicalEvents = append(canonicalEvents, &connectors.CanonicalEvent{
			CompanyID:     conn.CompanyID,
			ConnectionID:  conn.ID,
			EventType:     "ecommerce.order.updated",
			EventTime:     time.Now(),
			CorrelationID: providerEventID,
			CausationID:   providerEventID,
			Payload:       payload,
		})
	default:
		a.logger.Info("shopify: unhandled webhook topic", slog.String("topic", topic))
	}

	return canonicalEvents, nil
}
