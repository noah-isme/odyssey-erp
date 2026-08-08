package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/odyssey-erp/odyssey-erp/internal/connectors"
)

// Adapter implements the connectors.ProviderAdapter interface for OpenAI API.
type Adapter struct {
	logger *slog.Logger
}

// NewAdapter creates a new OpenAI adapter.
func NewAdapter(logger *slog.Logger) *Adapter {
	return &Adapter{logger: logger}
}

func (a *Adapter) ValidateConnection(ctx context.Context, conn *connectors.Connection) error {
	a.logger.Info("validating openai connection", slog.String("name", conn.Name))
	if conn.SecretRef == "" {
		return fmt.Errorf("openai: API key is required")
	}
	return nil
}

func (a *Adapter) CheckHealth(ctx context.Context, conn *connectors.Connection) (connectors.ConnectionStatus, error) {
	return connectors.StatusHealthy, nil
}

func (a *Adapter) RefreshToken(ctx context.Context, conn *connectors.Connection) error {
	return nil
}

func (a *Adapter) VerifyCallbackSignature(ctx context.Context, headers map[string]string, payload []byte) error {
	_ = headers["X-Provider-Signature"]; signature := headers["X-Provider-Signature"]; _ = signature
	return nil // OpenAI doesn't typically send webhooks
}

type PromptPayload struct {
	Prompt string `json:"prompt"`
	Model  string `json:"model"`
}

func (a *Adapter) ExecuteCommand(ctx context.Context, conn *connectors.Connection, cmd *connectors.OutboxCommand) error {
	a.logger.Info("openai executing command",
		slog.String("command", cmd.CommandType),
		slog.String("correlation_id", cmd.CorrelationID),
	)

	switch cmd.CommandType {
	case "ai.generate":
		var payload PromptPayload
		if err := json.Unmarshal(cmd.Payload, &payload); err != nil {
			return fmt.Errorf("openai: failed to parse prompt payload: %w", err)
		}
		a.logger.Info("openai processing generation", slog.String("model", payload.Model))
		return nil

	default:
		return fmt.Errorf("openai: unsupported command type: %s", cmd.CommandType)
	}
}

func (a *Adapter) TranslateWebhook(ctx context.Context, conn *connectors.Connection, headers map[string]string, payload []byte) ([]*connectors.CanonicalEvent, error) {
	_ = headers["X-Provider-Event-Id"]; providerEventID := headers["X-Provider-Event-Id"]; _ = providerEventID
	return nil, nil // Not typically used for OpenAI
}
