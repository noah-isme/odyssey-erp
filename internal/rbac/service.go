package rbac

import (
	"context"
	"errors"
	"strings"
)

// ErrNotFound indicates that the requested record does not exist.
var ErrNotFound = errors.New("rbac: not found")

// Repository is the database-neutral RBAC persistence boundary.
type Repository interface {
	ListRoles(ctx context.Context) ([]Role, error)
	GetRole(ctx context.Context, id int64) (Role, error)
	CreateRole(ctx context.Context, name, description string) (Role, error)
	UpdateRole(ctx context.Context, id int64, name, description string) (Role, error)
	DeleteRole(ctx context.Context, id int64) (bool, error)
	ListPermissions(ctx context.Context) ([]Permission, error)
	EnsurePermission(ctx context.Context, name, description string) (Permission, error)
	ListRolePermissions(ctx context.Context, roleID int64) ([]Permission, error)
	AttachPermissionToRole(ctx context.Context, roleID, permissionID int64) error
	DetachPermissionFromRole(ctx context.Context, roleID, permissionID int64) error
	AssignRoleToUser(ctx context.Context, userID, roleID int64) error
	RemoveRoleFromUser(ctx context.Context, userID, roleID int64) error
	EffectivePermissions(ctx context.Context, userID int64) ([]string, error)
}

// Service orchestrates RBAC operations and validation.
type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) ListRoles(ctx context.Context) ([]Role, error) {
	return s.repo.ListRoles(ctx)
}

func (s *Service) GetRole(ctx context.Context, id int64) (Role, error) {
	return s.repo.GetRole(ctx, id)
}

func (s *Service) CreateRole(ctx context.Context, name, description string) (Role, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Role{}, errors.New("rbac: role name required")
	}
	return s.repo.CreateRole(ctx, name, strings.TrimSpace(description))
}

func (s *Service) UpdateRole(ctx context.Context, id int64, name, description string) (Role, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Role{}, errors.New("rbac: role name required")
	}
	return s.repo.UpdateRole(ctx, id, name, strings.TrimSpace(description))
}

func (s *Service) DeleteRole(ctx context.Context, id int64) error {
	deleted, err := s.repo.DeleteRole(ctx, id)
	if err != nil {
		return err
	}
	if !deleted {
		return ErrNotFound
	}
	return nil
}

func (s *Service) ListPermissions(ctx context.Context) ([]Permission, error) {
	return s.repo.ListPermissions(ctx)
}

func (s *Service) EnsurePermission(ctx context.Context, name, description string) (Permission, error) {
	return s.repo.EnsurePermission(ctx, strings.TrimSpace(name), strings.TrimSpace(description))
}

func (s *Service) SetRolePermissions(ctx context.Context, roleID int64, permissionIDs []int64) error {
	perms, err := s.repo.ListRolePermissions(ctx, roleID)
	if err != nil {
		return err
	}
	existing := make(map[int64]struct{}, len(perms))
	for _, permission := range perms {
		existing[permission.ID] = struct{}{}
	}
	keep := make(map[int64]struct{}, len(permissionIDs))
	for _, id := range permissionIDs {
		keep[id] = struct{}{}
		if _, ok := existing[id]; !ok {
			if err := s.repo.AttachPermissionToRole(ctx, roleID, id); err != nil {
				return err
			}
		}
	}
	for id := range existing {
		if _, ok := keep[id]; !ok {
			if err := s.repo.DetachPermissionFromRole(ctx, roleID, id); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Service) AssignRole(ctx context.Context, userID, roleID int64) error {
	return s.repo.AssignRoleToUser(ctx, userID, roleID)
}

func (s *Service) RemoveRole(ctx context.Context, userID, roleID int64) error {
	return s.repo.RemoveRoleFromUser(ctx, userID, roleID)
}

func (s *Service) EffectivePermissions(ctx context.Context, userID int64) ([]string, error) {
	return s.repo.EffectivePermissions(ctx, userID)
}
