package stripe

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/odyssey-erp/odyssey-erp/internal/connectors"
	stripego "github.com/stripe/stripe-go/v79"
	"github.com/stripe/stripe-go/v79/balance"
	"github.com/stripe/stripe-go/v79/charge"
	"github.com/stripe/stripe-go/v79/webhook"
)

// Adapter implements connectors.ProviderAdapter for Stripe.
type Adapter struct {
	logger  *slog.Logger
	options connectors.ProviderOptions
}

// NewAdapter creates a Stripe adapter. DevelopmentMode must be explicit for
// tests to bypass real Stripe calls.
func NewAdapter(logger *slog.Logger, options ...connectors.ProviderOptions) *Adapter {
	var opts connectors.ProviderOptions
	if len(options) > 0 {
		opts = options[0]
	}
	return &Adapter{logger: logger, options: opts}
}

type Credentials struct {
	APIKey        string `json:"api_key"`
	WebhookSecret string `json:"webhook_secret"`
}

func (a *Adapter) credentials(conn *connectors.Connection) (Credentials, error) {
	secret, err := a.options.ResolveSecret(conn)
	if err != nil {
		return Credentials{}, err
	}
	var creds Credentials
	if err := json.Unmarshal([]byte(secret), &creds); err != nil {
		// A raw Stripe API key is accepted only in explicitly configured
		// development mode. Production secrets must be structured and vaulted.
		if a.options.DevelopmentMode && strings.HasPrefix(secret, "sk_") {
			creds.APIKey = secret
		} else {
			return Credentials{}, fmt.Errorf("stripe: invalid credential format: %w", err)
		}
	}
	if strings.TrimSpace(creds.APIKey) == "" {
		return Credentials{}, fmt.Errorf("stripe: api_key is required")
	}
	return creds, nil
}

// ValidateConnection checks that the encrypted secret resolves to a usable
// Stripe API key. A health check performs the remote validation separately.
func (a *Adapter) ValidateConnection(ctx context.Context, conn *connectors.Connection) error {
	if _, err := a.credentials(conn); err != nil {
		return fmt.Errorf("stripe: validate credentials: %w", err)
	}
	return nil
}

// CheckHealth queries Stripe's balance endpoint with the resolved API key.
func (a *Adapter) CheckHealth(ctx context.Context, conn *connectors.Connection) (connectors.ConnectionStatus, error) {
	creds, err := a.credentials(conn)
	if err != nil {
		return connectors.StatusActionRequired, err
	}
	if a.options.DevelopmentMode {
		a.logger.Info("stripe health check skipped in explicit development mode", slog.String("name", conn.Name))
		return connectors.StatusHealthy, nil
	}

	backends := stripego.NewBackends(a.options.HTTPClientOrDefault())
	configureBackend(backends.API, a.options.BaseURL)
	backends.API.SetMaxNetworkRetries(2)
	if _, err := (balance.Client{B: backends.API, Key: creds.APIKey}).Get(nil); err != nil {
		return connectors.StatusActionRequired, fmt.Errorf("stripe: health check failed: %w", err)
	}
	return connectors.StatusHealthy, nil
}

// RefreshToken is not applicable to Stripe API-key credentials.
func (a *Adapter) RefreshToken(ctx context.Context, conn *connectors.Connection) error {
	return connectors.ErrUnsupportedOperation
}

// VerifyCallbackSignature verifies Stripe's signed webhook envelope. No
// signature bypass exists, including in development mode.
func (a *Adapter) VerifyCallbackSignature(ctx context.Context, conn *connectors.Connection, headers map[string]string, payload []byte) error {
	creds, err := a.credentials(conn)
	if err != nil {
		return err
	}
	if strings.TrimSpace(creds.WebhookSecret) == "" {
		return fmt.Errorf("stripe: webhook_secret is required for callbacks")
	}
	signature := connectors.Header(headers, "Stripe-Signature", "X-Provider-Signature")
	if signature == "" {
		return fmt.Errorf("stripe: missing signature header")
	}
	if _, err := webhook.ConstructEvent(payload, signature, creds.WebhookSecret); err != nil {
		return fmt.Errorf("stripe: signature validation failed: %w", err)
	}
	return nil
}

// PaymentChargePayload defines the incoming JSON structure from domain modules.
type PaymentChargePayload struct {
	Amount      int64  `json:"amount"`
	Currency    string `json:"currency"`
	Source      string `json:"source"`
	Description string `json:"description"`
}

