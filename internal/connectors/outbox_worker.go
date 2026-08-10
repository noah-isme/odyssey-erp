package connectors

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"
)

// ProviderRegistry resolves adapters by provider name.
type ProviderRegistry interface {
	GetAdapter(provider string) (ProviderAdapter, error)
}

// OutboxWorker polls and executes pending connector outbox commands.
type OutboxWorker struct {
	repo     OutboxRepository
	registry ProviderRegistry
	logger   *slog.Logger
}

type OutboxWorkerOption func(*OutboxWorker)

func WithOutboxWorkerLogger(logger *slog.Logger) OutboxWorkerOption {
	return func(worker *OutboxWorker) {
		if logger != nil {
			worker.logger = logger
		}
	}
}

// NewOutboxWorker creates a new worker for external connector commands.
func NewOutboxWorker(repo OutboxRepository, registry ProviderRegistry, options ...OutboxWorkerOption) *OutboxWorker {
	worker := &OutboxWorker{
		repo:     repo,
		registry: registry,
		logger:   slog.Default(),
	}
	for _, option := range options {
		if option != nil {
			option(worker)
		}
	}
	return worker
}

// ProcessPending polls the database and dispatches commands to the appropriate provider adapter.
func (w *OutboxWorker) ProcessPending(ctx context.Context, limit int32) error {
	commands, err := w.repo.GetPendingOutboxCommands(ctx, limit)
	if err != nil {
		return fmt.Errorf("failed to fetch pending connector outbox commands: %w", err)
	}

	for _, cmd := range commands {
		w.processCommand(ctx, &cmd)
	}

	return nil
}

func (w *OutboxWorker) processCommand(ctx context.Context, sqlCmd *OutboxCommand) {
	connRec, err := w.repo.GetConnection(ctx, sqlCmd.CompanyID, sqlCmd.ConnectionID)
	if err != nil {
		w.markFailure(ctx, sqlCmd, fmt.Errorf("connection not found: %w", err))
		return
	}

	adapter, err := w.registry.GetAdapter(connRec.Provider)
	if err != nil {
		w.markFailure(ctx, sqlCmd, fmt.Errorf("provider adapter not found: %w", err))
		return
	}

	conn := &Connection{
		ID:        connRec.ID,
		CompanyID: connRec.CompanyID,
		Provider:  connRec.Provider,
		Type:      connRec.Type,
		Name:      connRec.Name,
		SecretRef: connRec.SecretRef,
		Status:    ConnectionStatus(connRec.Status),
	}

	err = adapter.ExecuteCommand(ctx, conn, sqlCmd)
	if err != nil {
		w.markFailure(ctx, sqlCmd, err)
		return
	}

	if refundRepo, ok := w.repo.(PaymentRefundStateRepository); ok && sqlCmd.CommandType == "payment.refund" {
		if refundKey := refundKeyFromPayload(sqlCmd.Payload); refundKey != "" {
			if err := refundRepo.MarkPaymentRefundProcessing(ctx, sqlCmd.CompanyID, sqlCmd.ConnectionID, refundKey); err != nil {
				w.logger.Error("connector refund dispatch state update failed", slog.Int64("outbox_id", sqlCmd.ID), slog.Any("error", err))
			}
		}
	}
	if err := w.repo.UpdateOutboxCommandState(ctx, OutboxCommandStateUpdate{ID: sqlCmd.ID, State: "completed", NextAttempt: time.Now()}); err != nil {
		w.logger.Error("connector outbox completion update failed", slog.Int64("outbox_id", sqlCmd.ID), slog.Any("error", err))
	}
}

func (w *OutboxWorker) markFailure(ctx context.Context, sqlCmd *OutboxCommand, execErr error) {
	// Log the execErr in a real system
	attempts := sqlCmd.Attempts + 1
	var nextState string
	var nextAttempt time.Time

	if attempts >= 5 {
		nextState = "dead_letter"
		nextAttempt = time.Now()
	} else {
		nextState = "pending"
		// Exponential backoff
		backoffDuration := time.Duration(1<<attempts) * time.Minute
		nextAttempt = time.Now().Add(backoffDuration)
	}

	if err := w.repo.UpdateOutboxCommandState(ctx, OutboxCommandStateUpdate{ID: sqlCmd.ID, State: nextState, NextAttempt: nextAttempt}); err != nil {
		w.logger.Error("connector outbox failure update failed", slog.Int64("outbox_id", sqlCmd.ID), slog.Any("error", err))
		return
	}
	if nextState != "dead_letter" {
		return
	}
	if deadLetterWriter, ok := w.repo.(ConnectorDeadLetterWriter); ok {
		if err := deadLetterWriter.RecordConnectorDeadLetter(ctx, *sqlCmd, execErr); err != nil {
			w.logger.Error("connector dead-letter record failed", slog.Int64("outbox_id", sqlCmd.ID), slog.Any("error", err))
		}
	}
	if refundRepo, ok := w.repo.(PaymentRefundStateRepository); ok && sqlCmd.CommandType == "payment.refund" {
		if refundKey := refundKeyFromPayload(sqlCmd.Payload); refundKey != "" {
			if err := refundRepo.MarkPaymentRefundFailed(ctx, sqlCmd.CompanyID, sqlCmd.ConnectionID, refundKey, execErr); err != nil {
				w.logger.Error("connector refund dead-letter state update failed", slog.Int64("outbox_id", sqlCmd.ID), slog.Any("error", err))
			}
		}
	}
}

func refundKeyFromPayload(payload []byte) string {
	var value struct {
		RefundKey string `json:"refund_key"`
	}
	if json.Unmarshal(payload, &value) != nil {
		return ""
	}
	return value.RefundKey
}
