package mappings

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/odyssey-erp/odyssey-erp/internal/accounting/shared"
)

type Repository interface {
	Get(ctx context.Context, module, key string) (AccountMapping, error)
}

type repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) Repository {
	return &repository{db: db}
}

// Get resolves an account mapping for the specified key.
func (r *repository) Get(ctx context.Context, module, key string) (AccountMapping, error) {
	if module == "" || key == "" {
		return AccountMapping{}, errors.New("accounting: module and key required")
	}
	normalized := strings.ToUpper(module)
	var mapping AccountMapping
	err := r.db.QueryRow(ctx, `SELECT module, key, account_id, created_at, updated_at FROM account_mappings WHERE module=$1 AND key=$2`, normalized, key).
		Scan(&mapping.Module, &mapping.Key, &mapping.AccountID, &mapping.CreatedAt, &mapping.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return AccountMapping{}, shared.ErrMappingNotFound
		}
		return AccountMapping{}, err
	}
	return mapping, nil
}
