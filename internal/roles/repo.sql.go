package roles

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

// ListRoles returns all roles.
func (r *Repository) ListRoles(ctx context.Context, filters RoleListFilters) ([]Role, error) {
	query := `SELECT id, name, description, created_at, updated_at FROM roles ORDER BY ` + sortOrderRole(filters.SortBy, filters.SortDir)
	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var roles []Role
	for rows.Next() {
		var role Role
		if err := rows.Scan(&role.ID, &role.Name, &role.Description, &role.CreatedAt, &role.UpdatedAt); err != nil {
			return nil, err
		}
		roles = append(roles, role)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return roles, nil
}

// CreateRole inserts a new role.
func (r *Repository) CreateRole(ctx context.Context, name, description string) (Role, error) {
	var role Role
	err := r.pool.QueryRow(ctx, `INSERT INTO roles (name, description, created_at, updated_at) VALUES ($1, $2, NOW(), NOW()) RETURNING id, name, description, created_at, updated_at`, name, description).Scan(&role.ID, &role.Name, &role.Description, &role.CreatedAt, &role.UpdatedAt)
	return role, err
}

func sortOrderRole(sortBy, sortDir string) string {
	dir := "ASC"
	if sortDir == "desc" {
		dir = "DESC"
	}
	switch sortBy {
	case "name":
		return "name " + dir
	case "created_at":
		return "created_at " + dir
	case "id":
		return "id " + dir
	default:
		return "id ASC"
	}
}
