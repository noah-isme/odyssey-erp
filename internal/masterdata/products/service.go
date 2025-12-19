package products

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

func (s *Service) List(ctx context.Context, filters shared.ListFilters) ([]Product, int, error) {
	return s.repo.List(ctx, filters)
}

func (s *Service) Get(ctx context.Context, id int64) (Product, error) {
	if id <= 0 {
		return Product{}, errors.New("invalid product ID")
	}
	return s.repo.Get(ctx, id)
}

func (s *Service) Create(ctx context.Context, product Product) (Product, error) {
	if err := s.validate(product); err != nil {
		return Product{}, err
	}
	return s.repo.Create(ctx, product)
}

func (s *Service) Update(ctx context.Context, id int64, product Product) error {
	if id <= 0 {
		return errors.New("invalid product ID")
	}
	if err := s.validate(product); err != nil {
		return err
	}
	return s.repo.Update(ctx, id, product)
}

func (s *Service) Delete(ctx context.Context, id int64) error {
	if id <= 0 {
		return errors.New("invalid product ID")
	}
	return s.repo.Delete(ctx, id)
}
