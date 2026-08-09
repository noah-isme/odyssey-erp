package outbox

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/odyssey-erp/odyssey-erp/internal/sqlc"
)

// Repository manages database interactions for the outbox.
type Repository struct {
	pool    *pgxpool.Pool
	queries *sqlc.Queries
}

// NewRepository constructs an outbox Repository.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{
		pool:    pool,
		queries: sqlc.New(pool),
	}
}

// InsertEvent records a new event using the repository's pool.
func (r *Repository) InsertEvent(ctx context.Context, req PublishRequest) (int64, error) {
	return r.insertEvent(ctx, r.queries, req)
}

// InsertEventTx records a new event on an existing transaction.
func (r *Repository) InsertEventTx(ctx context.Context, tx pgx.Tx, req PublishRequest) (int64, error) {
	return r.insertEvent(ctx, sqlc.New(tx), req)
}

func (r *Repository) insertEvent(ctx context.Context, q *sqlc.Queries, req PublishRequest) (int64, error) {
	payloadBytes, err := json.Marshal(req.Payload)
	if err != nil {
		return 0, err
	}

	causationID := pgtype.UUID{Valid: false}
	if req.CausationID != nil {
		causationID = pgtype.UUID{Bytes: *req.CausationID, Valid: true}
	}

	aggVersion := pgtype.Int4{Valid: false}
	if req.AggregateVersion != nil {
		aggVersion = pgtype.Int4{Int32: *req.AggregateVersion, Valid: true}
	}

	idempKey := pgtype.Text{Valid: false}
	if req.IdempotencyKey != nil {
		idempKey = pgtype.Text{String: *req.IdempotencyKey, Valid: true}
	}

	return q.InsertOutboxEvent(ctx, sqlc.InsertOutboxEventParams{
		CompanyID:        req.CompanyID,
		CorrelationID:    pgtype.UUID{Bytes: req.CorrelationID, Valid: true},
		CausationID:      causationID,
		EventType:        req.EventType,
		AggregateType:    req.AggregateType,
		AggregateID:      req.AggregateID,
		AggregateVersion: aggVersion,
		Payload:          payloadBytes,
		IdempotencyKey:   idempKey,
	})
}
