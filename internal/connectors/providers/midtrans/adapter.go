package midtrans

import (
	"context"
	"crypto/sha512"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/odyssey-erp/odyssey-erp/internal/connectors"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
)

// Adapter implements the connectors.ProviderAdapter interface for Midtrans.
type Adapter struct {
	logger  *slog.Logger
	options connectors.ProviderOptions
}

// NewAdapter keeps the original vault argument for compatibility while
// allowing the application to pass shared provider transport options. The
// options form is used by production wiring and contract tests.
func NewAdapter(logger *slog.Logger, vault *shared.Vault, options ...connectors.ProviderOptions) *Adapter {
	var opts connectors.ProviderOptions
	if len(options) > 0 {
		opts = options[0]
	}
	if opts.Vault == nil {
		opts.Vault = vault
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Adapter{logger: logger, options: opts}
}

// Credentials is the encrypted connection payload for Midtrans. Production
// must opt into the live endpoints explicitly with is_prod=true; the default
// is the sandbox so a newly created connection cannot send live transactions
// accidentally.
type Credentials struct {
	ServerKey string `json:"server_key"`
	IsProd    bool   `json:"is_prod,omitempty"`
	BaseURL   string `json:"base_url,omitempty"`
}

func (a *Adapter) credentials(conn *connectors.Connection) (Credentials, error) {
	secret, err := a.options.ResolveSecret(conn)
	if err != nil {
		return Credentials{}, err
	}
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return Credentials{}, errors.New("midtrans: server_key is required")
	}

	var creds Credentials
	if err := json.Unmarshal([]byte(secret), &creds); err != nil {
		// Raw server keys are retained for backwards-compatible vaulted
		// connections. New production connections should use the structured
		// payload so is_prod can be explicit.
		if strings.HasPrefix(secret, "{") {
			return Credentials{}, fmt.Errorf("midtrans: invalid credential format: %w", err)
		}
		creds.ServerKey = secret
	}
	if strings.TrimSpace(creds.ServerKey) == "" {
		return Credentials{}, errors.New("midtrans: server_key is required")
	}
	if creds.BaseURL != "" {
		parsed, parseErr := url.Parse(creds.BaseURL)
		if parseErr != nil || parsed.Scheme == "" || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			return Credentials{}, errors.New("midtrans: base_url must be an absolute HTTP(S) URL")
		}
		if !a.options.DevelopmentMode && parsed.Scheme != "https" {
			return Credentials{}, errors.New("midtrans: production base_url must use HTTPS")
		}
	}
	return creds, nil
}

func (a *Adapter) client(creds Credentials) *Client {
	options := a.options
	if strings.TrimSpace(creds.BaseURL) != "" {
		options.BaseURL = creds.BaseURL
	}
	return NewClientWithOptions(creds.ServerKey, creds.IsProd, options)
}

