package rbac

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/odyssey-erp/odyssey-erp/internal/rbac/db"
)

// ErrNotFound indicates that the requested record does not exist.
var ErrNotFound = errors.New("rbac: not found")

// Service orchestrates RBAC operations.
type Service struct {
	queries *rbacdb.Queries
}

// NewService constructs a Service backed by the provided pool.
func NewService(pool *pgxpool.Pool) *Service {
	return &Service{queries: rbacdb.New(pool)}
}

// ListRoles returns all roles ordered by name.
func (s *Service) ListRoles(ctx context.Context) ([]Role, error) {
	rows, err := s.queries.ListRoles(ctx)
	if err != nil {
		return nil, err
	}
	roles := make([]Role, 0, len(rows))
	for _, row := range rows {
		roles = append(roles, toDomainRole(row))
	}
	return roles, nil
}

// GetRole fetches a role by ID.
func (s *Service) GetRole(ctx context.Context, id int64) (Role, error) {
	row, err := s.queries.GetRole(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Role{}, ErrNotFound
		}
		return Role{}, err
	}
	return toDomainRole(row), nil
}

// CreateRole inserts a new role.
func (s *Service) CreateRole(ctx context.Context, name, description string) (Role, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Role{}, errors.New("rbac: role name required")
	}
	row, err := s.queries.CreateRole(ctx, rbacdb.CreateRoleParams{
		Name:        name,
		Description: strings.TrimSpace(description),
	})
	if err != nil {
		return Role{}, err
	}
	return toDomainRole(row), nil
}

// UpdateRole updates an existing role.
func (s *Service) UpdateRole(ctx context.Context, id int64, name, description string) (Role, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Role{}, errors.New("rbac: role name required")
	}
	row, err := s.queries.UpdateRole(ctx, rbacdb.UpdateRoleParams{
		ID:          id,
		Name:        name,
		Description: strings.TrimSpace(description),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Role{}, ErrNotFound
		}
		return Role{}, err
	}
	return toDomainRole(row), nil
}

// DeleteRole removes a role by ID. Returns ErrNotFound if nothing was deleted.
func (s *Service) DeleteRole(ctx context.Context, id int64) error {
	rows, err := s.queries.DeleteRole(ctx, id)
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

// ListPermissions returns all permissions ordered by name.
func (s *Service) ListPermissions(ctx context.Context) ([]Permission, error) {
	rows, err := s.queries.ListPermissions(ctx)
	if err != nil {
		return nil, err
	}
	perms := make([]Permission, 0, len(rows))
	for _, row := range rows {
		perms = append(perms, Permission{
			ID:          row.ID,
			Name:        row.Name,
			Description: row.Description,
		})
	}
	return perms, nil
}

// EnsurePermission upserts a permission ensuring description is stored.
func (s *Service) EnsurePermission(ctx context.Context, name, description string) (Permission, error) {
	row, err := s.queries.CreatePermission(ctx, rbacdb.CreatePermissionParams{
		Name:        strings.TrimSpace(name),
		Description: strings.TrimSpace(description),
	})
	if err != nil {
		return Permission{}, err
	}
	return Permission{ID: row.ID, Name: row.Name, Description: row.Description}, nil
}

// SetRolePermissions replaces permissions for a role.
func (s *Service) SetRolePermissions(ctx context.Context, roleID int64, permissionIDs []int64) error {
	// Remove existing assignments not in the new set by simple delete + reinsert approach.
	// For now we delete everything and reattach to keep logic straightforward.
	perms, err := s.queries.ListRolePermissions(ctx, roleID)
	if err != nil {
		return err
	}
	existing := make(map[int64]struct{}, len(perms))
	for _, p := range perms {
		existing[p.ID] = struct{}{}
	}
	keep := make(map[int64]struct{}, len(permissionIDs))
	for _, id := range permissionIDs {
		keep[id] = struct{}{}
		if _, ok := existing[id]; !ok {
			if err := s.queries.AttachPermissionToRole(ctx, rbacdb.AttachPermissionToRoleParams{RoleID: roleID, PermissionID: id}); err != nil {
				return err
			}
		}
	}
	for id := range existing {
		if _, ok := keep[id]; !ok {
			if err := s.queries.DetachPermissionFromRole(ctx, rbacdb.DetachPermissionFromRoleParams{RoleID: roleID, PermissionID: id}); err != nil {
				return err
			}
		}
	}
	return nil
}

// AssignRole assigns a role to the given user.
func (s *Service) AssignRole(ctx context.Context, userID, roleID int64) error {
	return s.queries.AssignRoleToUser(ctx, rbacdb.AssignRoleToUserParams{
		UserID: userID,
		RoleID: roleID,
	})
}

// RemoveRole removes a role from a user.
func (s *Service) RemoveRole(ctx context.Context, userID, roleID int64) error {
	return s.queries.RemoveRoleFromUser(ctx, rbacdb.RemoveRoleFromUserParams{
		UserID: userID,
		RoleID: roleID,
	})
}

// EffectivePermissions returns deduplicated permission names for a user.
func (s *Service) EffectivePermissions(ctx context.Context, userID int64) ([]string, error) {
	rows, err := s.queries.UserEffectivePermissions(ctx, userID)
	if err != nil {
		return nil, err
	}
	perms := make([]string, len(rows))
	copy(perms, rows)
	return perms, nil
}

func toDomainRole(row rbacdb.Role) Role {
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