// ExecuteCommand dispatches an outbound command to Stripe. The Stripe SDK
// receives a stable idempotency key derived from the durable outbox command.
func (a *Adapter) ExecuteCommand(ctx context.Context, conn *connectors.Connection, cmd *connectors.OutboxCommand) error {
	creds, err := a.credentials(conn)
	if err != nil {
		return err
	}
	if cmd == nil {
		return fmt.Errorf("stripe: command is required")
	}

	switch cmd.CommandType {
	case "payment.charge":
		var payload PaymentChargePayload
		if err := json.Unmarshal(cmd.Payload, &payload); err != nil {
			return fmt.Errorf("stripe: failed to parse charge payload: %w", err)
		}
		if payload.Amount <= 0 || strings.TrimSpace(payload.Currency) == "" || strings.TrimSpace(payload.Source) == "" {
			return fmt.Errorf("stripe: amount, currency, and source are required")
		}
		if a.options.DevelopmentMode {
			a.logger.Info("stripe charge simulated in explicit development mode", slog.String("correlation_id", cmd.CorrelationID))
			return nil
		}

		params := &stripego.ChargeParams{
			Amount:      stripego.Int64(payload.Amount),
			Currency:    stripego.String(strings.ToLower(payload.Currency)),
			Source:      &stripego.PaymentSourceSourceParams{Token: stripego.String(payload.Source)},
			Description: stripego.String(payload.Description),
		}
		params.IdempotencyKey = stripego.String(stripeIdempotencyKey(cmd))
		backends := stripego.NewBackends(a.options.HTTPClientOrDefault())
		configureBackend(backends.API, a.options.BaseURL)
		backends.API.SetMaxNetworkRetries(2)
		if _, err := (charge.Client{B: backends.API, Key: creds.APIKey}).New(params); err != nil {
			return fmt.Errorf("stripe: charge failed: %w", err)
		}
		return nil
	default:
		return fmt.Errorf("stripe: unsupported command type: %s", cmd.CommandType)
	}
}

// TranslateWebhook parses the Stripe webhook and emits canonical domain events.
func (a *Adapter) TranslateWebhook(ctx context.Context, conn *connectors.Connection, headers map[string]string, payload []byte) ([]*connectors.CanonicalEvent, error) {
	var event stripego.Event
	if err := json.Unmarshal(payload, &event); err != nil {
		return nil, fmt.Errorf("stripe: failed to unmarshal webhook event: %w", err)
	}
	if event.ID == "" || event.Created == 0 {
		return nil, fmt.Errorf("stripe: webhook event id and created timestamp are required")
	}

	var eventType string
	var correlationID string
	switch event.Type {
	case "charge.succeeded":
		var chg stripego.Charge
		if err := json.Unmarshal(event.Data.Raw, &chg); err != nil {
			return nil, fmt.Errorf("stripe: failed to parse charge data: %w", err)
		}
		eventType, correlationID = "payment.captured", chg.ID
	case "charge.failed":
		var chg stripego.Charge
		if err := json.Unmarshal(event.Data.Raw, &chg); err != nil {
			return nil, fmt.Errorf("stripe: failed to parse charge data: %w", err)
		}
		eventType, correlationID = "payment.failed", chg.ID
	default:
		return nil, nil
	}
	if correlationID == "" {
		return nil, fmt.Errorf("stripe: webhook charge id is required")
	}

	return []*connectors.CanonicalEvent{{
		CompanyID:     conn.CompanyID,
		ConnectionID:  conn.ID,
		EventType:     eventType,
		EventTime:     time.Unix(event.Created, 0).UTC(),
		CorrelationID: correlationID,
		CausationID:   event.ID,
		Payload:       event.Data.Raw,
	}}, nil
}

func stripeIdempotencyKey(cmd *connectors.OutboxCommand) string {
	if cmd.ID > 0 {
		return fmt.Sprintf("odyssey-command-%d", cmd.ID)
	}
	return "odyssey-correlation-" + cmd.CorrelationID
}

func configureBackend(backend stripego.Backend, baseURL string) {
	if strings.TrimSpace(baseURL) == "" {
		return
	}
	if implementation, ok := backend.(*stripego.BackendImplementation); ok {
		implementation.URL = strings.TrimRight(baseURL, "/")
	}
}
