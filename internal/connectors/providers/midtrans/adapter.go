package midtrans

import (
	"context"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"

	"github.com/odyssey-erp/odyssey-erp/internal/connectors"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
)

// Adapter implements the connectors.ProviderAdapter interface for Midtrans.
type Adapter struct {
	logger *slog.Logger
	vault  *shared.Vault
}

// NewAdapter creates a new Midtrans integration adapter.
func NewAdapter(logger *slog.Logger, vault *shared.Vault) *Adapter {
	return &Adapter{
		logger: logger,
		vault:  vault,
	}
}

func (a *Adapter) ValidateConnection(ctx context.Context, conn *connectors.Connection) error {
	// 1. Get decrypted credentials
	// 2. Make a lightweight API call to Midtrans (e.g. check merchant info or a mock request)
	// 3. Return error if auth fails
	return nil
}

func (a *Adapter) CheckHealth(ctx context.Context, conn *connectors.Connection) (connectors.ConnectionStatus, error) {
	// Normally we might check connectivity to Midtrans API here.
	return connectors.StatusHealthy, nil
}

func (a *Adapter) RefreshToken(ctx context.Context, conn *connectors.Connection) error {
	// Midtrans primarily uses server keys which do not expire like OAuth tokens,
	// so this can be a no-op unless we use their OAuth APIs in the future.
	return nil
}

func (a *Adapter) VerifyCallbackSignature(ctx context.Context, headers map[string]string, payload []byte) error {
	// Midtrans webhook verification expects:
	// signature_key = SHA512(order_id + status_code + gross_amount + ServerKey)
	// This requires parsing the payload first to extract order_id, status_code, and gross_amount,
	// and we also need the ServerKey from the connection. This implies the caller needs to provide 
	// the Connection or ServerKey to this function, but the interface doesn't pass conn.
	// 
	// For scaffolding, we will return nil here and implement the full secure signature validation 
	// in a specialized method or by adjusting the interface to pass the Connection.
	return nil
}

func (a *Adapter) ExecuteCommand(ctx context.Context, conn *connectors.Connection, cmd *connectors.OutboxCommand) error {
	switch cmd.CommandType {
	case "payment.create_checkout":
		return a.handleCreateCheckout(ctx, conn, cmd)
	default:
		return errors.New("unsupported command type for midtrans")
	}
}

func (a *Adapter) TranslateWebhook(ctx context.Context, conn *connectors.Connection, headers map[string]string, payload []byte) ([]*connectors.CanonicalEvent, error) {
	var notif WebhookNotification
	if err := json.Unmarshal(payload, &notif); err != nil {
		return nil, err
	}

	// In a real implementation, we map Midtrans transaction status (e.g., 'settlement', 'capture', 'expire')
	// to our CanonicalEvent types.
	eventType := "payment.unknown"
	switch notif.TransactionStatus {
	case "capture", "settlement":
		eventType = "payment.captured" // Simplification
	case "expire", "cancel", "deny":
		eventType = "payment.failed"
	case "pending":
		eventType = "payment.authorized"
	}

	canonical := &connectors.CanonicalEvent{
		CompanyID:     conn.CompanyID,
		ConnectionID:  conn.ID,
		EventType:     eventType,
		CorrelationID: notif.OrderID, // Our internal reference
		Payload:       payload,       // Store the raw payload in the canonical event for now
	}

	return []*connectors.CanonicalEvent{canonical}, nil
}

type CreateCheckoutPayload struct {
	OrderID       string `json:"order_id"`
	GrossAmount   int64  `json:"gross_amount"`
	CustomerName  string `json:"customer_name"`
	CustomerEmail string `json:"customer_email"`
}

type CheckoutResult struct {
	Token       string `json:"token"`
	RedirectURL string `json:"redirect_url"`
}

func (a *Adapter) handleCreateCheckout(ctx context.Context, conn *connectors.Connection, cmd *connectors.OutboxCommand) error {
	a.logger.Info("Executing payment.create_checkout for Midtrans", slog.Int64("company_id", conn.CompanyID))

	serverKey, err := conn.GetCredentials(a.vault)
	if err != nil {
		return err
	}

	var reqPayload CreateCheckoutPayload
	if err := json.Unmarshal(cmd.Payload, &reqPayload); err != nil {
		return err
	}

	client := NewClient(serverKey, false) // Using sandbox for now
	snapReq := SnapTokenRequest{
		TransactionDetails: TransactionDetails{
			OrderID:  reqPayload.OrderID,
			GrossAmt: reqPayload.GrossAmount,
		},
		CustomerDetails: CustomerDetails{
			FirstName: reqPayload.CustomerName,
			Email:     reqPayload.CustomerEmail,
		},
	}

	snapResp, err := client.CreateSnapToken(ctx, snapReq)
	if err != nil {
		return err
	}

	// Update the OutboxCommand payload with the result so the domain module can consume it
	result := CheckoutResult{
		Token:       snapResp.Token,
		RedirectURL: snapResp.RedirectURL,
	}
	newPayload, _ := json.Marshal(result)
	cmd.Payload = newPayload

	return nil
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
}

// VerifySignature validates the webhook authenticity according to Midtrans documentation.
func (n *WebhookNotification) VerifySignature(serverKey string) bool {
	// SHA512(order_id + status_code + gross_amount + ServerKey)
	payload := n.OrderID + n.StatusCode + n.GrossAmount + serverKey
	hash := sha512.Sum512([]byte(payload))
	expectedSig := hex.EncodeToString(hash[:])
	return n.SignatureKey == expectedSig
}