func (a *Adapter) ValidateConnection(ctx context.Context, conn *connectors.Connection) error {
	if _, err := a.credentials(conn); err != nil {
		return fmt.Errorf("midtrans: validate credentials: %w", err)
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

	orderID := "odyssey-health-check"
	if conn != nil && conn.ID > 0 {
		orderID = fmt.Sprintf("odyssey-health-check-%d", conn.ID)
	}
	statusCode, body, err := a.client(creds).ProbeTransactionStatus(ctx, orderID)
	if err != nil {
		return connectors.StatusActionRequired, err
	}
	if statusCode == 404 || (statusCode >= 200 && statusCode < 300) {
		return connectors.StatusHealthy, nil
	}
	return connectors.StatusActionRequired, fmt.Errorf("midtrans: health check returned %d: %s", statusCode, responseMessage(body))
}

func (a *Adapter) RefreshToken(ctx context.Context, conn *connectors.Connection) error {
	// Midtrans server keys do not use an OAuth refresh flow.
	return nil
}

func (a *Adapter) VerifyCallbackSignature(ctx context.Context, conn *connectors.Connection, headers map[string]string, payload []byte) error {
	var notif WebhookNotification
	if err := json.Unmarshal(payload, &notif); err != nil {
		return fmt.Errorf("midtrans: invalid webhook payload: %w", err)
	}
	creds, err := a.credentials(conn)
	if err != nil {
		return err
	}
	if !notif.VerifySignature(creds.ServerKey) {
		return errors.New("midtrans: signature validation failed")
	}
	return nil
}

func (a *Adapter) ExecuteCommand(ctx context.Context, conn *connectors.Connection, cmd *connectors.OutboxCommand) error {
	if cmd == nil {
		return errors.New("midtrans: command is required")
	}
	if conn == nil {
		return errors.New("midtrans: connection is required")
	}
	creds, err := a.credentials(conn)
	if err != nil {
		return err
	}

	switch cmd.CommandType {
	case "payment.create_checkout":
		return a.createCheckout(ctx, conn, cmd, creds)
	case "payment.refund":
		return a.refundPayment(ctx, conn, cmd, creds)
	case "payment.lookup":
		return a.lookupPayment(ctx, conn, cmd, creds)
	default:
		return fmt.Errorf("midtrans: unsupported command type: %s", cmd.CommandType)
	}
}

func (a *Adapter) createCheckout(ctx context.Context, conn *connectors.Connection, cmd *connectors.OutboxCommand, creds Credentials) error {
	a.logger.Info("executing Midtrans checkout command", slog.Int64("company_id", conn.CompanyID))

	var reqPayload CreateCheckoutPayload
	if err := json.Unmarshal(cmd.Payload, &reqPayload); err != nil {
		return fmt.Errorf("midtrans: invalid checkout payload: %w", err)
	}
	if strings.TrimSpace(reqPayload.OrderID) == "" || reqPayload.GrossAmount <= 0 {
		return errors.New("midtrans: order_id and positive gross_amount are required")
	}
	if reqPayload.Currency != "" && !strings.EqualFold(reqPayload.Currency, "IDR") {
		return fmt.Errorf("midtrans: unsupported checkout currency %q", reqPayload.Currency)
	}

	snapResp, err := a.client(creds).CreateSnapToken(ctx, SnapTokenRequest{
		TransactionDetails: TransactionDetails{
			OrderID:  reqPayload.OrderID,
			GrossAmt: reqPayload.GrossAmount,
		},
		CustomerDetails: CustomerDetails{
			FirstName: reqPayload.CustomerName,
			Email:     reqPayload.CustomerEmail,
		},
	})
	if err != nil {
		return err
	}

	result, err := json.Marshal(CheckoutResult{Token: snapResp.Token, RedirectURL: snapResp.RedirectURL})
	if err != nil {
		return fmt.Errorf("midtrans: encode checkout result: %w", err)
	}
	// The connector worker persists the command state, while the service
	// caller uses this response payload to persist the local payment intent.
	cmd.Payload = result
	return nil
}

func (a *Adapter) refundPayment(ctx context.Context, conn *connectors.Connection, cmd *connectors.OutboxCommand, creds Credentials) error {
	var reqPayload RefundPayload
	if err := json.Unmarshal(cmd.Payload, &reqPayload); err != nil {
		return fmt.Errorf("midtrans: invalid refund payload: %w", err)
	}
	if strings.TrimSpace(reqPayload.OrderID) == "" || strings.TrimSpace(reqPayload.RefundKey) == "" {
		return errors.New("midtrans: order_id and refund_key are required")
	}
	response, err := a.client(creds).RefundTransaction(ctx, RefundRequest(reqPayload))
	if err != nil {
		return err
	}
	result, err := json.Marshal(RefundResult{
		OrderID:           response.OrderID,
		TransactionID:     response.TransactionID,
		TransactionStatus: response.TransactionStatus,
		RefundKey:         response.RefundKey,
		RefundAmount:      response.RefundAmount,
	})
	if err != nil {
		return fmt.Errorf("midtrans: encode refund result: %w", err)
	}
	cmd.Payload = result
	return nil
}

func (a *Adapter) lookupPayment(ctx context.Context, conn *connectors.Connection, cmd *connectors.OutboxCommand, creds Credentials) error {
	var reqPayload LookupPayload
	if err := json.Unmarshal(cmd.Payload, &reqPayload); err != nil {
		return fmt.Errorf("midtrans: invalid lookup payload: %w", err)
	}
	if strings.TrimSpace(reqPayload.OrderID) == "" {
		return errors.New("midtrans: order_id is required for lookup")
	}
	status, err := a.client(creds).GetTransactionStatus(ctx, reqPayload.OrderID)
	if err != nil {
		return err
	}
	result, err := json.Marshal(status)
	if err != nil {
		return fmt.Errorf("midtrans: encode lookup result: %w", err)
	}
	cmd.Payload = result
	return nil
}

func (a *Adapter) TranslateWebhook(ctx context.Context, conn *connectors.Connection, headers map[string]string, payload []byte) ([]*connectors.CanonicalEvent, error) {
	if conn == nil {
		return nil, errors.New("midtrans: connection is required")
	}
	var notif WebhookNotification
	if err := json.Unmarshal(payload, &notif); err != nil {
		return nil, fmt.Errorf("midtrans: invalid webhook payload: %w", err)
	}
	if strings.TrimSpace(notif.OrderID) == "" || strings.TrimSpace(notif.TransactionStatus) == "" {
		return nil, errors.New("midtrans: webhook order_id and transaction_status are required")
	}

	eventType := eventTypeForTransactionStatus(notif.TransactionStatus)

	eventTime := time.Now().UTC()
	if parsed, err := parseTransactionTime(notif.TransactionTime); err == nil {
		eventTime = parsed
	}
	causationID := connectors.Header(headers, "X-Midtrans-Event-Id", "X-Provider-Event-Id")
	if causationID == "" {
		causationID = notif.TransactionID
	}
	if causationID == "" {
		causationID = connectors.ProviderPayloadID(payload)
	}

	return []*connectors.CanonicalEvent{{
		CompanyID:     conn.CompanyID,
		ConnectionID:  conn.ID,
		EventType:     eventType,
		EventTime:     eventTime,
		CorrelationID: notif.OrderID,
		CausationID:   causationID,
		Payload:       payload,
	}}, nil
}

func eventTypeForTransactionStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "capture":
		return "payment.captured"
	case "settlement":
		return "payment.settled"
	case "authorize":
		return "payment.authorized"
	case "pending":
		return "payment.pending"
	case "expire":
		return "payment.expired"
	case "cancel":
		return "payment.cancelled"
	case "deny", "failure":
		return "payment.failed"
	case "partial_refund":
		return "payment.partially_refunded"
	case "refund":
		return "payment.refunded"
	default:
		return "payment.unknown"
	}
}

