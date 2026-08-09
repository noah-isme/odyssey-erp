package rbac

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/odyssey-erp/odyssey-erp/internal/sqlc"
)

// PGRepository adapts generated RBAC persistence types to domain types.
type PGRepository struct {
	queries *sqlc.Queries
}

func NewRepository(pool *pgxpool.Pool) *PGRepository {
	return &PGRepository{queries: sqlc.New(pool)}
}

func (r *PGRepository) ListRoles(ctx context.Context) ([]Role, error) {
	rows, err := r.queries.RbacListRoles(ctx)
	if err != nil {
		return nil, err
	}
	roles := make([]Role, len(rows))
	for i, row := range rows {
		roles[i] = mapRole(row)
	}
	return roles, nil
}

func (r *PGRepository) GetRole(ctx context.Context, id int64) (Role, error) {
	row, err := r.queries.GetRole(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Role{}, ErrNotFound
	}
	if err != nil {
		return Role{}, err
	}
	return mapRole(row), nil
}

func (r *PGRepository) CreateRole(ctx context.Context, name, description string) (Role, error) {
	row, err := r.queries.RbacCreateRole(ctx, sqlc.RbacCreateRoleParams{Name: name, Description: description})
	if err != nil {
		return Role{}, err
	}
	return mapRole(row), nil
}

func (r *PGRepository) UpdateRole(ctx context.Context, id int64, name, description string) (Role, error) {
	row, err := r.queries.UpdateRole(ctx, sqlc.UpdateRoleParams{ID: id, Name: name, Description: description})
	if errors.Is(err, pgx.ErrNoRows) {
		return Role{}, ErrNotFound
	}
	if err != nil {
		return Role{}, err
	}
	return mapRole(row), nil
}

func (r *PGRepository) DeleteRole(ctx context.Context, id int64) (bool, error) {
	rows, err := r.queries.DeleteRole(ctx, id)
	return rows > 0, err
}

func (r *PGRepository) ListPermissions(ctx context.Context) ([]Permission, error) {
	rows, err := r.queries.ListPermissions(ctx)
	if err != nil {
		return nil, err
	}
	permissions := make([]Permission, len(rows))
	for i, row := range rows {
		permissions[i] = Permission{ID: row.ID, Name: row.Name, Description: row.Description}
	}
	return permissions, nil
}

func (r *PGRepository) EnsurePermission(ctx context.Context, name, description string) (Permission, error) {
	row, err := r.queries.CreatePermission(ctx, sqlc.CreatePermissionParams{Name: name, Description: description})
	if err != nil {
		return Permission{}, err
	}
	return Permission{ID: row.ID, Name: row.Name, Description: row.Description}, nil
}

func (r *PGRepository) ListRolePermissions(ctx context.Context, roleID int64) ([]Permission, error) {
	rows, err := r.queries.ListRolePermissions(ctx, roleID)
	if err != nil {
		return nil, err
	}
	permissions := make([]Permission, len(rows))
	for i, row := range rows {
		permissions[i] = Permission{ID: row.ID, Name: row.Name, Description: row.Description}
	}
	return permissions, nil
}

func (r *PGRepository) AttachPermissionToRole(ctx context.Context, roleID, permissionID int64) error {
	return r.queries.AttachPermissionToRole(ctx, sqlc.AttachPermissionToRoleParams{RoleID: roleID, PermissionID: permissionID})
}

func (r *PGRepository) DetachPermissionFromRole(ctx context.Context, roleID, permissionID int64) error {
	return r.queries.DetachPermissionFromRole(ctx, sqlc.DetachPermissionFromRoleParams{RoleID: roleID, PermissionID: permissionID})
}

func (r *PGRepository) AssignRoleToUser(ctx context.Context, userID, roleID int64) error {
	return r.queries.AssignRoleToUser(ctx, sqlc.AssignRoleToUserParams{UserID: userID, RoleID: roleID})
}

func (r *PGRepository) RemoveRoleFromUser(ctx context.Context, userID, roleID int64) error {
	return r.queries.RemoveRoleFromUser(ctx, sqlc.RemoveRoleFromUserParams{UserID: userID, RoleID: roleID})
}

func (r *PGRepository) EffectivePermissions(ctx context.Context, userID int64) ([]string, error) {
	return r.queries.UserEffectivePermissions(ctx, userID)
}

func mapRole(row sqlc.Role) Role {
	return Role{
		ID:          row.ID,
		Name:        row.Name,
		Description: row.Description,
		CreatedAt:   safeTime(row.CreatedAt.Time),
		UpdatedAt:   safeTime(row.UpdatedAt.Time),
	}
}

func safeTime(t time.Time) time.Time {
	if t.IsZero() {
		return time.Time{}
	}
	return t
}
