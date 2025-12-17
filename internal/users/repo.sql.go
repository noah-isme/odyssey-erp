package users

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository provides PostgreSQL backed persistence.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository constructs a repository.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// ListUsers returns all users.
func (r *Repository) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := r.pool.Query(ctx, `SELECT id, email, name, is_active, created_at, updated_at FROM users ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []User
	for rows.Next() {
		var user User
		if err := rows.Scan(&user.ID, &user.Email, &user.Name, &user.IsActive, &user.CreatedAt, &user.UpdatedAt); err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return users, nil
}
