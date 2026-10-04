package connectors

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// maxOutboxAttempts mirrors the `attempts < 5` guard in ClaimOutboxCommand and
// the `attempts >= 5` guard in DeadLetterExhaustedOutboxCommands.
const maxOutboxAttempts = 5

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
//
// Delivery semantics: every command is claimed (CAS that bumps attempts and
// leases the row for 10 minutes) before the adapter runs, so overlapping
// sweeps never execute the same command concurrently. External execution is
// still at-least-once: a crash after the adapter call and before the
// completed update re-executes the command once the lease expires. Making the
// external effect idempotent relies on provider idempotency keys derived from
// the command's correlation_id. A worker that crashes repeatedly consumes an
// attempt per claim, and exhausted commands are dead-lettered here.
func (w *OutboxWorker) ProcessPending(ctx context.Context, limit int32) error {
	if _, err := w.repo.DeadLetterExhaustedOutboxCommands(ctx); err != nil {
		return fmt.Errorf("failed to dead-letter exhausted connector outbox commands: %w", err)
	}

	commands, err := w.repo.GetPendingOutboxCommands(ctx, limit)
	if err != nil {
		return fmt.Errorf("failed to fetch pending connector outbox commands: %w", err)
	}

	var claimErr error
	for _, cmd := range commands {
		if err := w.processCommand(ctx, &cmd); err != nil && claimErr == nil {
			claimErr = err
		}
	}

	return claimErr
}

// processCommand claims and executes one command. It returns an error only
// when the claim itself failed for a reason other than "not claimable"; the
// command then stays unclaimed for a later sweep. Execution failures are
// recorded on the command row, not returned.
func (w *OutboxWorker) processCommand(ctx context.Context, pending *OutboxCommand) error {
	claimed, err := w.repo.ClaimOutboxCommand(ctx, pending.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		// Another run owns the command, it is not due yet, or it is exhausted.
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to claim connector outbox command %d: %w", pending.ID, err)
	}
	sqlCmd := &claimed

	connRec, err := w.repo.GetConnection(ctx, sqlCmd.CompanyID, sqlCmd.ConnectionID)
	if err != nil {
		w.markFailure(ctx, sqlCmd, fmt.Errorf("connection not found: %w", err))
		return nil
	}

	adapter, err := w.registry.GetAdapter(connRec.Provider)
	if err != nil {
		w.markFailure(ctx, sqlCmd, fmt.Errorf("provider adapter not found: %w", err))
		return nil
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
		return nil
	}

	_ = w.repo.UpdateOutboxCommandState(ctx, OutboxCommandStateUpdate{ID: sqlCmd.ID, State: "completed", NextAttempt: time.Now()})
	return nil
}

func (w *OutboxWorker) markFailure(ctx context.Context, sqlCmd *OutboxCommand, execErr error) {
	// Log the execErr in a real system
	// The claim already incremented attempts; UpdateOutboxCommandState no
	// longer does, so the claimed value is the attempt count.
	attempts := sqlCmd.Attempts
	var nextState string
	var nextAttempt time.Time

	if attempts >= maxOutboxAttempts {
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
