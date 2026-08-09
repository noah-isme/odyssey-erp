package stripe_test

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/odyssey-erp/odyssey-erp/internal/connectors"
	"github.com/odyssey-erp/odyssey-erp/internal/connectors/providers/stripe"
	"github.com/stripe/stripe-go/v79/webhook"
)

func TestStripeAdapter_ValidateConnectionAndHealthInExplicitDevelopmentMode(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	adapter := stripe.NewAdapter(logger, connectors.ProviderOptions{DevelopmentMode: true})
	validConn := &connectors.Connection{SecretRef: `{"api_key":"sk_test_123","webhook_secret":"whsec_test"}`}
	if err := adapter.ValidateConnection(context.Background(), validConn); err != nil {
		t.Fatalf("expected valid credentials, got %v", err)
	}
	status, err := adapter.CheckHealth(context.Background(), validConn)
	if err != nil || status != connectors.StatusHealthy {
		t.Fatalf("CheckHealth() = %v, %v", status, err)
	}
	if err := adapter.ValidateConnection(context.Background(), &connectors.Connection{}); err == nil {
		t.Fatal("ValidateConnection() accepted missing secret")
	}
}

func TestStripeAdapter_VerifiesSignatureAndRejectsFake(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	adapter := stripe.NewAdapter(logger, connectors.ProviderOptions{DevelopmentMode: true})
	secret := "whsec_test"
	payload := []byte(fmt.Sprintf(`{"id":"evt_1","object":"event","api_version":"2024-06-20","created":%d}`, time.Now().Unix()))
	timestamp := time.Now()
	signature := fmt.Sprintf("t=%d,v1=%s", timestamp.Unix(), hex.EncodeToString(webhook.ComputeSignature(timestamp, payload, secret)))
	conn := &connectors.Connection{SecretRef: `{"api_key":"sk_test_123","webhook_secret":"whsec_test"}`}
	if err := adapter.VerifyCallbackSignature(context.Background(), conn, map[string]string{"Stripe-Signature": signature}, payload); err != nil {
		t.Fatalf("valid signature rejected: %v", err)
	}
	if err := adapter.VerifyCallbackSignature(context.Background(), conn, map[string]string{"Stripe-Signature": "fake-sig-456"}, payload); err == nil {
		t.Fatal("fake signature accepted")
	}
}

func TestStripeAdapter_ChargeContractUsesRealCallAndIdempotency(t *testing.T) {
	var gotMethod, gotPath, gotIdempotency string
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		gotMethod, gotPath = req.Method, req.URL.Path
		gotIdempotency = req.Header.Get("Idempotency-Key")
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"id":"ch_1","object":"charge"}`)), Request: req}, nil
	})
	adapter := stripe.NewAdapter(slog.Default(), connectors.ProviderOptions{
		HTTPClient:                &http.Client{Transport: transport},
		AllowPlaintextCredentials: true,
		BaseURL:                   "https://stripe.sandbox.invalid",
	})
	secret, _ := json.Marshal(struct {
		APIKey        string `json:"api_key"`
		WebhookSecret string `json:"webhook_secret"`
	}{APIKey: "sk_test_123", WebhookSecret: "whsec_test"})
	cmd := &connectors.OutboxCommand{ID: 19, CommandType: "payment.charge", CorrelationID: "corr-19", Payload: []byte(`{"amount":1500,"currency":"USD","source":"tok_visa"}`)}
	if err := adapter.ExecuteCommand(context.Background(), &connectors.Connection{SecretRef: string(secret)}, cmd); err != nil {
		t.Fatalf("charge contract failed: %v", err)
	}
	if gotMethod != http.MethodPost || gotPath != "/v1/charges" || gotIdempotency != "odyssey-command-19" {
		t.Fatalf("request = %s %s idempotency=%q", gotMethod, gotPath, gotIdempotency)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }
