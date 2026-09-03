package whatsapp_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/odyssey-erp/odyssey-erp/internal/connectors"
	"github.com/odyssey-erp/odyssey-erp/internal/connectors/providers/whatsapp"
)

func TestExecuteCommand_SendWhatsAppInExplicitDevelopmentMode(t *testing.T) {
	adapter := whatsapp.NewAdapter(slog.Default(), nil, connectors.ProviderOptions{DevelopmentMode: true})
	err := adapter.ExecuteCommand(context.Background(), &connectors.Connection{CompanyID: 1}, &connectors.OutboxCommand{
		CommandType: "messaging.send",
		Payload:     []byte(`{"to":"1234567890","content":"Hello World"}`),
	})
	if err != nil {
		t.Fatalf("explicit development fake failed: %v", err)
	}
}

func TestWhatsAppSignatureAndSendContract(t *testing.T) {
	secret := "app-secret"
	callbackPayload := []byte(`{"object":"whatsapp_business_account"}`)
	h := hmac.New(sha256.New, []byte(secret))
	_, _ = h.Write(callbackPayload)
	credentials, _ := json.Marshal(whatsapp.WhatsAppCredentials{AccessToken: "access-token", PhoneNumberID: "phone-1", AppSecret: secret})
	adapter := whatsapp.NewAdapter(slog.Default(), nil, connectors.ProviderOptions{DevelopmentMode: true})
	conn := &connectors.Connection{SecretRef: string(credentials)}
	validHeader := "sha256=" + hex.EncodeToString(h.Sum(nil))
	if err := adapter.VerifyCallbackSignature(context.Background(), conn, map[string]string{"X-Hub-Signature-256": validHeader}, callbackPayload); err != nil {
		t.Fatalf("valid signature rejected: %v", err)
	}
	if err := adapter.VerifyCallbackSignature(context.Background(), conn, map[string]string{"X-Hub-Signature-256": "sha256=bad"}, callbackPayload); err == nil {
		t.Fatal("invalid signature accepted")
	}

	var gotMethod, gotPath, gotIdempotency string
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		gotMethod, gotPath = req.Method, req.URL.Path
		gotIdempotency = req.Header.Get("Idempotency-Key")
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"messages":[{"id":"wamid.1"}]}`)), Request: req}, nil
	})
	adapter = whatsapp.NewAdapter(slog.Default(), nil, connectors.ProviderOptions{
		HTTPClient:                &http.Client{Transport: transport},
		AllowPlaintextCredentials: true,
		BaseURL:                   "https://graph.sandbox.invalid/v19.0",
	})
	err := adapter.ExecuteCommand(context.Background(), conn, &connectors.OutboxCommand{
		ID:            5,
		CommandType:   "messaging.send",
		CorrelationID: "corr-5",
		Payload:       []byte(`{"to":"1234567890","content":"Hello World"}`),
	})
	if err != nil {
		t.Fatalf("WhatsApp contract failed: %v", err)
	}
	if gotMethod != http.MethodPost || gotPath != "/v19.0/phone-1/messages" || gotIdempotency != "odyssey-command-5" {
		t.Fatalf("request = %s %s idempotency=%q", gotMethod, gotPath, gotIdempotency)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }
