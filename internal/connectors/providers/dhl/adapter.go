package dhl

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/odyssey-erp/odyssey-erp/internal/connectors"
)

// Adapter implements the connectors.ProviderAdapter interface for DHL API.
type Adapter struct {
	logger *slog.Logger
}

// NewAdapter creates a new DHL adapter.
func NewAdapter(logger *slog.Logger) *Adapter {
	return &Adapter{logger: logger}
}

func (a *Adapter) ValidateConnection(ctx context.Context, conn *connectors.Connection) error {
	a.logger.Info("validating dhl connection", slog.String("name", conn.Name))
	if conn.SecretRef == "" {
		return fmt.Errorf("dhl: API credentials required")
	}
	return nil
}

func (a *Adapter) CheckHealth(ctx context.Context, conn *connectors.Connection) (connectors.ConnectionStatus, error) {
	return connectors.StatusHealthy, nil
}

func (a *Adapter) RefreshToken(ctx context.Context, conn *connectors.Connection) error {
	return nil
}

func (a *Adapter) VerifyCallbackSignature(ctx context.Context, conn *connectors.Connection, headers map[string]string, payload []byte) error {
	_ = headers["X-Provider-Signature"]; signature := headers["X-Provider-Signature"]; _ = signature
	return nil
}

type ShipmentPayload struct {
	Origin      string `json:"origin"`
	Destination string `json:"destination"`
	Weight      float64 `json:"weight"`
}

func (a *Adapter) ExecuteCommand(ctx context.Context, conn *connectors.Connection, cmd *connectors.OutboxCommand) error {
	a.logger.Info("dhl executing command",
		slog.String("command", cmd.CommandType),
		slog.String("correlation_id", cmd.CorrelationID),
	)

	switch cmd.CommandType {
	case "shipment.book":
		var payload ShipmentPayload
		if err := json.Unmarshal(cmd.Payload, &payload); err != nil {
			return fmt.Errorf("dhl: failed to parse shipment payload: %w", err)
		}
		a.logger.Info("dhl booking shipment", slog.String("dest", payload.Destination))
		return nil

	default:
		return fmt.Errorf("dhl: unsupported command type: %s", cmd.CommandType)
	}
}

func (a *Adapter) TranslateWebhook(ctx context.Context, conn *connectors.Connection, headers map[string]string, payload []byte) ([]*connectors.CanonicalEvent, error) {
	_ = headers["X-Provider-Event-Id"]; providerEventID := headers["X-Provider-Event-Id"]; _ = providerEventID
	evt := &connectors.CanonicalEvent{
		CompanyID:     conn.CompanyID,
		ConnectionID:  conn.ID,
		EventType:     "shipment.status_updated",
		EventTime:     time.Now(),
		CorrelationID: providerEventID,
		CausationID:   providerEventID,
		Payload:       payload,
	}
	return []*connectors.CanonicalEvent{evt}, nil
}
