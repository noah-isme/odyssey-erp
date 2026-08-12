package automation

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// OutboxStore is the minimal persistence boundary required by the worker.
// Claim/Complete/Fail are deliberately separate so a handler can never mark
// a message complete unless it still owns the lease.
type OutboxStore interface {
	Claim(context.Context, string, int, RetryPolicy) ([]OutboxMessage, error)
	Complete(context.Context, int64, string) error
	Fail(context.Context, int64, string, error, time.Time) (OutboxStatus, error)
	DeadLetter(context.Context, int64, string, error) error
}

type OperationHandler func(context.Context, OutboxMessage) error

// Dispatcher is an at-least-once outbox consumer. Operation handlers must use
// the message's company, correlation, and idempotency key when mutating state.
// Re-delivery is expected and is safe at the queue boundary.
type Dispatcher struct {
	store    OutboxStore
	workerID string
	policy   RetryPolicy
	logger   *slog.Logger
	handlers map[string]OperationHandler
}

func NewDispatcher(store OutboxStore, workerID string, logger *slog.Logger) *Dispatcher {
	return &Dispatcher{
		store:    store,
		workerID: strings.TrimSpace(workerID),
		policy:   DefaultRetryPolicy(),
		logger:   logger,
		handlers: make(map[string]OperationHandler),
	}
}

func (d *Dispatcher) Register(operation string, handler OperationHandler) error {
	operation = strings.TrimSpace(operation)
	if operation == "" || handler == nil {
		return ErrInvalidOutboxMessage
	}
	if _, exists := d.handlers[operation]; exists {
		return fmt.Errorf("finance automation: handler already registered for %s", operation)
	}
	d.handlers[operation] = handler
	return nil
}

// DispatchFinanceAutomation claims and processes up to limit messages. It
// continues after individual failures so one command cannot starve the queue.
func (d *Dispatcher) DispatchFinanceAutomation(ctx context.Context, limit int) error {
	if d == nil || d.store == nil || d.workerID == "" || limit < 1 {
		return ErrInvalidOutboxMessage
	}
	messages, err := d.store.Claim(ctx, d.workerID, limit, d.policy)
	if err != nil {
		return err
	}
	var failures []error
	for _, message := range messages {
		handler := d.handlers[message.Operation]
		if handler == nil {
			failure := fmt.Errorf("finance automation operation %q is not configured", message.Operation)
			if err := d.store.DeadLetter(ctx, message.ID, d.workerID, failure); err != nil {
				failures = append(failures, fmt.Errorf("dead-letter %s: %w", message, err))
			} else {
				failures = append(failures, failure)
			}
			continue
		}

		err := handler(ctx, message)
		if err == nil {
			if completeErr := d.store.Complete(ctx, message.ID, d.workerID); completeErr != nil {
				failures = append(failures, fmt.Errorf("complete %s: %w", message, completeErr))
			}
			continue
		}
		if d.logger != nil {
			d.logger.Error("finance automation command failed", slog.Any("error", err), slog.Int64("outbox_id", message.ID), slog.String("operation", message.Operation))
		}
		if !retryableOutboxError(err) {
			if deadErr := d.store.DeadLetter(ctx, message.ID, d.workerID, err); deadErr != nil {
				failures = append(failures, fmt.Errorf("dead-letter %s: %w", message, deadErr))
			} else {
				failures = append(failures, err)
			}
			continue
		}
		retryAt := time.Now().Add(retryDelay(message.Attempts))
		if _, failErr := d.store.Fail(ctx, message.ID, d.workerID, err, retryAt); failErr != nil {
			failures = append(failures, fmt.Errorf("retry %s: %w", message, failErr))
		} else {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

func retryableOutboxError(err error) bool {
	if errors.Is(err, ErrAmbiguousOutcome) {
		// The remote side may have accepted the command. Retrying the outbox
		// message would be an unsafe blind resubmission; operators must first
		// resolve the provider state and replay deliberately.
		return false
	}
	var providerErr *ProviderError
	if errors.As(err, &providerErr) {
		return providerErr.Retryable()
	}
	return true
}

func retryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	if attempt > 6 {
		attempt = 6
	}
	return time.Duration(1<<(attempt-1)) * time.Second
}
