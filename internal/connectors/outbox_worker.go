package connectors

import (
	"context"
	"fmt"
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
}

// NewOutboxWorker creates a new worker for external connector commands.
func NewOutboxWorker(repo OutboxRepository, registry ProviderRegistry) *OutboxWorker {
	return &OutboxWorker{
		repo:     repo,
		registry: registry,
	}
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

	_ = w.repo.UpdateOutboxCommandState(ctx, OutboxCommandStateUpdate{ID: sqlCmd.ID, State: "completed", NextAttempt: time.Now()})
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

	_ = w.repo.UpdateOutboxCommandState(ctx, OutboxCommandStateUpdate{ID: sqlCmd.ID, State: nextState, NextAttempt: nextAttempt})
}
