package connectors

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/odyssey-erp/odyssey-erp/internal/sqlc"
)

// ProviderRegistry resolves adapters by provider name.
type ProviderRegistry interface {
	GetAdapter(provider string) (ProviderAdapter, error)
}

// OutboxWorker polls and executes pending connector outbox commands.
type OutboxWorker struct {
	queries  *sqlc.Queries
	registry ProviderRegistry
}

// NewOutboxWorker creates a new worker for external connector commands.
func NewOutboxWorker(queries *sqlc.Queries, registry ProviderRegistry) *OutboxWorker {
	return &OutboxWorker{
		queries:  queries,
		registry: registry,
	}
}

// ProcessPending polls the database and dispatches commands to the appropriate provider adapter.
func (w *OutboxWorker) ProcessPending(ctx context.Context, limit int32) error {
	commands, err := w.queries.GetPendingOutboxCommands(ctx, limit)
	if err != nil {
		return fmt.Errorf("failed to fetch pending connector outbox commands: %w", err)
	}

	for _, cmd := range commands {
		w.processCommand(ctx, &cmd)
	}

	return nil
}

func (w *OutboxWorker) processCommand(ctx context.Context, sqlCmd *sqlc.ConnectorOutboxCommand) {
	connRec, err := w.queries.GetConnection(ctx, sqlc.GetConnectionParams{
		ID:        sqlCmd.ConnectionID,
		CompanyID: sqlCmd.CompanyID,
	})
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

	domainCmd := &OutboxCommand{
		ID:            sqlCmd.ID,
		CompanyID:     sqlCmd.CompanyID,
		ConnectionID:  sqlCmd.ConnectionID,
		CommandType:   sqlCmd.CommandType,
		CorrelationID: sqlCmd.CorrelationID,
		Payload:       sqlCmd.Payload,
		State:         sqlCmd.State,
		Attempts:      int(sqlCmd.Attempts),
		NextAttempt:   sqlCmd.NextAttempt.Time,
		CreatedAt:     sqlCmd.CreatedAt.Time,
		UpdatedAt:     sqlCmd.UpdatedAt.Time,
	}

	err = adapter.ExecuteCommand(ctx, conn, domainCmd)
	if err != nil {
		w.markFailure(ctx, sqlCmd, err)
		return
	}

	_, _ = w.queries.UpdateOutboxCommandState(ctx, sqlc.UpdateOutboxCommandStateParams{
		ID:          sqlCmd.ID,
		State:       "completed",
		NextAttempt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
	})
}

func (w *OutboxWorker) markFailure(ctx context.Context, sqlCmd *sqlc.ConnectorOutboxCommand, execErr error) {
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

	_, _ = w.queries.UpdateOutboxCommandState(ctx, sqlc.UpdateOutboxCommandStateParams{
		ID:          sqlCmd.ID,
		State:       nextState,
		NextAttempt: pgtype.Timestamptz{Time: nextAttempt, Valid: true},
	})
}
