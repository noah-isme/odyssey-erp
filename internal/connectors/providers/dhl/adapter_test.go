package dhl_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/odyssey-erp/odyssey-erp/internal/connectors"
	"github.com/odyssey-erp/odyssey-erp/internal/connectors/providers/dhl"
)

func TestAdapterFailsClosedAndSupportsExplicitDevelopmentFake(t *testing.T) {
	adapter := dhl.NewAdapter(nilLogger(), connectors.ProviderOptions{DevelopmentMode: true})
	ctx := context.Background()
	conn := &connectors.Connection{ID: 4, CompanyID: 9, Name: "DHL"}

	if err := adapter.ValidateConnection(ctx, conn); err == nil {
		t.Fatal("ValidateConnection() accepted missing credentials")
	}
	if err := adapter.ExecuteCommand(ctx, conn, &connectors.OutboxCommand{CommandType: "shipment.book", Payload: []byte(`{}`)}); err != nil {
		t.Fatalf("explicit development fake should succeed: %v", err)
	}
	if err := adapter.RefreshToken(ctx, conn); err == nil {
		t.Fatal("RefreshToken() should report unsupported capability")
	}
}

func TestAdapterSignatureAndShipmentContract(t *testing.T) {
	ctx := context.Background()
	payload := []byte(`{"origin":"Jakarta","destination":"Bandung","weight":2.5}`)
	secret := "webhook-secret"
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(payload)
	signature := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	devCredentials, _ := json.Marshal(dhl.Credentials{APIKey: "key", APISecret: "secret", BaseURL: "http://sandbox.invalid", WebhookSecret: secret})
	devAdapter := dhl.NewAdapter(nilLogger(), connectors.ProviderOptions{DevelopmentMode: true})
	devConn := &connectors.Connection{SecretRef: string(devCredentials)}
	if err := devAdapter.VerifyCallbackSignature(ctx, devConn, map[string]string{"X-DHL-Signature": signature}, payload); err != nil {
		t.Fatalf("valid signature rejected: %v", err)
	}
	if err := devAdapter.VerifyCallbackSignature(ctx, devConn, map[string]string{"X-DHL-Signature": "sha256=bad"}, payload); err == nil {
		t.Fatal("invalid signature accepted")
	}

	var gotMethod, gotPath, gotBody string
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		gotMethod, gotPath = req.Method, req.URL.Path
		body, _ := io.ReadAll(req.Body)
		gotBody = string(body)
		return &http.Response{StatusCode: http.StatusCreated, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"shipmentTrackingNumber":"DHL-1"}`)), Request: req}, nil
	})
	credentials, _ := json.Marshal(dhl.Credentials{APIKey: "key", APISecret: "secret", AccountNumber: "acct", BaseURL: "https://sandbox.invalid", WebhookSecret: secret})
	adapter := dhl.NewAdapter(nilLogger(), connectors.ProviderOptions{
		HTTPClient:                &http.Client{Transport: transport},
		AllowPlaintextCredentials: true,
	})
	err := adapter.ExecuteCommand(ctx, &connectors.Connection{SecretRef: string(credentials)}, &connectors.OutboxCommand{
		ID:            7,
		CommandType:   "shipment.book",
		CorrelationID: "corr-7",
		Payload:       payload,
	})
	if err != nil {
		t.Fatalf("shipment contract failed: %v", err)
	}
	if gotMethod != http.MethodPost || gotPath != "/shipments" {
		t.Fatalf("request = %s %s", gotMethod, gotPath)
	}
	if !strings.Contains(gotBody, "Jakarta") || gotBody == "" {
		t.Fatalf("unexpected request body: %s", gotBody)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }
