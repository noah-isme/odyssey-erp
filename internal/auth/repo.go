package auth

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/sqlc"
)

// Repository defines persistence operations for auth module.
type Repository interface {
	FindByEmail(ctx context.Context, email string) (*User, error)
	FindByID(ctx context.Context, id int64) (*User, error)
	UpdateMFA(ctx context.Context, id int64, mfaEnabled bool, totpSecret string) error
	CreateSession(ctx context.Context, id string, userID int64, expiresAt time.Time, ip, ua string) error
	DeleteSession(ctx context.Context, id string) error
}

// OIDCConnection contains only the provider settings needed by the HTTP OIDC flow.
// The generated connector row remains private to this repository adapter.
type OIDCConnection struct {
	Provider  string
	SecretRef string
}

// ConnectionReader loads the provider settings used by the SSO boundary.
type ConnectionReader interface {
	GetConnection(ctx context.Context, id, companyID int64) (OIDCConnection, error)
}

// PGRepository implements Repository using PostgreSQL.
type PGRepository struct {
	queries *sqlc.Queries
}

// NewRepository constructs a PostgreSQL repository.
func NewRepository(pool *pgxpool.Pool) *PGRepository {
	return &PGRepository{queries: sqlc.New(pool)}
}

// FindByEmail fetches a user by email.
func (r *PGRepository) FindByEmail(ctx context.Context, email string) (*User, error) {
	record, err := r.queries.AuthGetUserByEmail(ctx, email)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, shared.ErrNotFound
		}
		return nil, err
	}
	totpSecret := ""
	if record.TotpSecret.Valid {
		totpSecret = record.TotpSecret.String
	}
	user := &User{
		ID:           record.ID,
		Email:        record.Email,
		PasswordHash: record.PasswordHash,
		IsActive:     record.IsActive,
		MFAEnabled:   record.MfaEnabled,
		TOTPSecret:   totpSecret,
		CreatedAt:    record.CreatedAt.Time,
		UpdatedAt:    record.UpdatedAt.Time,
	}
	return user, nil
}

// FindByID fetches a user by ID.
func (r *PGRepository) FindByID(ctx context.Context, id int64) (*User, error) {
	record, err := r.queries.AuthGetUserByID(ctx, id)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, shared.ErrNotFound
		}
		return nil, err
	}
	totpSecret := ""
	if record.TotpSecret.Valid {
		totpSecret = record.TotpSecret.String
	}
	user := &User{
		ID:           record.ID,
		Email:        record.Email,
		PasswordHash: record.PasswordHash,
		IsActive:     record.IsActive,
		MFAEnabled:   record.MfaEnabled,
		TOTPSecret:   totpSecret,
		CreatedAt:    record.CreatedAt.Time,
		UpdatedAt:    record.UpdatedAt.Time,
	}
	return user, nil
}

// UpdateMFA updates the MFA settings for a user.
func (r *PGRepository) UpdateMFA(ctx context.Context, id int64, mfaEnabled bool, totpSecret string) error {
	secretNull := pgtype.Text{Valid: false}
	if totpSecret != "" {
		secretNull = pgtype.Text{String: totpSecret, Valid: true}
	}
	return r.queries.UpdateUserMFA(ctx, sqlc.UpdateUserMFAParams{
		ID:         id,
		MfaEnabled: mfaEnabled,
		TotpSecret: secretNull,
	})
}

// CreateSession persists a new login session in the database for auditing.
func (r *PGRepository) CreateSession(ctx context.Context, id string, userID int64, expiresAt time.Time, ip, ua string) error {
	now := time.Now().UTC()
	return r.queries.CreateSession(ctx, sqlc.CreateSessionParams{
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

// GetConnection loads a connector configuration for the SSO flow.
func (r *PGRepository) GetConnection(ctx context.Context, id, companyID int64) (OIDCConnection, error) {
	row, err := r.queries.GetConnection(ctx, sqlc.GetConnectionParams{ID: id, CompanyID: companyID})
	if err != nil {
		return OIDCConnection{}, err
	}
	return OIDCConnection{Provider: row.Provider, SecretRef: row.SecretRef}, nil
}

var _ Repository = (*PGRepository)(nil)
var _ ConnectionReader = (*PGRepository)(nil)
