package oidc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/odyssey-erp/odyssey-erp/internal/connectors"
)

// Adapter implements connectors.ProviderAdapter for OpenID Connect (OIDC).
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
	Issuer       string `json:"issuer"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	SCIMEndpoint string `json:"scim_endpoint"`
	SCIMToken    string `json:"scim_token"`
}

func (a *Adapter) credentials(conn *connectors.Connection) (Credentials, error) {
	secret, err := a.options.ResolveSecret(conn)
	if err != nil {
		return Credentials{}, err
	}
	var creds Credentials
	if err := json.Unmarshal([]byte(secret), &creds); err != nil {
		return Credentials{}, fmt.Errorf("oidc: invalid credential format: %w", err)
	}
	if strings.TrimSpace(creds.Issuer) == "" || strings.TrimSpace(creds.ClientID) == "" {
		return Credentials{}, errors.New("oidc: issuer and client_id are required")
	}
	issuer, err := url.Parse(creds.Issuer)
	if err != nil || issuer.Scheme == "" || issuer.Host == "" {
		return Credentials{}, errors.New("oidc: issuer must be an absolute URL")
	}
	if !a.options.DevelopmentMode && issuer.Scheme != "https" {
		return Credentials{}, errors.New("oidc: production issuer must use HTTPS")
	}
	if strings.TrimSpace(creds.SCIMEndpoint) != "" {
		scimEndpoint, err := url.Parse(creds.SCIMEndpoint)
		if err != nil || scimEndpoint.Scheme == "" || scimEndpoint.Host == "" {
			return Credentials{}, errors.New("oidc: scim_endpoint must be an absolute URL")
		}
		if !a.options.DevelopmentMode && scimEndpoint.Scheme != "https" {
			return Credentials{}, errors.New("oidc: production scim_endpoint must use HTTPS")
		}
	}
	return creds, nil
}

func (a *Adapter) ValidateConnection(ctx context.Context, conn *connectors.Connection) error {
	if _, err := a.credentials(conn); err != nil {
		return fmt.Errorf("oidc: validate credentials: %w", err)
	}
	return nil
}

// CheckHealth performs OIDC discovery against the configured issuer. There is
// no hardcoded fallback issuer.
func (a *Adapter) CheckHealth(ctx context.Context, conn *connectors.Connection) (connectors.ConnectionStatus, error) {
	creds, err := a.credentials(conn)
	if err != nil {
		return connectors.StatusActionRequired, err
	}
	if a.options.DevelopmentMode {
		return connectors.StatusHealthy, nil
	}
	if _, err := a.provider(ctx, creds); err != nil {
		return connectors.StatusActionRequired, fmt.Errorf("oidc: provider discovery failed: %w", err)
	}
	return connectors.StatusHealthy, nil
}

func (a *Adapter) RefreshToken(ctx context.Context, conn *connectors.Connection) error {
	return connectors.ErrUnsupportedOperation
}

// VerifyCallbackSignature verifies and validates an OIDC back-channel logout
// JWT using the issuer's discovered JWKS. The token itself is the signature
// envelope; a caller-controlled header is never trusted.
func (a *Adapter) VerifyCallbackSignature(ctx context.Context, conn *connectors.Connection, headers map[string]string, payload []byte) error {
	creds, err := a.credentials(conn)
	if err != nil {
		return err
	}
	if _, err := a.verifyLogout(ctx, creds, payload); err != nil {
		return fmt.Errorf("oidc: invalid back-channel logout token: %w", err)
	}
	return nil
}

type SCIMUserPayload struct {
	ID          string `json:"id,omitempty"`
	UserName    string `json:"userName,omitempty"`
	DisplayName string `json:"displayName,omitempty"`
	Active      *bool  `json:"active,omitempty"`
}

func (a *Adapter) ExecuteCommand(ctx context.Context, conn *connectors.Connection, cmd *connectors.OutboxCommand) error {
	if cmd == nil {
		return errors.New("oidc: command is required")
	}
	if a.options.DevelopmentMode && strings.TrimSpace(conn.SecretRef) == "" {
		a.logger.Info("OIDC SCIM command simulated in explicit development mode", slog.String("command", cmd.CommandType))
		return nil
	}
	creds, err := a.credentials(conn)
	if err != nil {
		return err
	}
	if strings.TrimSpace(creds.SCIMEndpoint) == "" || strings.TrimSpace(creds.SCIMToken) == "" {
		return errors.New("oidc: scim_endpoint and scim_token are required for SCIM commands")
	}
	key := cmd.CorrelationID
	if cmd.ID > 0 {
		key = fmt.Sprintf("odyssey-command-%d", cmd.ID)
	}

	switch cmd.CommandType {
	case "auth.user.provision":
		var user SCIMUserPayload
		if err := json.Unmarshal(cmd.Payload, &user); err != nil {
			return fmt.Errorf("oidc: invalid SCIM user payload: %w", err)
		}
		if strings.TrimSpace(user.UserName) == "" {
			return errors.New("oidc: SCIM userName is required")
		}
		body, err := json.Marshal(user)
		if err != nil {
			return fmt.Errorf("oidc: marshal SCIM user: %w", err)
		}
		return a.scimRequest(ctx, creds, http.MethodPost, "/Users", body, key)
	case "auth.user.deprovision":
		var user SCIMUserPayload
		if err := json.Unmarshal(cmd.Payload, &user); err != nil {
			return fmt.Errorf("oidc: invalid SCIM deprovision payload: %w", err)
		}
		if strings.TrimSpace(user.ID) == "" {
			return errors.New("oidc: SCIM user id is required")
		}
		return a.scimRequest(ctx, creds, http.MethodDelete, "/Users/"+url.PathEscape(user.ID), nil, key)
	default:
		return fmt.Errorf("oidc: unsupported command type: %s", cmd.CommandType)
	}
}

func (a *Adapter) TranslateWebhook(ctx context.Context, conn *connectors.Connection, headers map[string]string, payload []byte) ([]*connectors.CanonicalEvent, error) {
	creds, err := a.credentials(conn)
	if err != nil {
		return nil, err
	}
	logoutToken, err := a.verifyLogout(ctx, creds, payload)
	if err != nil {
		return nil, fmt.Errorf("oidc: invalid back-channel logout token: %w", err)
	}
	correlationID := logoutToken.Subject
	if correlationID == "" {
		correlationID = logoutToken.SessionID
	}
	claims := map[string]string{
		"subject":    logoutToken.Subject,
		"session_id": logoutToken.SessionID,
		"token_id":   logoutToken.TokenID,
	}
	claimsJSON, _ := json.Marshal(claims)
	return []*connectors.CanonicalEvent{{
		CompanyID:     conn.CompanyID,
		ConnectionID:  conn.ID,
		EventType:     "auth.user.logged_out",
		EventTime:     logoutToken.IssuedAt.UTC(),
		CorrelationID: correlationID,
		CausationID:   logoutToken.TokenID,
		Payload:       claimsJSON,
	}}, nil
}

func (a *Adapter) provider(ctx context.Context, creds Credentials) (*oidc.Provider, error) {
	return oidc.NewProvider(oidc.ClientContext(ctx, a.options.HTTPClientOrDefault()), creds.Issuer)
}

func (a *Adapter) verifyLogout(ctx context.Context, creds Credentials, payload []byte) (*oidc.LogoutToken, error) {
	values, err := url.ParseQuery(string(payload))
	if err != nil {
		return nil, fmt.Errorf("parse form payload: %w", err)
	}
	rawToken := values.Get("logout_token")
	if rawToken == "" {
		return nil, errors.New("logout_token is required")
	}
	provider, err := a.provider(ctx, creds)
	if err != nil {
		return nil, err
	}
	return provider.VerifierContext(ctx, &oidc.Config{ClientID: creds.ClientID}).VerifyLogout(ctx, rawToken)
}

func (a *Adapter) scimRequest(ctx context.Context, creds Credentials, method, path string, body []byte, idempotencyKey string) error {
	endpoint := strings.TrimRight(creds.SCIMEndpoint, "/") + "/" + strings.TrimLeft(path, "/")
	headers := http.Header{
		"Accept":        []string{"application/scim+json, application/json"},
		"Content-Type":  []string{"application/scim+json"},
		"Authorization": []string{"Bearer " + creds.SCIMToken},
	}
	if idempotencyKey != "" {
		headers.Set("Idempotency-Key", idempotencyKey)
	}
	resp, responseBody, err := connectors.DoWithRetry(ctx, a.options.HTTPClientOrDefault(), method, endpoint, body, headers, a.options.RetryPolicyOrDefault())
	if err != nil {
		return fmt.Errorf("oidc: SCIM request failed: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("oidc: SCIM request returned %d: %s", resp.StatusCode, string(responseBody))
	}
	return nil
}
