package qms

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/odyssey-erp/odyssey-erp/internal/outbox"
)

// RegisterOutboxHandlers connects QMS logic to shared outbox events from other modules (like MRP).
func RegisterOutboxHandlers(dispatcher *outbox.Dispatcher, service *Service, logger *slog.Logger) {
	dispatcher.Register("mrp.quality_hold.requested", func(ctx context.Context, event outbox.Event) error {
		var req CreateQualityHoldRequest
		if err := json.Unmarshal(event.Payload, &req); err != nil {
			return fmt.Errorf("failed to decode mrp.quality_hold.requested payload: %w", err)
		}
		// ensure company id is set from event
		req.CompanyID = event.CompanyID
		_, err := service.CreateQualityHold(ctx, req)
		if err != nil {
			logger.Error("failed to create quality hold from outbox", slog.Any("err", err))
			return err
		}
		return nil
	})

	dispatcher.Register("mrp.quality_hold.release_requested", func(ctx context.Context, event outbox.Event) error {
		var payload struct {
			HoldID  int64 `json:"hold_id"`
			ActorID int64 `json:"actor_id"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return err
		}
		err := service.ReleaseQualityHold(ctx, payload.HoldID, event.CompanyID, payload.ActorID)
		if err != nil {
			return err
		}
		return nil
	})

	dispatcher.Register("mrp.inspection.requested", func(ctx context.Context, event outbox.Event) error {
		var req CreateInspectionRequest
		if err := json.Unmarshal(event.Payload, &req); err != nil {
			return err
		}
		req.CompanyID = event.CompanyID
		_, err := service.CreateInspection(ctx, req)
		return err
	})

	dispatcher.Register("mrp.inspection_plan.requested", func(ctx context.Context, event outbox.Event) error {
		var req CreateInspectionPlanRequest
		if err := json.Unmarshal(event.Payload, &req); err != nil {
			return err
		}
		req.CompanyID = event.CompanyID
		_, err := service.CreateInspectionPlan(ctx, req)
		return err
	})

	dispatcher.Register("mrp.ncr.requested", func(ctx context.Context, event outbox.Event) error {
		var req CreateNCRRequest
		if err := json.Unmarshal(event.Payload, &req); err != nil {
			return err
		}
		req.CompanyID = event.CompanyID
		_, err := service.CreateNCR(ctx, req)
		return err
	})

	dispatcher.Register("mrp.capa.requested", func(ctx context.Context, event outbox.Event) error {
		var req CreateCAPARequest
		if err := json.Unmarshal(event.Payload, &req); err != nil {
			return err
		}
		req.CompanyID = event.CompanyID
		_, err := service.CreateCAPA(ctx, req)
		return err
	})
}
