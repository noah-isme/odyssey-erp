package roles

import (
	"context"
)

// RepositoryPort defines data access methods for roles.
type RepositoryPort interface {
	ListRoles(ctx context.Context, filters RoleListFilters) ([]Role, error)
	CreateRole(ctx context.Context, name, description string) (Role, error)
}

// Service handles role business logic.
type Service struct {
	repo RepositoryPort
}

// NewService builds Service instance.
func NewService(repo RepositoryPort) *Service {
	return &Service{repo: repo}
}

// ListRoles returns all roles.
func (s *Service) ListRoles(ctx context.Context, filters RoleListFilters) ([]Role, error) {
	return s.repo.ListRoles(ctx, filters)
}

// CreateRole inserts a new role.
func (s *Service) CreateRole(ctx context.Context, name, description string) (Role, error) {
	return s.repo.CreateRole(ctx, name, description)
}
