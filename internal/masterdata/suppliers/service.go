package suppliers

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

func (s *Service) List(ctx context.Context, filters shared.ListFilters) ([]Supplier, int, error) {
	return s.repo.List(ctx, filters)
}

func (s *Service) Get(ctx context.Context, id int64) (Supplier, error) {
	if id <= 0 {
		return Supplier{}, errors.New("invalid supplier ID")
	}
	return s.repo.Get(ctx, id)
}

func (s *Service) Create(ctx context.Context, supplier Supplier) (Supplier, error) {
	if err := s.validate(supplier); err != nil {
		return Supplier{}, err
	}
	return s.repo.Create(ctx, supplier)
}

func (s *Service) Update(ctx context.Context, id int64, supplier Supplier) error {
	if id <= 0 {
		return errors.New("invalid supplier ID")
	}
	if err := s.validate(supplier); err != nil {
		return err
	}
	return s.repo.Update(ctx, id, supplier)
}

func (s *Service) Delete(ctx context.Context, id int64) error {
	if id <= 0 {
		return errors.New("invalid supplier ID")
	}
	return s.repo.Delete(ctx, id)
}
