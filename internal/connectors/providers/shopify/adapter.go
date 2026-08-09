package shopify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/odyssey-erp/odyssey-erp/internal/connectors"
)

// Adapter implements connectors.ProviderAdapter for Shopify.
type Adapter struct {
	logger  *slog.Logger
	options connectors.ProviderOptions
}

func NewAdapter(logger *slog.Logger, options ...connectors.ProviderOptions) *Adapter {
	var opts connectors.ProviderOptions
	if len(options) > 0 {
		opts = options[0]
	}
	return &Adapter{logger: logger, options: opts}
}

type Credentials struct {
	ShopURL       string `json:"shop_url"`
	AccessToken   string `json:"access_token"`
	AppSecret     string `json:"app_secret"`
	WebhookSecret string `json:"webhook_secret"`
}

func (a *Adapter) credentials(conn *connectors.Connection) (Credentials, error) {
	secret, err := a.options.ResolveSecret(conn)
	if err != nil {
		return Credentials{}, err
	}
	var creds Credentials
	if err := json.Unmarshal([]byte(secret), &creds); err != nil {
		return Credentials{}, fmt.Errorf("shopify: invalid credential format: %w", err)
	}
	if strings.TrimSpace(creds.ShopURL) == "" || strings.TrimSpace(creds.AccessToken) == "" {
		return Credentials{}, errors.New("shopify: shop_url and access_token are required")
	}
	parsed, err := url.Parse(creds.ShopURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return Credentials{}, errors.New("shopify: shop_url must be an absolute URL")
	}
	if !a.options.DevelopmentMode && parsed.Scheme != "https" {
		return Credentials{}, errors.New("shopify: production shop_url must use HTTPS")
	}
	return creds, nil
}

func (a *Adapter) ValidateConnection(ctx context.Context, conn *connectors.Connection) error {
	if _, err := a.credentials(conn); err != nil {
		return fmt.Errorf("shopify: validate credentials: %w", err)
	}
	return nil
}

func (a *Adapter) CheckHealth(ctx context.Context, conn *connectors.Connection) (connectors.ConnectionStatus, error) {
	creds, err := a.credentials(conn)
	if err != nil {
		return connectors.StatusActionRequired, err
	}
	if a.options.DevelopmentMode {
		return connectors.StatusHealthy, nil
	}
	if err := NewClientWithOptions(creds.ShopURL, creds.AccessToken, a.options).CheckHealth(ctx); err != nil {
		return connectors.StatusActionRequired, err
	}
	return connectors.StatusHealthy, nil
}

func (a *Adapter) RefreshToken(ctx context.Context, conn *connectors.Connection) error {
	return connectors.ErrUnsupportedOperation
}

func (a *Adapter) VerifyCallbackSignature(ctx context.Context, conn *connectors.Connection, headers map[string]string, payload []byte) error {
	creds, err := a.credentials(conn)
	if err != nil {
		return err
	}
	secret := creds.WebhookSecret
	if secret == "" {
		secret = creds.AppSecret
	}
	if strings.TrimSpace(secret) == "" {
		return errors.New("shopify: webhook_secret or app_secret is required for callbacks")
	}
	signature := connectors.Header(headers, "X-Shopify-Hmac-Sha256")
	if signature == "" {
		return errors.New("shopify: missing X-Shopify-Hmac-Sha256 header")
	}
	if !verifyWebhookSignature(secret, signature, payload) {
		return errors.New("shopify: invalid webhook signature")
	}
	return nil
}

type OrderPayload struct {
	OrderID       string             `json:"order_id"`
	TotalAmount   string             `json:"total_amount"`
	Currency      string             `json:"currency"`
	Status        string             `json:"status"`
	CustomerEmail string             `json:"customer_email,omitempty"`
	LineItems     []ShopifyOrderLine `json:"line_items"`
}

