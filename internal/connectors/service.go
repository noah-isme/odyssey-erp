package connectors

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/sqlc"
)

// Service provides administration and management methods for external connections.
type Service struct {
	pool     *pgxpool.Pool
	queries  sqlc.Querier
	vault    *shared.Vault
	registry *DefaultRegistry
}

// NewService creates a new connectors management service.
func NewService(pool *pgxpool.Pool, vault *shared.Vault, registry *DefaultRegistry) *Service {
	return &Service{
		pool:     pool,
		queries:  sqlc.New(pool),
		vault:    vault,
		registry: registry,
	}
}

// ListConnections returns all connections for the given company.
func (s *Service) ListConnections(ctx context.Context, companyID int64) ([]sqlc.ConnectorConnection, error) {
	return s.queries.ListConnections(ctx, companyID)
}

// CreateConnection creates a new connection, encrypting the secret before storage.
func (s *Service) CreateConnection(ctx context.Context, params CreateConnectionParams) (sqlc.ConnectorConnection, error) {
	cipherText, err := s.vault.EncryptSecure(params.SecretPlaintext)
	if err != nil {
		return sqlc.ConnectorConnection{}, err
	}

	var tokenExpiry pgtype.Timestamptz
	if params.TokenExpiry != nil {
		tokenExpiry = pgtype.Timestamptz{Time: *params.TokenExpiry, Valid: true}
	}

	return s.queries.CreateConnection(ctx, sqlc.CreateConnectionParams{
		CompanyID:   params.CompanyID,
		Provider:    params.Provider,
		Type:        params.Type,
		Name:        params.Name,
		SecretRef:   cipherText,
		Status:      "disabled",
		TokenExpiry: tokenExpiry,
	})
}

// UpdateConnectionStatus updates the status of an existing connection.
func (s *Service) UpdateConnectionStatus(ctx context.Context, companyID int64, connectionID int64, status string) (sqlc.ConnectorConnection, error) {
	return s.queries.UpdateConnectionStatus(ctx, sqlc.UpdateConnectionStatusParams{
		ID:        connectionID,
		CompanyID: companyID,
		Status:    status,
		LastError: pgtype.Text{},
	})
}

// CreateConnectionParams represents the input to create a new connection.
type CreateConnectionParams struct {
	CompanyID       int64
	Provider        string
	Type            string
	Name            string
	SecretPlaintext string
	TokenExpiry     *time.Time
}

type CreateCheckoutIntentRequest struct {
	CompanyID     int64
	ConnectionID  int64
	SourceType    string
	SourceID      int64
	Amount        float64
	Currency      string
	CustomerName  string
	CustomerEmail string
	OrderID       string
}

type CreateCheckoutIntentResult struct {
	PaymentIntentID int64
	Token           string
	RedirectURL     string
}

// CreateCheckoutIntent directly invokes the provider to generate a checkout link and records a Payment Intent.
func (s *Service) CreateCheckoutIntent(ctx context.Context, req CreateCheckoutIntentRequest) (CreateCheckoutIntentResult, error) {
	connRec, err := s.queries.GetConnection(ctx, sqlc.GetConnectionParams{
		ID:        req.ConnectionID,
		CompanyID: req.CompanyID,
	})
	if err != nil {
		return CreateCheckoutIntentResult{}, err
	}

	adapter, err := s.registry.GetAdapter(connRec.Provider)
	if err != nil {
		return CreateCheckoutIntentResult{}, err
	}

	conn := &Connection{
		ID:        connRec.ID,
		CompanyID: connRec.CompanyID,
		Provider:  connRec.Provider,
		Type:      connRec.Type,
		Name:      connRec.Name,
		SecretRef: connRec.SecretRef,
		Status:    ConnectionStatus(connRec.Status),
	}

	// We create a synthetic OutboxCommand payload representing payment.create_checkout
	payloadMap := map[string]any{
		"order_id":       req.OrderID,
		"gross_amount":   int64(req.Amount * 100),
		"customer_name":  req.CustomerName,
		"customer_email": req.CustomerEmail,
	}
	importJSON, _ := json.Marshal(payloadMap)

	cmd := &OutboxCommand{
		CompanyID:    req.CompanyID,
		ConnectionID: req.ConnectionID,
		CommandType:  "payment.create_checkout",
		Payload:      importJSON,
	}

	if err := adapter.ExecuteCommand(ctx, conn, cmd); err != nil {
		return CreateCheckoutIntentResult{}, err
	}

	var result struct {
		Token       string `json:"token"`
		RedirectURL string `json:"redirect_url"`
	}
	if err := json.Unmarshal(cmd.Payload, &result); err != nil {
		return CreateCheckoutIntentResult{}, err
	}

	// Insert Payment Intent
	var amountNum pgtype.Numeric
	_ = amountNum.Scan(fmt.Sprintf("%.2f", req.Amount))
	intent, err := s.queries.CreatePaymentIntent(ctx, sqlc.CreatePaymentIntentParams{
		CompanyID:         req.CompanyID,
		ConnectionID:      req.ConnectionID,
		SourceType:        req.SourceType,
		SourceID:          req.SourceID,
		Amount:            amountNum,
		Currency:          req.Currency,
		Status:            "PENDING",
		ProviderReference: pgtype.Text{String: req.OrderID, Valid: true},
		CheckoutUrl:       pgtype.Text{String: result.RedirectURL, Valid: true},
	})
	if err != nil {
		return CreateCheckoutIntentResult{}, err
	}

	return CreateCheckoutIntentResult{
		PaymentIntentID: intent.ID,
		Token:           result.Token,
		RedirectURL:     result.RedirectURL,
	}, nil
}

