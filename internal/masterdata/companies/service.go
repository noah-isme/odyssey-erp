package companies

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

func (s *Service) List(ctx context.Context, filters shared.ListFilters) ([]Company, int, error) {
	return s.repo.List(ctx, filters)
}

func (s *Service) Get(ctx context.Context, id int64) (Company, error) {
	if id <= 0 {
		return Company{}, errors.New("invalid company ID")
	}
	return s.repo.Get(ctx, id)
}

func (s *Service) Create(ctx context.Context, company Company) (Company, error) {
	if err := s.validate(company); err != nil {
		return Company{}, err
	}
	return s.repo.Create(ctx, company)
}

func (s *Service) Update(ctx context.Context, id int64, company Company) error {
	if id <= 0 {
		return errors.New("invalid company ID")
	}
	if err := s.validate(company); err != nil {
		return err
	}
	return s.repo.Update(ctx, id, company)
}

func (s *Service) Delete(ctx context.Context, id int64) error {
	if id <= 0 {
		return errors.New("invalid company ID")
	}
	return s.repo.Delete(ctx, id)
}