type ShopifyOrderLine struct {
	VariantID int64  `json:"variant_id,omitempty"`
	Title     string `json:"title,omitempty"`
	Quantity  int    `json:"quantity"`
	Price     string `json:"price,omitempty"`
}

type InventorySyncPayload struct {
	ProductID       int64 `json:"product_id"`
	WarehouseID     int64 `json:"warehouse_id"`
	LocationID      int64 `json:"location_id"`
	InventoryItemID int64 `json:"inventory_item_id"`
	Available       *int  `json:"available"`
}

func (a *Adapter) ExecuteCommand(ctx context.Context, conn *connectors.Connection, cmd *connectors.OutboxCommand) error {
	if cmd == nil {
		return errors.New("shopify: command is required")
	}
	if a.options.DevelopmentMode && strings.TrimSpace(conn.SecretRef) == "" {
		a.logger.Info("Shopify command simulated in explicit development mode", slog.String("command", cmd.CommandType))
		return nil
	}
	creds, err := a.credentials(conn)
	if err != nil {
		return err
	}
	client := NewClientWithOptions(creds.ShopURL, creds.AccessToken, a.options)
	key := cmd.CorrelationID
	if cmd.ID > 0 {
		key = fmt.Sprintf("odyssey-command-%d", cmd.ID)
	}
	switch cmd.CommandType {
	case "ecommerce.order.sync":
		var payload OrderPayload
		if err := json.Unmarshal(cmd.Payload, &payload); err != nil {
			return fmt.Errorf("shopify: failed to parse order payload: %w", err)
		}
		return client.CreateOrder(ctx, payload, key)
	case "ecommerce.inventory.sync":
		var payload InventorySyncPayload
		if err := json.Unmarshal(cmd.Payload, &payload); err != nil {
			return fmt.Errorf("shopify: failed to parse inventory sync payload: %w", err)
		}
		if payload.Available == nil {
			return errors.New("shopify: provider available quantity is required; map local inventory before enqueueing")
		}
		return client.UpdateInventory(ctx, payload.LocationID, payload.InventoryItemID, *payload.Available, key)
	default:
		return fmt.Errorf("shopify: unsupported command type: %s", cmd.CommandType)
	}
}

func (a *Adapter) TranslateWebhook(ctx context.Context, conn *connectors.Connection, headers map[string]string, payload []byte) ([]*connectors.CanonicalEvent, error) {
	topic := connectors.Header(headers, "X-Shopify-Topic")
	if topic == "" {
		return nil, errors.New("shopify: missing X-Shopify-Topic header")
	}
	providerEventID := connectors.Header(headers, "X-Shopify-Webhook-Id")
	if providerEventID == "" {
		return nil, errors.New("shopify: missing X-Shopify-Webhook-Id header")
	}

	var eventType, correlationID string
	var eventTime = time.Now().UTC()
	switch topic {
	case "orders/create":
		var order ShopifyOrder
		if err := json.Unmarshal(payload, &order); err != nil {
			return nil, fmt.Errorf("shopify: failed to unmarshal orders/create: %w", err)
		}
		if order.ID <= 0 {
			return nil, errors.New("shopify: orders/create payload is missing order id")
		}
		eventType = "ecommerce.order.created"
		correlationID = fmt.Sprintf("shopify_order_%d", order.ID)
		if parsed, err := time.Parse(time.RFC3339, order.CreatedAt); err == nil {
			eventTime = parsed.UTC()
		}
	case "orders/updated":
		eventType = "ecommerce.order.updated"
		correlationID = providerEventID
	default:
		a.logger.Info("shopify: unhandled webhook topic", slog.String("topic", topic))
		return nil, nil
	}
	return []*connectors.CanonicalEvent{{
		CompanyID:     conn.CompanyID,
		ConnectionID:  conn.ID,
		EventType:     eventType,
		EventTime:     eventTime,
		CorrelationID: correlationID,
		CausationID:   providerEventID,
		Payload:       payload,
	}}, nil
}