// EnqueueInventorySync asynchronously enqueues a command to sync inventory to all commerce connectors.
func (s *Service) EnqueueInventorySync(ctx context.Context, companyID int64, warehouseID int64, productID int64, deltaQty float64) error {
	conns, err := s.queries.ListConnections(ctx, companyID)
	if err != nil {
		return fmt.Errorf("connectors: failed to list connections: %w", err)
	}

	for _, conn := range conns {
		if conn.Status != "active" || conn.Type != "commerce" {
			continue
		}

		payload := map[string]any{
			"warehouse_id": warehouseID,
			"product_id":   productID,
			"delta_qty":    deltaQty,
		}
		
		payloadBytes, _ := json.Marshal(payload)

		_, err := s.queries.EnqueueOutboxCommand(ctx, sqlc.EnqueueOutboxCommandParams{
			CompanyID:     companyID,
			ConnectionID:  conn.ID,
			CommandType:   "ecommerce.inventory.sync",
			CorrelationID: fmt.Sprintf("inv_sync_%d_%d", productID, time.Now().Unix()),
			Payload:       payloadBytes,
		})
		if err != nil {
			return fmt.Errorf("connectors: failed to enqueue inventory sync: %w", err)
		}
	}
	return nil
}

// EnqueueMessage asynchronously enqueues a message (SMS or WhatsApp) via the outbox.
func (s *Service) EnqueueMessage(ctx context.Context, companyID int64, channel string, to string, content string, correlationID string) error {
	conns, err := s.queries.ListConnections(ctx, companyID)
	if err != nil {
		return fmt.Errorf("list connections: %w", err)
	}

	var connID int64
	for _, c := range conns {
		if c.Provider == channel {
			connID = c.ID
			break
		}
	}
	if connID == 0 {
		return fmt.Errorf("connectors: no connection found for provider %q", channel)
	}

	payload := map[string]string{
		"to":      to,
		"content": content,
	}
	payloadBytes, _ := json.Marshal(payload)

	_, err = s.queries.EnqueueOutboxCommand(ctx, sqlc.EnqueueOutboxCommandParams{
		CompanyID:     companyID,
		ConnectionID:  connID,
		CommandType:   "messaging.send",
		CorrelationID: correlationID,
		Payload:       payloadBytes,
	})
	return err
}

// EnqueueBIExport asynchronously enqueues a BI export payload to an object storage provider (e.g. AWS S3).
func (s *Service) EnqueueBIExport(ctx context.Context, companyID int64, provider string, objectKey string, content string, mimeType string, correlationID string) error {
	conns, err := s.queries.ListConnections(ctx, companyID)
	if err != nil {
		return fmt.Errorf("list connections: %w", err)
	}

	var connID int64
	for _, c := range conns {
		if c.Provider == provider {
			connID = c.ID
			break
		}
	}
	if connID == 0 {
		return fmt.Errorf("connectors: no connection found for provider %q", provider)
	}

	payload := map[string]string{
		"object_key": objectKey,
		"content":    content,
		"mime_type":  mimeType,
	}
	payloadBytes, _ := json.Marshal(payload)

	_, err = s.queries.EnqueueOutboxCommand(ctx, sqlc.EnqueueOutboxCommandParams{
		CompanyID:     companyID,
		ConnectionID:  connID,
		CommandType:   "bi.export",
		CorrelationID: correlationID,
		Payload:       payloadBytes,
	})
	return err
}
