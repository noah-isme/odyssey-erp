package oidc_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/odyssey-erp/odyssey-erp/internal/connectors"
	"github.com/odyssey-erp/odyssey-erp/internal/connectors/providers/oidc"
)

func TestOIDCAdapter_ValidateAndExplicitDevelopmentCommand(t *testing.T) {
	adapter := oidc.NewAdapter(slog.Default(), connectors.ProviderOptions{DevelopmentMode: true})
	conn := &connectors.Connection{SecretRef: `{"issuer":"http://issuer.invalid","client_id":"client-1"}`}
	if err := adapter.ValidateConnection(context.Background(), conn); err != nil {
		t.Fatalf("expected valid connection, got %v", err)
	}
	if err := adapter.ValidateConnection(context.Background(), &connectors.Connection{}); err == nil {
		t.Fatal("missing secret accepted")
	}
	if err := adapter.ExecuteCommand(context.Background(), &connectors.Connection{}, &connectors.OutboxCommand{
		CommandType: "auth.user.provision",
		Payload:     []byte(`{}`),
	}); err != nil {
		t.Fatalf("explicit development fake failed: %v", err)
	}
	if err := adapter.RefreshToken(context.Background(), conn); err == nil {
		t.Fatal("RefreshToken() should report unsupported capability")
	}
}

func TestOIDCAdapter_RejectsUnverifiedLogoutPayload(t *testing.T) {
	adapter := oidc.NewAdapter(slog.Default(), connectors.ProviderOptions{DevelopmentMode: true})
	conn := &connectors.Connection{CompanyID: 1, ID: 10, SecretRef: `{"issuer":"http://issuer.invalid","client_id":"client-1"}`}
	if _, err := adapter.TranslateWebhook(context.Background(), conn, nil, []byte("some_other_data=123")); err == nil {
		t.Fatal("payload without logout_token accepted")
	}
}

func TestOIDCAdapter_DiscoveryAndSCIMContracts(t *testing.T) {
	var gotPath, gotMethod, gotIdempotency string
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		gotPath, gotMethod = req.URL.Path, req.Method
		gotIdempotency = req.Header.Get("Idempotency-Key")
		if strings.Contains(req.URL.Path, "openid-configuration") {
			body := `{"issuer":"https://oidc.sandbox.invalid","authorization_endpoint":"https://oidc.sandbox.invalid/auth","token_endpoint":"https://oidc.sandbox.invalid/token","jwks_uri":"https://oidc.sandbox.invalid/keys"}`
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
		}
		return &http.Response{StatusCode: http.StatusCreated, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"id":"scim-1"}`)), Request: req}, nil
	})
	adapter := oidc.NewAdapter(slog.Default(), connectors.ProviderOptions{
		HTTPClient:                &http.Client{Transport: transport},
		AllowPlaintextCredentials: true,
	})
	credentials, _ := json.Marshal(struct {
		Issuer       string `json:"issuer"`
		ClientID     string `json:"client_id"`
		SCIMEndpoint string `json:"scim_endpoint"`
		SCIMToken    string `json:"scim_token"`
	}{Issuer: "https://oidc.sandbox.invalid", ClientID: "client-1", SCIMEndpoint: "https://scim.sandbox.invalid", SCIMToken: "token"})
	status, err := adapter.CheckHealth(context.Background(), &connectors.Connection{SecretRef: string(credentials)})
	if err != nil || status != connectors.StatusHealthy {
		t.Fatalf("CheckHealth() = %v, %v", status, err)
	}
	payload, _ := json.Marshal(oidc.SCIMUserPayload{UserName: "user@example.com", DisplayName: "Test User"})
	err = adapter.ExecuteCommand(context.Background(), &connectors.Connection{SecretRef: string(credentials)}, &connectors.OutboxCommand{
		ID:            8,
		CommandType:   "auth.user.provision",
		CorrelationID: "corr-8",
		Payload:       payload,
	})
	if err != nil {
		t.Fatalf("SCIM contract failed: %v", err)
	}
	if gotMethod != http.MethodPost || gotPath != "/Users" || gotIdempotency != "odyssey-command-8" {
		t.Fatalf("request = %s %s idempotency=%q", gotMethod, gotPath, gotIdempotency)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }
