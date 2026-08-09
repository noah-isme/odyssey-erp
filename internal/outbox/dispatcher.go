package outbox

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Handler processes an outbox event.
type Handler func(ctx context.Context, event Event) error

type eventStore interface {
	ClaimPending(ctx context.Context, limit int) ([]Event, error)
	MarkPublished(ctx context.Context, id int64) error
	MarkFailed(ctx context.Context, id int64, errStr string) error
}

type postgresEventStore struct {
	pool *pgxpool.Pool
}

// Dispatcher polls and routes outbox events to registered handlers.
type Dispatcher struct {
	pool     *pgxpool.Pool
	repo     *Repository
	logger   *slog.Logger
	handlers map[string][]Handler
	store    eventStore
}

// NewDispatcher constructs a Dispatcher.
func NewDispatcher(pool *pgxpool.Pool, repo *Repository, logger *slog.Logger) *Dispatcher {
	return &Dispatcher{
		pool:     pool,
		repo:     repo,
		logger:   logger,
		handlers: make(map[string][]Handler),
		store:    &postgresEventStore{pool: pool},
	}
}

// Register adds a handler for a specific event type.
func (d *Dispatcher) Register(eventType string, handler Handler) {
	d.handlers[eventType] = append(d.handlers[eventType], handler)
}

// ProcessPending claims up to limit events and dispatches them.
func (d *Dispatcher) ProcessPending(ctx context.Context, limit int) error {
	events, err := d.store.ClaimPending(ctx, limit)
	if err != nil {
		return err
	}
	for _, e := range events {
		d.processEvent(ctx, e)
	}
	return nil
}

func (s *postgresEventStore) ClaimPending(ctx context.Context, limit int) ([]Event, error) {
	// A simple approach: we find all companies, then process events per company.
	// In a real multi-tenant system we'd iterate over active companies.
	// For now, let's just use a direct SQL update to claim the oldest unpublished events.
	rows, err := s.pool.Query(ctx, `
		WITH claimed AS (
			SELECT id
			FROM outbox_events
			WHERE published_at IS NULL AND publish_attempts < 10
			ORDER BY created_at ASC
			FOR UPDATE SKIP LOCKED
			LIMIT $1
		)
		UPDATE outbox_events o
		SET publish_attempts = publish_attempts + 1
		FROM claimed
		WHERE o.id = claimed.id
		RETURNING o.id, o.company_id, o.correlation_id, o.causation_id, o.event_type,
		          o.aggregate_type, o.aggregate_id, o.aggregate_version, o.payload, o.idempotency_key, o.created_at, o.publish_attempts
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []Event
	for rows.Next() {
		var e Event
		if err := rows.Scan(
			&e.ID, &e.CompanyID, &e.CorrelationID, &e.CausationID, &e.EventType,
			&e.AggregateType, &e.AggregateID, &e.AggregateVersion, &e.Payload, &e.IdempotencyKey, &e.CreatedAt, &e.PublishAttempts,
		); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close() // Close early so we can do further queries
	return events, nil
}

func (d *Dispatcher) processEvent(ctx context.Context, e Event) {
	handlers := d.handlers[e.EventType]
	if len(handlers) == 0 {
		// No handlers registered, just mark as published (or keep it pending?)
		// Usually we mark as published to avoid blocking the queue if it's an event no one cares about anymore.
		d.markPublished(ctx, e.ID)
		return
	}

	var hasError error
	for _, h := range handlers {
		if err := h(ctx, e); err != nil {
			hasError = err
			break
		}
	}

	if hasError != nil {
		if d.logger != nil {
			d.logger.Warn("outbox handler failed", slog.Int64("event_id", e.ID), slog.String("event_type", e.EventType), slog.Any("error", hasError))
		}
		d.markFailed(ctx, e.ID, hasError.Error())
	} else {
		d.markPublished(ctx, e.ID)
	}
}

func (d *Dispatcher) markPublished(ctx context.Context, id int64) {
	err := d.store.MarkPublished(ctx, id)
	if err != nil {
		if d.logger != nil {
			d.logger.Error("failed to mark event published", slog.Int64("event_id", id), slog.Any("error", err))
		}
	}
}

func (d *Dispatcher) markFailed(ctx context.Context, id int64, errStr string) {
	// Let's cap the error string length
	if len(errStr) > 2000 {
		errStr = errStr[:2000]
	}
	err := d.store.MarkFailed(ctx, id, errStr)
	if err != nil {
		if d.logger != nil {
			d.logger.Error("failed to mark event failed", slog.Int64("event_id", id), slog.Any("error", err))
		}
	}
}

func (s *postgresEventStore) MarkPublished(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx, "UPDATE outbox_events SET published_at = NOW() WHERE id = $1", id)
	return err
}

func (s *postgresEventStore) MarkFailed(ctx context.Context, id int64, errStr string) error {
	_, err := s.pool.Exec(ctx, "UPDATE outbox_events SET last_error = $2 WHERE id = $1", id, errStr)
	return err
}
