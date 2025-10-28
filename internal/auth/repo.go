package auth

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	authdb "github.com/odyssey-erp/odyssey-erp/internal/auth/db"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
)

// Repository defines persistence operations for auth module.
type Repository interface {
	FindByEmail(ctx context.Context, email string) (*User, error)
	CreateSession(ctx context.Context, id string, userID int64, expiresAt time.Time, ip, ua string) error
	DeleteSession(ctx context.Context, id string) error
}

// PGRepository implements Repository using PostgreSQL.
type PGRepository struct {
	queries *authdb.Queries
}

// NewRepository constructs a PostgreSQL repository.
func NewRepository(pool *pgxpool.Pool) *PGRepository {
	return &PGRepository{queries: authdb.New(pool)}
}

// FindByEmail fetches a user by email.
func (r *PGRepository) FindByEmail(ctx context.Context, email string) (*User, error) {
	record, err := r.queries.GetUserByEmail(ctx, email)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, shared.ErrNotFound
		}
		return nil, err
	}
	user := &User{
		ID:           record.ID,
		Email:        record.Email,
		PasswordHash: record.PasswordHash,
		IsActive:     record.IsActive,
		CreatedAt:    record.CreatedAt.Time,
		UpdatedAt:    record.UpdatedAt.Time,
	}
	return user, nil
}

// CreateSession persists a new login session in the database for auditing.
func (r *PGRepository) CreateSession(ctx context.Context, id string, userID int64, expiresAt time.Time, ip, ua string) error {
	now := time.Now().UTC()
	return r.queries.CreateSession(ctx, authdb.CreateSessionParams{
		ID:        id,
		UserID:    userID,
		CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
		ExpiresAt: pgtype.Timestamptz{Time: expiresAt.UTC(), Valid: true},
		Ip:        pgtype.Text{String: ip, Valid: ip != ""},
		Ua:        pgtype.Text{String: ua, Valid: ua != ""},
	})
}

// DeleteSession removes a session record from the database.
func (r *PGRepository) DeleteSession(ctx context.Context, id string) error {
	return r.queries.DeleteSession(ctx, id)
}

var _ Repository = (*PGRepository)(nil)
