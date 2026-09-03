package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/hibiken/asynq"
	"github.com/odyssey-erp/odyssey-erp/internal/finance/bankfeeds"
)

const (
	TypeBankFeedsSync  = "bankfeeds:sync"
	TypeBankFeedsEvent = "bankfeeds:event"
)

type BankFeedsSyncPayload struct {
	ConnectionID int64 `json:"connection_id"`
}

type BankFeedsEventPayload struct {
	EventID int64 `json:"event_id"`
}

// NewBankFeedsEventTask creates the durable consumer task used by the HTTP
// inbox. The event itself remains the idempotency boundary in PostgreSQL.
func NewBankFeedsEventTask(eventID int64) (*asynq.Task, error) {
	if eventID <= 0 {
		return nil, fmt.Errorf("bank feed event id is required")
	}
	body, err := json.Marshal(BankFeedsEventPayload{EventID: eventID})
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TypeBankFeedsEvent, body), nil
}

type BankFeedsProcessor struct {
	service *bankfeeds.Service
	logger  *slog.Logger
}

func NewBankFeedsProcessor(service *bankfeeds.Service, logger *slog.Logger) *BankFeedsProcessor {
	return &BankFeedsProcessor{
		service: service,
		logger:  logger,
	}
}

func (p *BankFeedsProcessor) ProcessSyncTask(ctx context.Context, t *asynq.Task) error {
	var payload BankFeedsSyncPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("json.Unmarshal failed: %v: %w", err, asynq.SkipRetry)
	}

	if p.logger != nil {
		p.logger.Info("Starting bank feed sync", "connection_id", payload.ConnectionID)
	}
	err := p.service.SyncConnection(ctx, payload.ConnectionID)
	if err != nil {
		if p.logger != nil {
			p.logger.Error("Bank feed sync failed", "connection_id", payload.ConnectionID, "error", err)
		}
		return err
	}

	if p.logger != nil {
		p.logger.Info("Bank feed sync completed successfully", "connection_id", payload.ConnectionID)
	}
	return nil
}

func (p *BankFeedsProcessor) ProcessEventTask(ctx context.Context, t *asynq.Task) error {
	var payload BankFeedsEventPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("json.Unmarshal failed: %v: %w", err, asynq.SkipRetry)
	}
	if payload.EventID <= 0 {
		return fmt.Errorf("bank feed event id is required: %w", asynq.SkipRetry)
	}

	if p.logger != nil {
		p.logger.Info("Processing bank feed event", "event_id", payload.EventID)
	}
	if err := p.service.ProcessWebhookEvent(ctx, payload.EventID); err != nil {
		if p.logger != nil {
			p.logger.Error("bank feed event processing failed", "event_id", payload.EventID, "error", err)
		}
		return err
	}
	if p.logger != nil {
		p.logger.Info("Bank feed event processed", "event_id", payload.EventID)
	}
	return nil
}
