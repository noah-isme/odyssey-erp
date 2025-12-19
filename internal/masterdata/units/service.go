package units

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

func (s *Service) List(ctx context.Context, filters shared.ListFilters) ([]Unit, int, error) {
	return s.repo.List(ctx, filters)
}

func (s *Service) Get(ctx context.Context, id int64) (Unit, error) {
	if id <= 0 {
		return Unit{}, errors.New("invalid unit ID")
	}
	return s.repo.Get(ctx, id)
}

func (s *Service) Create(ctx context.Context, unit Unit) (Unit, error) {
	if err := s.validate(unit); err != nil {
		return Unit{}, err
	}
	return s.repo.Create(ctx, unit)
}

func (s *Service) Update(ctx context.Context, id int64, unit Unit) error {
	if id <= 0 {
		return errors.New("invalid unit ID")
	}
	if err := s.validate(unit); err != nil {
		return err
	}
	return s.repo.Update(ctx, id, unit)
}

func (s *Service) Delete(ctx context.Context, id int64) error {
	if id <= 0 {
		return errors.New("invalid unit ID")
	}
	return s.repo.Delete(ctx, id)
}
