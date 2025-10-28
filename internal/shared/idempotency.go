package shared

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// IdempotencyStore persists processed keys.
type IdempotencyStore struct {
	pool *pgxpool.Pool
}

// NewIdempotencyStore constructs the store.
func NewIdempotencyStore(pool *pgxpool.Pool) *IdempotencyStore {
	return &IdempotencyStore{pool: pool}
}

// ErrIdempotencyConflict indicates a duplicate key.
var ErrIdempotencyConflict = errors.New("idempotent request already processed")

// CheckAndInsert ensures key uniqueness per module.
func (s *IdempotencyStore) CheckAndInsert(ctx context.Context, key, module string) error {
	if s == nil {
		return errors.New("idempotency store not initialised")
	}
	if key == "" {
		return errors.New("idempotency key required")
	}
	if module == "" {
		return errors.New("idempotency module required")
	}
	_, err := s.pool.Exec(ctx, `INSERT INTO idempotency_keys (key, module, created_at) VALUES ($1, $2, $3)`, key, module, time.Now())
	if err != nil {
		if pgErr, ok := err.(*pgconn.PgError); ok {
			if pgErr.Code == "23505" {
				return ErrIdempotencyConflict
			}
		}
		return err
	}
	return nil
}

// Cleanup removes entries older than retention.
func (s *IdempotencyStore) Cleanup(ctx context.Context, olderThan time.Duration) error {
	if s == nil {
		return nil
	}
	cutoff := time.Now().Add(-olderThan)
	_, err := s.pool.Exec(ctx, `DELETE FROM idempotency_keys WHERE created_at < $1`, cutoff)
	return err
}

// Delete removes a key, typically used to roll back failed processing.
func (s *IdempotencyStore) Delete(ctx context.Context, key string) error {
	if s == nil {
		return nil
	}
	if key == "" {
		return errors.New("idempotency key required")
	}
	_, err := s.pool.Exec(ctx, `DELETE FROM idempotency_keys WHERE key=$1`, key)
	return err
}
