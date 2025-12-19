package taxes

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

func (s *Service) List(ctx context.Context, filters shared.ListFilters) ([]Tax, int, error) {
	return s.repo.List(ctx, filters)
}

func (s *Service) Get(ctx context.Context, id int64) (Tax, error) {
	if id <= 0 {
		return Tax{}, errors.New("invalid tax ID")
	}
	return s.repo.Get(ctx, id)
}

func (s *Service) Create(ctx context.Context, tax Tax) (Tax, error) {
	if err := s.validate(tax); err != nil {
		return Tax{}, err
	}
	return s.repo.Create(ctx, tax)
}

func (s *Service) Update(ctx context.Context, id int64, tax Tax) error {
	if id <= 0 {
		return errors.New("invalid tax ID")
	}
	if err := s.validate(tax); err != nil {
		return err
	}
	return s.repo.Update(ctx, id, tax)
}

func (s *Service) Delete(ctx context.Context, id int64) error {
	if id <= 0 {
		return errors.New("invalid tax ID")
	}
	return s.repo.Delete(ctx, id)
}
