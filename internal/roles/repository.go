package roles

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/odyssey-erp/odyssey-erp/internal/roles/db"
)

// Repository provides PostgreSQL backed persistence.
type Repository struct {
	pool    *pgxpool.Pool
	queries *rolesdb.Queries
}

// NewRepository constructs a repository.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{
		pool:    pool,
		queries: rolesdb.New(pool),
	}
}

// ListRoles returns all roles.
func (r *Repository) ListRoles(ctx context.Context, filters RoleListFilters) ([]Role, error) {
	rows, err := r.queries.ListRoles(ctx, rolesdb.ListRolesParams{
		SortBy:  filters.SortBy,
		SortDir: filters.SortDir,
	})
	if err != nil {
		return nil, err
	}
	
	roles := make([]Role, len(rows))
	for i, row := range rows {
		roles[i] = Role{
			ID:          row.ID,
			Name:        row.Name,
			Description: row.Description,
			CreatedAt:   row.CreatedAt.Time,
			UpdatedAt:   row.UpdatedAt.Time,
		}
	}
	return roles, nil
}

// CreateRole inserts a new role.
func (r *Repository) CreateRole(ctx context.Context, name, description string) (Role, error) {
	row, err := r.queries.CreateRole(ctx, rolesdb.CreateRoleParams{
		Name:        name,
		Description: description,
	})
	if err != nil {
		return Role{}, err
	}
	
	return Role{
		ID:          row.ID,
		Name:        row.Name,
		Description: row.Description,
		CreatedAt:   row.CreatedAt.Time,
		UpdatedAt:   row.UpdatedAt.Time,
	}, nil
}
