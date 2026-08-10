package connectors

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/odyssey-erp/odyssey-erp/internal/shared"
)

type ConnectionCreateInput struct {
	CompanyID   int64
	Provider    string
	Type        string
	Name        string
	SecretRef   string
	TokenExpiry *time.Time
}

type PaymentIntentInput struct {
	CompanyID         int64
	ConnectionID      int64
	SourceType        string
	SourceID          int64
	Amount            float64
	Currency          string
	Status            string
	ProviderReference string
	CheckoutURL       string
}

// PaymentIntent is the connector-owned view used for recovery and lifecycle
// updates. It deliberately does not expose SQLC types to callers.
type PaymentIntent struct {
	ID                int64
	CompanyID         int64
	ConnectionID      int64
	SourceType        string
	SourceID          int64
	Amount            float64
	Currency          string
	Status            PaymentStatus
	ProviderReference string
	CheckoutURL       string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type PaymentIntentEventInput struct {
	CompanyID         int64
	ConnectionID      int64
	ProviderReference string
	EventType         string
	ProviderEventID   string
	OccurredAt        time.Time
	RawPayload        []byte
}

// PaymentIntentRepository is optional so existing connector administration
// fakes remain small, while the PostgreSQL repository can provide atomic
// lifecycle updates and timeout recovery.
type PaymentIntentRepository interface {
	GetPaymentIntentByProviderReference(ctx context.Context, companyID, connectionID int64, providerReference string) (PaymentIntent, error)
	UpdatePaymentIntentCheckout(ctx context.Context, companyID, connectionID, intentID int64, status, providerReference, checkoutURL string) error
	ApplyPaymentIntentEvent(ctx context.Context, input PaymentIntentEventInput) (PaymentTransitionResult, error)
}

// PaymentRefundRequest describes a durable refund command. A zero amount
// means full refund; a positive amount requests a partial refund.
type PaymentRefundRequest struct {
	CompanyID       int64
	ConnectionID    int64
	PaymentIntentID int64
	Amount          float64
	Currency        string
	Reason          string
	RefundKey       string
}

type PaymentRefundRequestResult struct {
	RefundID          int64
	OutboxCommandID   int64
	ProviderReference string
}

// PaymentRefundRepository persists a refund request and its provider command
// in one transaction, then advances the refund state as the command and
// provider callback progress.
type PaymentRefundRepository interface {
	RequestPaymentRefund(ctx context.Context, input PaymentRefundRequest) (PaymentRefundRequestResult, error)
	MarkPaymentRefundProcessing(ctx context.Context, companyID, connectionID int64, refundKey string) error
	MarkPaymentRefundFailed(ctx context.Context, companyID, connectionID int64, refundKey string, cause error) error
}

type OutboxEnqueueInput struct {
	CompanyID     int64
	ConnectionID  int64
	CommandType   string
	CorrelationID string
	Payload       []byte
}

type InboxEventInput struct {
	CompanyID       int64
	ConnectionID    int64
	ProviderEventID string
	RawPayload      []byte
}

type CanonicalEventInput struct {
	CompanyID     int64
	ConnectionID  int64
	EventType     string
	EventTime     time.Time
	CorrelationID string
	CausationID   string
	Payload       []byte
}

type OutboxCommandStateUpdate struct {
	ID          int64
	State       string
	NextAttempt time.Time
}

type InboxRepository interface {
	GetConnection(ctx context.Context, companyID, connectionID int64) (Connection, error)
	InsertInboxEvent(ctx context.Context, input InboxEventInput) (InboxEvent, error)
	InsertCanonicalEvent(ctx context.Context, input CanonicalEventInput) (int64, error)
	MarkInboxEventProcessed(ctx context.Context, id int64) error
}

type OutboxRepository interface {
	GetConnection(ctx context.Context, companyID, connectionID int64) (Connection, error)
	GetPendingOutboxCommands(ctx context.Context, limit int32) ([]OutboxCommand, error)
	UpdateOutboxCommandState(ctx context.Context, update OutboxCommandStateUpdate) error
}

// ConnectorDeadLetterWriter records an exhausted connector command without
// changing the generic outbox repository contract used by administration
// fakes.
type ConnectorDeadLetterWriter interface {
	RecordConnectorDeadLetter(context.Context, OutboxCommand, error) error
}

// PaymentRefundStateRepository lets the connector outbox update the durable
// refund request as it is dispatched or dead-lettered.
type PaymentRefundStateRepository interface {
	MarkPaymentRefundProcessing(context.Context, int64, int64, string) error
	MarkPaymentRefundFailed(context.Context, int64, int64, string, error) error
}

// Repository is the persistence boundary for connector administration and
// outbound commands. SQLC types stay inside its PostgreSQL implementation.
type Repository interface {
	ListConnections(ctx context.Context, companyID int64) ([]Connection, error)
	CreateConnection(ctx context.Context, input ConnectionCreateInput) (Connection, error)
	GetConnection(ctx context.Context, companyID, connectionID int64) (Connection, error)
	UpdateConnectionStatus(ctx context.Context, companyID, connectionID int64, status string) (Connection, error)
	CreatePaymentIntent(ctx context.Context, input PaymentIntentInput) (int64, error)
	EnqueueOutboxCommand(ctx context.Context, input OutboxEnqueueInput) (int64, error)
}

// Service provides administration and management methods for external connections.
type Service struct {
	repo     Repository
	vault    *shared.Vault
	registry *DefaultRegistry
}

// NewService creates a new connectors management service.
func NewService(repo Repository, vault *shared.Vault, registry *DefaultRegistry) *Service {
	return &Service{
		repo:     repo,
		vault:    vault,
		registry: registry,
	}
}

// ListConnections returns all connections for the given company.
func (s *Service) ListConnections(ctx context.Context, companyID int64) ([]Connection, error) {
	return s.repo.ListConnections(ctx, companyID)
}

// CreateConnection creates a new connection, encrypting the secret before storage.
func (s *Service) CreateConnection(ctx context.Context, params CreateConnectionParams) (Connection, error) {
	if s.vault == nil {
		return Connection{}, errors.New("connectors: credential vault is unavailable")
	}
	cipherText, err := s.vault.EncryptSecure(params.SecretPlaintext)
	if err != nil {
		return Connection{}, err
	}
	return s.repo.CreateConnection(ctx, ConnectionCreateInput{
		CompanyID:   params.CompanyID,
		Provider:    params.Provider,
		Type:        params.Type,
		Name:        params.Name,
		SecretRef:   cipherText,
		TokenExpiry: params.TokenExpiry,
	})
}

// UpdateConnectionStatus updates the status of an existing connection.
func (s *Service) UpdateConnectionStatus(ctx context.Context, companyID int64, connectionID int64, status string) (Connection, error) {
	return s.repo.UpdateConnectionStatus(ctx, companyID, connectionID, status)
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

type RecoverCheckoutIntentResult struct {
	PaymentIntentID int64
	Status          PaymentStatus
	ProviderStatus  string
	Applied         bool
}

// CreateCheckoutIntent directly invokes the provider to generate a checkout link and records a Payment Intent.
func (s *Service) CreateCheckoutIntent(ctx context.Context, req CreateCheckoutIntentRequest) (CreateCheckoutIntentResult, error) {
	if req.Amount <= 0 || math.IsNaN(req.Amount) || math.IsInf(req.Amount, 0) {
		return CreateCheckoutIntentResult{}, errors.New("connectors: checkout amount must be positive and finite")
	}
	if strings.TrimSpace(req.OrderID) == "" {
		return CreateCheckoutIntentResult{}, errors.New("connectors: checkout order ID is required")
	}
	if strings.TrimSpace(req.Currency) == "" {
		return CreateCheckoutIntentResult{}, errors.New("connectors: checkout currency is required")
	}
	if !strings.EqualFold(strings.TrimSpace(req.Currency), "IDR") {
		return CreateCheckoutIntentResult{}, errors.New("connectors: Midtrans checkout currently supports IDR only")
	}
	minorAmount, err := idrMinorUnits(req.Amount)
	if err != nil {
		return CreateCheckoutIntentResult{}, err
	}
	connRec, err := s.repo.GetConnection(ctx, req.CompanyID, req.ConnectionID)
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

	// Persist the intent before calling the provider. If the provider accepts the
	// request and the HTTP response times out, recovery can look up the same
	// provider reference instead of creating a second checkout.
	intentID, err := s.repo.CreatePaymentIntent(ctx, PaymentIntentInput{
		CompanyID:         req.CompanyID,
		ConnectionID:      req.ConnectionID,
		SourceType:        req.SourceType,
		SourceID:          req.SourceID,
		Amount:            req.Amount,
		Currency:          strings.ToUpper(strings.TrimSpace(req.Currency)),
		Status:            string(PaymentStatusCreated),
		ProviderReference: req.OrderID,
	})
	if err != nil {
		return CreateCheckoutIntentResult{}, fmt.Errorf("connectors: create payment intent: %w", err)
	}

	// We create a synthetic OutboxCommand payload representing payment.create_checkout.
	// Midtrans accepts integer rupiah units, not a float multiplied by 100.
	payloadMap := map[string]any{
		"order_id":       req.OrderID,
		"gross_amount":   minorAmount,
		"currency":       strings.ToUpper(strings.TrimSpace(req.Currency)),
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
		return CreateCheckoutIntentResult{PaymentIntentID: intentID}, err
	}

	var result struct {
		Token       string `json:"token"`
		RedirectURL string `json:"redirect_url"`
	}
	if err := json.Unmarshal(cmd.Payload, &result); err != nil {
		return CreateCheckoutIntentResult{PaymentIntentID: intentID}, err
	}

	if paymentRepo, ok := s.repo.(PaymentIntentRepository); ok {
		if err := paymentRepo.UpdatePaymentIntentCheckout(ctx, req.CompanyID, req.ConnectionID, intentID, string(PaymentStatusPending), req.OrderID, result.RedirectURL); err != nil {
			return CreateCheckoutIntentResult{PaymentIntentID: intentID, Token: result.Token, RedirectURL: result.RedirectURL}, fmt.Errorf("connectors: persist checkout intent: %w", err)
		}
	}

	return CreateCheckoutIntentResult{
		PaymentIntentID: intentID,
		Token:           result.Token,
		RedirectURL:     result.RedirectURL,
	}, nil
}

// RecoverCheckoutIntent resolves an ambiguous provider call through a status
// lookup and applies the result through the same monotonic lifecycle reducer
// used by callbacks. No new checkout or refund request is sent.
func (s *Service) RecoverCheckoutIntent(ctx context.Context, companyID, connectionID int64, orderID string) (RecoverCheckoutIntentResult, error) {
	if strings.TrimSpace(orderID) == "" {
		return RecoverCheckoutIntentResult{}, errors.New("connectors: recovery order ID is required")
	}
	connRec, err := s.repo.GetConnection(ctx, companyID, connectionID)
	if err != nil {
		return RecoverCheckoutIntentResult{}, err
	}
	adapter, err := s.registry.GetAdapter(connRec.Provider)
	if err != nil {
		return RecoverCheckoutIntentResult{}, err
	}
	lookup, ok := adapter.(PaymentStatusLookup)
	if !ok {
		return RecoverCheckoutIntentResult{}, fmt.Errorf("connectors: provider %q does not support payment status recovery", connRec.Provider)
	}
	snapshot, err := lookup.LookupPaymentStatus(ctx, &connRec, orderID)
	if err != nil {
		return RecoverCheckoutIntentResult{}, err
	}
	result := RecoverCheckoutIntentResult{ProviderStatus: snapshot.Status}
	if paymentRepo, ok := s.repo.(PaymentIntentRepository); ok {
		intent, err := paymentRepo.GetPaymentIntentByProviderReference(ctx, companyID, connectionID, orderID)
		if err != nil {
			return RecoverCheckoutIntentResult{}, err
		}
		result.PaymentIntentID = intent.ID
		transition, err := paymentRepo.ApplyPaymentIntentEvent(ctx, PaymentIntentEventInput{
			CompanyID:         companyID,
			ConnectionID:      connectionID,
			ProviderReference: orderID,
			EventType:         snapshot.EventType,
			ProviderEventID:   "status-" + orderID + "-" + snapshot.EventType,
			OccurredAt:        snapshot.OccurredAt,
		})
		if err != nil {
			return RecoverCheckoutIntentResult{}, err
		}
		result.Applied = transition.Applied
		result.Status = transition.ToStatus
		if !result.Applied {
			result.Status = intent.Status
		}
	}
	return result, nil
}

// RequestPaymentRefund records a PENDING refund before placing its provider
// command in the durable connector outbox. Retries reuse RefundKey and cannot
// create a second refund request for the same payment intent.
func (s *Service) RequestPaymentRefund(ctx context.Context, input PaymentRefundRequest) (PaymentRefundRequestResult, error) {
	if input.CompanyID <= 0 || input.ConnectionID <= 0 || input.PaymentIntentID <= 0 {
		return PaymentRefundRequestResult{}, errors.New("connectors: refund company, connection, and payment intent are required")
	}
	if input.Amount < 0 || math.IsNaN(input.Amount) || math.IsInf(input.Amount, 0) {
		return PaymentRefundRequestResult{}, errors.New("connectors: refund amount cannot be negative or non-finite")
	}
	if strings.TrimSpace(input.RefundKey) == "" {
		return PaymentRefundRequestResult{}, errors.New("connectors: refund key is required")
	}
	repo, ok := s.repo.(PaymentRefundRepository)
	if !ok {
		return PaymentRefundRequestResult{}, errors.New("connectors: refund persistence is not configured")
	}
	return repo.RequestPaymentRefund(ctx, input)
}

func idrMinorUnits(amount float64) (int64, error) {
	rounded := math.Round(amount)
	if rounded <= 0 || math.Abs(amount-rounded) > 1e-9 || rounded > math.MaxInt64 {
		return 0, errors.New("connectors: IDR checkout amount must be a positive whole rupiah amount")
	}
	return int64(rounded), nil
}

// EnqueueInventorySync asynchronously enqueues a command to sync inventory to all commerce connectors.
func (s *Service) EnqueueInventorySync(ctx context.Context, companyID int64, warehouseID int64, productID int64, deltaQty float64) error {
	conns, err := s.repo.ListConnections(ctx, companyID)
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

		_, err := s.repo.EnqueueOutboxCommand(ctx, OutboxEnqueueInput{
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
	conns, err := s.repo.ListConnections(ctx, companyID)
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

	_, err = s.repo.EnqueueOutboxCommand(ctx, OutboxEnqueueInput{
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
	conns, err := s.repo.ListConnections(ctx, companyID)
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

	_, err = s.repo.EnqueueOutboxCommand(ctx, OutboxEnqueueInput{
		CompanyID:     companyID,
		ConnectionID:  connID,
		CommandType:   "bi.export",
		CorrelationID: correlationID,
		Payload:       payloadBytes,
	})
	return err
}
