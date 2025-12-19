package accounts

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository interface {
	List(ctx context.Context) ([]Account, error)
}

type repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) Repository {
	return &repository{db: db}
}

func (r *repository) List(ctx context.Context) ([]Account, error) {
	rows, err := r.db.Query(ctx, `SELECT id, code, name, type, parent_id, is_active, created_at, updated_at FROM accounts ORDER BY code`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var accounts []Account
	for rows.Next() {
		var a Account
		err := rows.Scan(&a.ID, &a.Code, &a.Name, &a.Type, &a.ParentID, &a.IsActive, &a.CreatedAt, &a.UpdatedAt)
		if err != nil {
			return nil, err
		}
		accounts = append(accounts, a)
	}
	return accounts, rows.Err()
}
