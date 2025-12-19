package warehouses

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

func (s *Service) List(ctx context.Context, filters shared.ListFilters) ([]Warehouse, int, error) {
	return s.repo.List(ctx, filters)
}

func (s *Service) Get(ctx context.Context, id int64) (Warehouse, error) {
	if id <= 0 {
		return Warehouse{}, errors.New("invalid warehouse ID")
	}
	return s.repo.Get(ctx, id)
}

func (s *Service) Create(ctx context.Context, warehouse Warehouse) (Warehouse, error) {
	if err := s.validate(warehouse); err != nil {
		return Warehouse{}, err
	}
	return s.repo.Create(ctx, warehouse)
}

func (s *Service) Update(ctx context.Context, id int64, warehouse Warehouse) error {
	if id <= 0 {
		return errors.New("invalid warehouse ID")
	}
	if err := s.validate(warehouse); err != nil {
		return err
	}
	return s.repo.Update(ctx, id, warehouse)
}

func (s *Service) Delete(ctx context.Context, id int64) error {
	if id <= 0 {
		return errors.New("invalid warehouse ID")
	}
	return s.repo.Delete(ctx, id)
}
