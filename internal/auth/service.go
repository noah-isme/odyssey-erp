package auth

import (
	"context"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/odyssey-erp/odyssey-erp/internal/shared"
)

// Service wraps authentication business rules.
type Service struct {
	repo Repository
}

// NewService constructs a new Service.
func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// Authenticate validates email/password credentials.
func (s *Service) Authenticate(ctx context.Context, email, password string) (*User, error) {
	user, err := s.repo.FindByEmail(ctx, email)
	if err != nil {
		return nil, shared.ErrInvalidCredentials
	}
	if !user.IsActive {
		return nil, shared.ErrInvalidCredentials
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, shared.ErrInvalidCredentials
	}
	return user, nil
}

// RegisterSession persists the session metadata in postgres.
func (s *Service) RegisterSession(ctx context.Context, id string, userID int64, expiresAt time.Time, ip, ua string) error {
	return s.repo.CreateSession(ctx, id, userID, expiresAt, ip, ua)
}

// RemoveSession deletes a session record from postgres.
func (s *Service) RemoveSession(ctx context.Context, id string) error {
	return s.repo.DeleteSession(ctx, id)
}
