package users

import (
	"context"
)

// RepositoryPort defines data access methods for users.
type RepositoryPort interface {
	ListUsers(ctx context.Context) ([]User, error)
}

// Service handles user business logic.
type Service struct {
	repo RepositoryPort
}

// NewService builds Service instance.
func NewService(repo RepositoryPort) *Service {
	return &Service{repo: repo}
}

// ListUsers returns all users.
func (s *Service) ListUsers(ctx context.Context) ([]User, error) {
	return s.repo.ListUsers(ctx)
}
