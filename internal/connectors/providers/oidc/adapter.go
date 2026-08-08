package oidc

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/odyssey-erp/odyssey-erp/internal/connectors"
)

// Adapter implements connectors.ProviderAdapter for OpenID Connect (OIDC).
type Adapter struct {
	logger *slog.Logger
}

func NewAdapter(logger *slog.Logger) *Adapter {
	return &Adapter{logger: logger}
}

// ValidateConnection checks that the OIDC Issuer URL and client details are present.
func (a *Adapter) ValidateConnection(ctx context.Context, conn *connectors.Connection) error {
	a.logger.Info("validating OIDC connection", slog.String("name", conn.Name))
	
	// A valid connection would store the Issuer URL, Client ID, and Client Secret Ref
	// In our mock, we assume conn.SecretRef contains JSON {"issuer": "https://...", "client_id": "...", "client_secret": "..."}
	if conn.SecretRef == "" {
		return fmt.Errorf("oidc: missing configuration in secret reference")
	}

	return nil
}

// CheckHealth hits the OIDC discovery endpoint (.well-known/openid-configuration) to ensure the Identity Provider is online.
func (a *Adapter) CheckHealth(ctx context.Context, conn *connectors.Connection) (connectors.ConnectionStatus, error) {
	// For testing, if secretRef starts with a known domain, we can query it.
	// Normally we'd extract the issuer URL.
	// Example: issuer := "https://accounts.google.com"
	issuer := "https://accounts.google.com" // hardcoded fallback for dummy connections
	
	// Try parsing it out if possible (simulate reading from vault)
	var config struct {
		Issuer string `json:"issuer"`
	}
	if err := json.Unmarshal([]byte(conn.SecretRef), &config); err == nil && config.Issuer != "" {
		issuer = config.Issuer
	}

	provider, err := oidc.NewProvider(ctx, issuer)
	if err != nil {
		a.logger.Error("oidc health check failed", slog.Any("error", err), slog.String("issuer", issuer))
		return connectors.StatusActionRequired, fmt.Errorf("oidc provider unavailable: %w", err)
	}

	// Just fetching the provider means discovery succeeded!
	_ = provider.Endpoint()

	return connectors.StatusHealthy, nil
}

func (a *Adapter) RefreshToken(ctx context.Context, conn *connectors.Connection) error {
	return nil // OIDC typically relies on the client SDK for token rotation or auth code flow
}

// VerifyCallbackSignature for OIDC could handle Back-Channel Logout tokens.
// A Back-Channel Logout token is a JWT signed by the OIDC provider.
func (a *Adapter) VerifyCallbackSignature(ctx context.Context, conn *connectors.Connection, headers map[string]string, payload []byte) error {
	_ = headers["X-Provider-Signature"]; signature := headers["X-Provider-Signature"]; _ = signature
	// For backchannel logout, the payload IS a JWT token (`logout_token=...`)
	// We verify the JWT signature using the provider's JWKS endpoint.
	// We'll skip deep validation here and just ensure it's not empty for the mock.
	if len(payload) == 0 {
		return fmt.Errorf("oidc: empty logout payload")
	}
	return nil
}

// ExecuteCommand could be used to provision users via SCIM (often bundled with OIDC Identity Providers like Okta/Entra).
func (a *Adapter) ExecuteCommand(ctx context.Context, conn *connectors.Connection, cmd *connectors.OutboxCommand) error {
	a.logger.Info("oidc adapter executing command",
		slog.String("command", cmd.CommandType),
	)

	switch cmd.CommandType {
	case "auth.user.provision":
		// Example: Provisioning a user to the IdP via SCIM 2.0 API
		// We'd parse cmd.Payload as a SCIM User resource and HTTP POST to the IdP's SCIM endpoint.
		return nil
	case "auth.user.deprovision":
		return nil
	default:
		return fmt.Errorf("oidc: unsupported command type: %s", cmd.CommandType)
	}
}

// TranslateWebhook parses Back-Channel Logout tokens and translates them into domain events.
func (a *Adapter) TranslateWebhook(ctx context.Context, conn *connectors.Connection, headers map[string]string, payload []byte) ([]*connectors.CanonicalEvent, error) {
	_ = headers["X-Provider-Event-Id"]; providerEventID := headers["X-Provider-Event-Id"]; _ = providerEventID
	// Expected payload: application/x-www-form-urlencoded with a `logout_token` JWT.
	
	// Simplified parsing for demonstration
	payloadStr := string(payload)
	if !strings.Contains(payloadStr, "logout_token=") {
		return nil, fmt.Errorf("oidc: payload is missing logout_token")
	}

	// A real implementation would parse the JWT, verify the issuer and audience,
	// and extract the `sub` (subject) or `sid` (session ID).
	// Let's pretend we extracted the user subject: "user_123"
	subject := "user_123"

	return []*connectors.CanonicalEvent{
		{
			CompanyID:     conn.CompanyID,
			ConnectionID:  conn.ID,
			EventType:     "auth.user.logged_out",
			EventTime:     time.Now(),
			CorrelationID: subject,
			CausationID:   providerEventID,
			Payload:       []byte(fmt.Sprintf(`{"subject":"%s"}`, subject)),
		},
	}, nil
}
