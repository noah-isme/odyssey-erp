package dhl

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/odyssey-erp/odyssey-erp/internal/connectors"
)

// Adapter implements the DHL API connector.
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
	APIKey        string `json:"api_key"`
	APISecret     string `json:"api_secret"`
	AccountNumber string `json:"account_number"`
	BaseURL       string `json:"base_url"`
	WebhookSecret string `json:"webhook_secret"`
	HealthPath    string `json:"health_path"`
}

func (a *Adapter) credentials(conn *connectors.Connection) (Credentials, error) {
	secret, err := a.options.ResolveSecret(conn)
	if err != nil {
		return Credentials{}, err
	}
	var creds Credentials
	if err := json.Unmarshal([]byte(secret), &creds); err != nil {
		return Credentials{}, fmt.Errorf("dhl: invalid credential format: %w", err)
	}
	if strings.TrimSpace(creds.APIKey) == "" || strings.TrimSpace(creds.APISecret) == "" {
		return Credentials{}, errors.New("dhl: api_key and api_secret are required")
	}
	if strings.TrimSpace(creds.BaseURL) == "" {
		return Credentials{}, errors.New("dhl: base_url is required")
	}
	parsed, err := url.Parse(creds.BaseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return Credentials{}, errors.New("dhl: base_url must be an absolute URL")
	}
	if !a.options.DevelopmentMode && parsed.Scheme != "https" {
		return Credentials{}, errors.New("dhl: production base_url must use HTTPS")
	}
	return creds, nil
}

func (a *Adapter) ValidateConnection(ctx context.Context, conn *connectors.Connection) error {
	if _, err := a.credentials(conn); err != nil {
		return fmt.Errorf("dhl: validate credentials: %w", err)
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
	path := creds.HealthPath
	if path == "" {
		path = "/"
	}
	resp, body, err := a.request(ctx, creds, http.MethodGet, path, nil, "")
	if err != nil {
		return connectors.StatusActionRequired, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return connectors.StatusActionRequired, fmt.Errorf("dhl: health check returned %d: %s", resp.StatusCode, string(body))
	}
	return connectors.StatusHealthy, nil
}

func (a *Adapter) RefreshToken(ctx context.Context, conn *connectors.Connection) error {
	return connectors.ErrUnsupportedOperation
}

// VerifyCallbackSignature verifies the configured DHL HMAC callback secret.
// DHL products differ in webhook headers, so both the provider-specific and
// generic signature headers are accepted.
func (a *Adapter) VerifyCallbackSignature(ctx context.Context, conn *connectors.Connection, headers map[string]string, payload []byte) error {
	creds, err := a.credentials(conn)
	if err != nil {
		return err
	}
	if strings.TrimSpace(creds.WebhookSecret) == "" {
		return errors.New("dhl: webhook_secret is required for callbacks")
	}
	signature := connectors.Header(headers, "X-DHL-Signature", "X-Provider-Signature")
	if signature == "" {
		return errors.New("dhl: missing callback signature")
	}
	if strings.HasPrefix(signature, "sha256=") {
		provided, decodeErr := hex.DecodeString(strings.TrimPrefix(signature, "sha256="))
		if decodeErr != nil || len(provided) != sha256.Size {
			return errors.New("dhl: invalid callback signature encoding")
		}
		h := hmac.New(sha256.New, []byte(creds.WebhookSecret))
		_, _ = h.Write(payload)
		if !hmac.Equal(provided, h.Sum(nil)) {
			return errors.New("dhl: callback signature mismatch")
		}
		return nil
	}
	decoded, decodeErr := base64.StdEncoding.DecodeString(signature)
	if decodeErr != nil {
		return errors.New("dhl: invalid callback signature format")
	}
	h := hmac.New(sha256.New, []byte(creds.WebhookSecret))
	_, _ = h.Write(payload)
	if !hmac.Equal(decoded, h.Sum(nil)) {
		return errors.New("dhl: callback signature mismatch")
	}
	return nil
}

type ShipmentPayload struct {
	Origin        string  `json:"origin"`
	Destination   string  `json:"destination"`
	Weight        float64 `json:"weight"`
	ServiceCode   string  `json:"service_code,omitempty"`
	AccountNumber string  `json:"account_number,omitempty"`
}

func (a *Adapter) ExecuteCommand(ctx context.Context, conn *connectors.Connection, cmd *connectors.OutboxCommand) error {
	if cmd == nil {
		return errors.New("dhl: command is required")
	}
	if cmd.CommandType != "shipment.book" {
		return fmt.Errorf("dhl: unsupported command type: %s", cmd.CommandType)
	}
	if a.options.DevelopmentMode && strings.TrimSpace(conn.SecretRef) == "" {
		a.logger.Info("DHL shipment simulated in explicit development mode", slog.String("correlation_id", cmd.CorrelationID))
		return nil
	}
	creds, err := a.credentials(conn)
	if err != nil {
		return err
	}
	var payload ShipmentPayload
	if err := json.Unmarshal(cmd.Payload, &payload); err != nil {
		return fmt.Errorf("dhl: failed to parse shipment payload: %w", err)
	}
	if strings.TrimSpace(payload.Origin) == "" || strings.TrimSpace(payload.Destination) == "" || payload.Weight <= 0 {
		return errors.New("dhl: origin, destination, and positive weight are required")
	}
	if payload.AccountNumber == "" {
		payload.AccountNumber = creds.AccountNumber
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("dhl: marshal shipment payload: %w", err)
	}
	resp, responseBody, err := a.request(ctx, creds, http.MethodPost, "/shipments", body, cmd.CorrelationID)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("dhl: shipment booking returned %d: %s", resp.StatusCode, string(responseBody))
	}
	var result struct {
		ShipmentTrackingNumber string `json:"shipmentTrackingNumber"`
		ID                     string `json:"id"`
	}
	if err := json.Unmarshal(responseBody, &result); err != nil {
		return fmt.Errorf("dhl: invalid booking response: %w", err)
	}
	if strings.TrimSpace(result.ShipmentTrackingNumber) == "" && strings.TrimSpace(result.ID) == "" {
		return errors.New("dhl: booking response did not contain a shipment identifier")
	}
	return nil
}

func (a *Adapter) TranslateWebhook(ctx context.Context, conn *connectors.Connection, headers map[string]string, payload []byte) ([]*connectors.CanonicalEvent, error) {
	var event struct {
		Status    string `json:"status"`
		Timestamp string `json:"timestamp"`
	}
	if err := json.Unmarshal(payload, &event); err != nil {
		return nil, fmt.Errorf("dhl: invalid webhook payload: %w", err)
	}
	providerEventID := connectors.Header(headers, "X-DHL-Event-Id", "X-Provider-Event-Id", "X-Message-Reference")
	if providerEventID == "" {
		providerEventID = connectors.ProviderPayloadID(payload)
	}
	eventTime := time.Now().UTC()
	if event.Timestamp != "" {
		if parsed, err := time.Parse(time.RFC3339, event.Timestamp); err == nil {
			eventTime = parsed.UTC()
		}
	}
	return []*connectors.CanonicalEvent{{
		CompanyID:     conn.CompanyID,
		ConnectionID:  conn.ID,
		EventType:     "shipment.status_updated",
		EventTime:     eventTime,
		CorrelationID: providerEventID,
		CausationID:   providerEventID,
		Payload:       payload,
	}}, nil
}

func (a *Adapter) request(ctx context.Context, creds Credentials, method, path string, body []byte, idempotencyKey string) (*http.Response, []byte, error) {
	baseURL := strings.TrimRight(creds.BaseURL, "/")
	requestURL := baseURL + "/" + strings.TrimLeft(path, "/")
	headers := http.Header{
		"Accept":        []string{"application/json"},
		"Content-Type":  []string{"application/json"},
		"Authorization": []string{"Basic " + base64.StdEncoding.EncodeToString([]byte(creds.APIKey+":"+creds.APISecret))},
	}
	if strings.TrimSpace(idempotencyKey) != "" {
		headers.Set("Idempotency-Key", idempotencyKey)
	}
	resp, responseBody, err := connectors.DoWithRetry(ctx, a.options.HTTPClientOrDefault(), method, requestURL, body, headers, a.options.RetryPolicyOrDefault())
	if err != nil {
		return nil, nil, fmt.Errorf("dhl: provider request failed: %w", err)
	}
	return resp, responseBody, nil
}
