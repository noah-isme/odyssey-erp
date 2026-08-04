package cmms

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/odyssey-erp/odyssey-erp/internal/outbox"
)

// FixedAssetDisposedPayload defines the expected payload when a fixed asset is disposed.
type FixedAssetDisposedPayload struct {
	AssetID  int64   `json:"asset_id"`
	Date     string  `json:"date"`
	Proceeds float64 `json:"proceeds"`
}

// CalibrationRequestedPayload defines the expected payload for qms.calibration.required.
type CalibrationRequestedPayload struct {
	FindingID int64 `json:"finding_id"`
	AssetID   int64 `json:"asset_id"`
	ActorID   int64 `json:"actor_id"`
}

// RegisterOutboxHandlers connects CMMS logic to shared outbox events.
func RegisterOutboxHandlers(dispatcher *outbox.Dispatcher, service *Service, logger *slog.Logger) {
	dispatcher.Register("fixed_assets.asset.disposed", func(ctx context.Context, event outbox.Event) error {
		logger.Info("received fixed_assets.asset.disposed event", slog.Int64("event_id", event.ID), slog.Int64("aggregate_id", event.AggregateID))

		var payload FixedAssetDisposedPayload
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return fmt.Errorf("failed to decode payload: %w", err)
		}

		// In a complete implementation, CMMS Asset table would have fixed_asset_id.
		// Since we don't have it in the schema, we'll log the intention for this POC:
		logger.Info("cross-module integration: CMMS would disable maintenance for asset linked to disposed fixed asset", 
			slog.Int64("fixed_asset_id", payload.AssetID),
			slog.String("disposal_date", payload.Date),
		)

		return nil
	})

	dispatcher.Register("qms.calibration.required", func(ctx context.Context, event outbox.Event) error {
		logger.Info("received qms.calibration.required event", slog.Int64("event_id", event.ID), slog.Int64("aggregate_id", event.AggregateID))

		var payload CalibrationRequestedPayload
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return fmt.Errorf("failed to decode calibration payload: %w", err)
		}

		// Create a Work Order for the requested calibration
		req := CreateWorkOrderRequest{
			CompanyID:   event.CompanyID,
			Title:       fmt.Sprintf("Calibration for Asset %d (QMS Finding %d)", payload.AssetID, payload.FindingID),
			Description: fmt.Sprintf("Calibration required due to QMS Audit Finding %d", payload.FindingID),
			AssetID:     &payload.AssetID,
			Priority:    PriorityHigh,
			Category:    "CALIBRATION",
			RequesterID: payload.ActorID,
			ActorID:     payload.ActorID,
		}

		_, err := service.CreateWorkOrder(ctx, req)
		if err != nil {
			logger.Error("failed to create calibration work order", slog.Any("err", err))
			return err
		}

		return nil
	})
}
