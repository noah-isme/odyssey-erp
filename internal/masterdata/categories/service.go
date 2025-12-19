package categories

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

func (s *Service) List(ctx context.Context, filters shared.ListFilters) ([]Category, int, error) {
	return s.repo.List(ctx, filters)
}

func (s *Service) Get(ctx context.Context, id int64) (Category, error) {
	if id <= 0 {
		return Category{}, errors.New("invalid category ID")
	}
	return s.repo.Get(ctx, id)
}

func (s *Service) Create(ctx context.Context, category Category) (Category, error) {
	if err := s.validate(category); err != nil {
		return Category{}, err
	}
	return s.repo.Create(ctx, category)
}

func (s *Service) Update(ctx context.Context, id int64, category Category) error {
	if id <= 0 {
		return errors.New("invalid category ID")
	}
	if err := s.validate(category); err != nil {
		return err
	}
	return s.repo.Update(ctx, id, category)
}

func (s *Service) Delete(ctx context.Context, id int64) error {
	if id <= 0 {
		return errors.New("invalid category ID")
	}
	return s.repo.Delete(ctx, id)
}
