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

	p.logger.Info("Starting bank feed sync", "connection_id", payload.ConnectionID)
	err := p.service.SyncConnection(ctx, payload.ConnectionID)
	if err != nil {
		p.logger.Error("Bank feed sync failed", "connection_id", payload.ConnectionID, "error", err)
		return err
	}

	p.logger.Info("Bank feed sync completed successfully", "connection_id", payload.ConnectionID)
	return nil
}

func (p *BankFeedsProcessor) ProcessEventTask(ctx context.Context, t *asynq.Task) error {
	var payload BankFeedsEventPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("json.Unmarshal failed: %v: %w", err, asynq.SkipRetry)
	}

	p.logger.Info("Processing bank feed event", "event_id", payload.EventID)
	// Currently s.service.ProcessWebhookEvent is not implemented.
	// We would normally route this to the service to fetch the event, parse it via FeedPort, and insert transactions.
	
	return nil
}
