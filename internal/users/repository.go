package users

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/odyssey-erp/odyssey-erp/internal/users/db"
)

// Repository provides PostgreSQL backed persistence.
type Repository struct {
	pool    *pgxpool.Pool
	queries *usersdb.Queries
}

// NewRepository constructs a repository.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{
		pool:    pool,
		queries: usersdb.New(pool),
	}
}

// ListUsers returns all users.
func (r *Repository) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := r.queries.ListUsers(ctx)
	if err != nil {
		return nil, err
	}
	
	users := make([]User, len(rows))
	for i, row := range rows {
		users[i] = User{
			ID:        row.ID,
			Email:     row.Email,
			Name:      row.Name,
			IsActive:  row.IsActive,
			CreatedAt: row.CreatedAt.Time,
			UpdatedAt: row.UpdatedAt.Time,
		}
	}
	return users, nil
}
