package whatsapp

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/odyssey-erp/odyssey-erp/internal/connectors"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
)

// Adapter implements the connectors.ProviderAdapter interface for WhatsApp.
type Adapter struct {
	logger  *slog.Logger
	vault   *shared.Vault
	options connectors.ProviderOptions
}

func NewAdapter(logger *slog.Logger, vault *shared.Vault, options ...connectors.ProviderOptions) *Adapter {
	var opts connectors.ProviderOptions
	if len(options) > 0 {
		opts = options[0]
	}
	if opts.Vault == nil {
		opts.Vault = vault
	}
	return &Adapter{logger: logger, vault: vault, options: opts}
}

type WhatsAppCredentials struct {
	AccessToken   string `json:"access_token"`
	PhoneNumberID string `json:"phone_number_id"`
	AppSecret     string `json:"app_secret"`
}

func (a *Adapter) credentials(conn *connectors.Connection) (WhatsAppCredentials, error) {
	options := a.options
	if options.Vault == nil {
		options.Vault = a.vault
	}
	secret, err := options.ResolveSecret(conn)
	if err != nil {
		return WhatsAppCredentials{}, err
	}
	var creds WhatsAppCredentials
	if err := json.Unmarshal([]byte(secret), &creds); err != nil {
		return WhatsAppCredentials{}, fmt.Errorf("whatsapp: invalid credential format: %w", err)
	}
	if strings.TrimSpace(creds.AccessToken) == "" || strings.TrimSpace(creds.PhoneNumberID) == "" {
		return WhatsAppCredentials{}, errors.New("whatsapp: access_token and phone_number_id are required")
	}
	return creds, nil
}

func (a *Adapter) ValidateConnection(ctx context.Context, conn *connectors.Connection) error {
	if _, err := a.credentials(conn); err != nil {
		return fmt.Errorf("whatsapp: validate credentials: %w", err)
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
	client := NewClientWithOptions(creds.AccessToken, creds.PhoneNumberID, a.options)
	if err := client.CheckPhoneNumber(ctx); err != nil {
		return connectors.StatusActionRequired, err
	}
	return connectors.StatusHealthy, nil
}

func (a *Adapter) RefreshToken(ctx context.Context, conn *connectors.Connection) error {
	return connectors.ErrUnsupportedOperation
}

// VerifyCallbackSignature verifies WhatsApp Cloud API's X-Hub-Signature-256
// HMAC envelope using the app secret stored with the connection.
func (a *Adapter) VerifyCallbackSignature(ctx context.Context, conn *connectors.Connection, headers map[string]string, payload []byte) error {
	creds, err := a.credentials(conn)
	if err != nil {
		return err
	}
	if strings.TrimSpace(creds.AppSecret) == "" {
		return errors.New("whatsapp: app_secret is required for callbacks")
	}
	signature := connectors.Header(headers, "X-Hub-Signature-256", "X-Provider-Signature")
	if signature == "" {
		return errors.New("whatsapp: missing callback signature")
	}
	prefix, encoded, ok := strings.Cut(signature, "=")
	if !ok || prefix != "sha256" || strings.TrimSpace(encoded) == "" {
		return errors.New("whatsapp: invalid callback signature format")
	}
	provided, err := hex.DecodeString(encoded)
	if err != nil || len(provided) != sha256.Size {
		return errors.New("whatsapp: invalid callback signature encoding")
	}
	h := hmac.New(sha256.New, []byte(creds.AppSecret))
	_, _ = h.Write(payload)
	if !hmac.Equal(provided, h.Sum(nil)) {
		return errors.New("whatsapp: callback signature mismatch")
	}
	return nil
}

func (a *Adapter) ExecuteCommand(ctx context.Context, conn *connectors.Connection, cmd *connectors.OutboxCommand) error {
	if cmd == nil {
		return errors.New("whatsapp: command is required")
	}
	if cmd.CommandType != "messaging.send" {
		return errors.New("unsupported command type for whatsapp")
	}
	if a.options.DevelopmentMode && strings.TrimSpace(conn.SecretRef) == "" {
		a.logger.Info("WhatsApp message simulated in explicit development mode", slog.Int64("company_id", conn.CompanyID))
		return nil
	}
	return a.handleSendWhatsApp(ctx, conn, cmd)
}

// WebhookPayload represents a WhatsApp Cloud API webhook structure.
type WebhookPayload struct {
	Object string `json:"object"`
	Entry  []struct {
		ID      string `json:"id"`
		Changes []struct {
			Value struct {
				MessagingProduct string `json:"messaging_product"`
				Metadata         struct {
					DisplayPhoneNumber string `json:"display_phone_number"`
					PhoneNumberID      string `json:"phone_number_id"`
				} `json:"metadata"`
				Statuses []struct {
					ID        string `json:"id"`
					Status    string `json:"status"`
					Timestamp string `json:"timestamp"`
				} `json:"statuses"`
			} `json:"value"`
			Field string `json:"field"`
		} `json:"changes"`
	} `json:"entry"`
}

func (a *Adapter) TranslateWebhook(ctx context.Context, conn *connectors.Connection, headers map[string]string, payload []byte) ([]*connectors.CanonicalEvent, error) {
	var notif WebhookPayload
	if err := json.Unmarshal(payload, &notif); err != nil {
		return nil, fmt.Errorf("whatsapp: invalid webhook payload: %w", err)
	}
	if notif.Object != "whatsapp_business_account" && notif.Object != "" {
		return nil, errors.New("whatsapp: unexpected webhook object")
	}

	var events []*connectors.CanonicalEvent
	for _, entry := range notif.Entry {
		for _, change := range entry.Changes {
			if change.Field != "messages" {
				continue
			}
			for _, status := range change.Value.Statuses {
				eventType := "messaging.unknown"
				switch status.Status {
				case "sent":
					eventType = "messaging.sent"
				case "delivered":
					eventType = "messaging.delivered"
				case "read":
					eventType = "messaging.read"
				case "failed":
					eventType = "messaging.failed"
				}
				if strings.TrimSpace(status.ID) == "" {
					return nil, errors.New("whatsapp: webhook message id is required")
				}
				events = append(events, &connectors.CanonicalEvent{
					CompanyID:     conn.CompanyID,
					ConnectionID:  conn.ID,
					EventType:     eventType,
					CorrelationID: status.ID,
					Payload:       payload,
				})
			}
		}
	}
	return events, nil
}

type SendWhatsAppPayload struct {
	To      string `json:"to"`
	Content string `json:"content"`
}

func (a *Adapter) handleSendWhatsApp(ctx context.Context, conn *connectors.Connection, cmd *connectors.OutboxCommand) error {
	creds, err := a.credentials(conn)
	if err != nil {
		return err
	}
	var reqPayload SendWhatsAppPayload
	if err := json.Unmarshal(cmd.Payload, &reqPayload); err != nil {
		return fmt.Errorf("whatsapp: failed to parse send payload: %w", err)
	}
	client := NewClientWithOptions(creds.AccessToken, creds.PhoneNumberID, a.options)
	key := cmd.CorrelationID
	if cmd.ID > 0 {
		key = fmt.Sprintf("odyssey-command-%d", cmd.ID)
	}
	if _, err := client.SendTextMessageWithIdempotency(ctx, reqPayload.To, reqPayload.Content, key); err != nil {
		return err
	}
	return nil
}