type CreateCheckoutPayload struct {
	OrderID       string `json:"order_id"`
	GrossAmount   int64  `json:"gross_amount"`
	Currency      string `json:"currency,omitempty"`
	CustomerName  string `json:"customer_name"`
	CustomerEmail string `json:"customer_email"`
}

type CheckoutResult struct {
	Token       string `json:"token"`
	RedirectURL string `json:"redirect_url"`
}

type RefundPayload struct {
	OrderID   string `json:"order_id"`
	RefundKey string `json:"refund_key"`
	Amount    int64  `json:"amount,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

type RefundResult struct {
	OrderID           string `json:"order_id"`
	TransactionID     string `json:"transaction_id,omitempty"`
	TransactionStatus string `json:"transaction_status"`
	RefundKey         string `json:"refund_key,omitempty"`
	RefundAmount      string `json:"refund_amount,omitempty"`
}

type LookupPayload struct {
	OrderID string `json:"order_id"`
}

// WebhookNotification represents the JSON payload sent by Midtrans webhooks.
type WebhookNotification struct {
	TransactionID     string `json:"transaction_id"`
	OrderID           string `json:"order_id"`
	GrossAmount       string `json:"gross_amount"`
	PaymentType       string `json:"payment_type"`
	TransactionTime   string `json:"transaction_time"`
	TransactionStatus string `json:"transaction_status"`
	FraudStatus       string `json:"fraud_status"`
	StatusCode        string `json:"status_code"`
	SignatureKey      string `json:"signature_key"`
	RefundKey         string `json:"refund_key"`
	RefundAmount      string `json:"refund_amount"`
	SettlementTime    string `json:"settlement_time"`
}

// LookupPaymentStatus resolves an ambiguous checkout or refund request by
// querying Midtrans before any retry can create another financial operation.
func (a *Adapter) LookupPaymentStatus(ctx context.Context, conn *connectors.Connection, orderID string) (connectors.PaymentStatusSnapshot, error) {
	if conn == nil {
		return connectors.PaymentStatusSnapshot{}, errors.New("midtrans: connection is required")
	}
	creds, err := a.credentials(conn)
	if err != nil {
		return connectors.PaymentStatusSnapshot{}, err
	}
	status, err := a.client(creds).GetTransactionStatus(ctx, orderID)
	if err != nil {
		return connectors.PaymentStatusSnapshot{}, err
	}
	eventTime := time.Now().UTC()
	if parsed, parseErr := parseTransactionTime(status.TransactionTime); parseErr == nil {
		eventTime = parsed
	}
	return connectors.PaymentStatusSnapshot{
		ProviderReference: status.OrderID,
		TransactionID:     status.TransactionID,
		Status:            status.TransactionStatus,
		EventType:         eventTypeForTransactionStatus(status.TransactionStatus),
		OccurredAt:        eventTime,
		RefundKey:         status.RefundKey,
		RefundAmount:      status.RefundAmount,
	}, nil
}

// VerifySignature validates the webhook authenticity according to Midtrans
// documentation: SHA-512(order_id + status_code + gross_amount + ServerKey).
func (n *WebhookNotification) VerifySignature(serverKey string) bool {
	if n == nil || strings.TrimSpace(serverKey) == "" || n.OrderID == "" || n.StatusCode == "" || n.GrossAmount == "" || n.SignatureKey == "" {
		return false
	}
	payload := n.OrderID + n.StatusCode + n.GrossAmount + serverKey
	hash := sha512.Sum512([]byte(payload))
	expectedSig := hex.EncodeToString(hash[:])
	providedSig := strings.ToLower(strings.TrimSpace(n.SignatureKey))
	return subtle.ConstantTimeCompare([]byte(providedSig), []byte(expectedSig)) == 1
}

func parseTransactionTime(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, errors.New("transaction time is empty")
	}
	if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
		return parsed.UTC(), nil
	}
	wib := time.FixedZone("WIB", 7*60*60)
	parsed, err := time.ParseInLocation("2006-01-02 15:04:05", raw, wib)
	if err != nil {
		return time.Time{}, err
	}
	return parsed.UTC(), nil
}

func responseMessage(body []byte) string {
	message := strings.TrimSpace(string(body))
	if message == "" {
		return "provider request failed"
	}
	if len(message) > 1024 {
		return message[:1024]
	}
	return message
}

// Compile-time assertion that *Adapter satisfies ProviderAdapter.
var _ connectors.ProviderAdapter = (*Adapter)(nil)
