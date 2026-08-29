package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"

	"github.com/hibiken/asynq"
	"github.com/odyssey-erp/odyssey-erp/internal/finance/bankfeeds"
)

const (
	TypeBankFeedsSync     = "bankfeeds:sync"
	TypeBankFeedsSyncScan = "bankfeeds:sync_scan"
	TypeBankFeedsEvent    = "bankfeeds:event"
)

type BankFeedsSyncPayload struct {
	ConnectionID int64 `json:"connection_id"`
}

type BankFeedsEventPayload struct {
	EventID int64 `json:"event_id"`
}

// NewBankFeedsSyncScanTask creates the profile-gated scheduler task that
// expands into one scoped connection sync per due company-owned connection.
// The scan carries no company wildcard or provider credentials.
func NewBankFeedsSyncScanTask() *asynq.Task {
	return asynq.NewTask(TypeBankFeedsSyncScan, nil, asynq.Queue(QueueDefault))
}

// NewBankFeedsSyncTask creates the durable polling task used by an approved
// scheduler or an operator-triggered retry. The connection ID is the tenant
// boundary; no provider credentials or company-wide wildcard is accepted.
func NewBankFeedsSyncTask(connectionID int64) (*asynq.Task, error) {
	if connectionID <= 0 {
		return nil, fmt.Errorf("bank feed connection id is required")
	}
	body, err := json.Marshal(BankFeedsSyncPayload{ConnectionID: connectionID})
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TypeBankFeedsSync, body), nil
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

// EnqueueBankFeedsSync submits one already-scoped connection for polling. The
// stable task ID collapses duplicate scheduler ticks or operator requests
// while the task is queued; a later tick can enqueue it after completion.
func EnqueueBankFeedsSync(ctx context.Context, client *asynq.Client, connectionID int64) (*asynq.TaskInfo, error) {
	task, err := NewBankFeedsSyncTask(connectionID)
	if err != nil {
		return nil, err
	}
	if client == nil {
		return nil, fmt.Errorf("bank feed sync: queue client is not configured")
	}
	info, err := client.EnqueueContext(ctx, task,
		asynq.Queue(QueueDefault),
		asynq.MaxRetry(10),
		asynq.TaskID("bank-feed-sync:"+strconv.FormatInt(connectionID, 10)),
	)
	if errors.Is(err, asynq.ErrTaskIDConflict) {
		return nil, nil
	}
	return info, err
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
	if p == nil || p.service == nil {
		return fmt.Errorf("bank feed sync: service not configured: %w", asynq.SkipRetry)
	}
	if t == nil {
		return fmt.Errorf("bank feed sync: task not configured: %w", asynq.SkipRetry)
	}
	var payload BankFeedsSyncPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("json.Unmarshal failed: %v: %w", err, asynq.SkipRetry)
	}
	if payload.ConnectionID <= 0 {
		return fmt.Errorf("bank feed sync: connection id is required: %w", asynq.SkipRetry)
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
	if p == nil || p.service == nil {
		return fmt.Errorf("bank feed event: service not configured: %w", asynq.SkipRetry)
	}
	if t == nil {
		return fmt.Errorf("bank feed event: task not configured: %w", asynq.SkipRetry)
	}
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
