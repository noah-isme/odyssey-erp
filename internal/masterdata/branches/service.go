package branches

import (
	"context"
	"errors"

	"github.com/odyssey-erp/odyssey-erp/internal/masterdata/shared"
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) List(ctx context.Context, filters shared.ListFilters) ([]Branch, int, error) {
	return s.repo.List(ctx, filters)
}

func (s *Service) Get(ctx context.Context, id int64) (Branch, error) {
	if id <= 0 {
		return Branch{}, errors.New("invalid branch ID")
	}
	return s.repo.Get(ctx, id)
}

func (s *Service) Create(ctx context.Context, branch Branch) (Branch, error) {
	if err := s.validate(branch); err != nil {
		return Branch{}, err
	}
	return s.repo.Create(ctx, branch)
}

func (s *Service) Update(ctx context.Context, id int64, branch Branch) error {
	if id <= 0 {
		return errors.New("invalid branch ID")
	}
	if err := s.validate(branch); err != nil {
		return err
	}
	return s.repo.Update(ctx, id, branch)
}

func (s *Service) Delete(ctx context.Context, id int64) error {
	if id <= 0 {
		return errors.New("invalid branch ID")
	}
	return s.repo.Delete(ctx, id)
}
