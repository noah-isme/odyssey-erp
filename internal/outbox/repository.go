package outbox

import (
	"context"
	"encoding/json"

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

// Queries returns the raw sqlc.Queries instance, for use with transactional contexts.
func (r *Repository) Queries() *sqlc.Queries {
	return r.queries
}

// InsertEvent records a new event transactionally if passed the correct DB context.
func (r *Repository) InsertEvent(ctx context.Context, q *sqlc.Queries, req PublishRequest) (int64, error) {
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
