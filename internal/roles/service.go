package roles

import (
	"context"
)

// RepositoryPort defines data access methods for roles.
type RepositoryPort interface {
	ListRoles(ctx context.Context) ([]Role, error)
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
func (s *Service) ListRoles(ctx context.Context) ([]Role, error) {
	return s.repo.ListRoles(ctx)
}
