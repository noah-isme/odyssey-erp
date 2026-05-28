package accounts

import (
	"context"
	"github.com/odyssey-erp/odyssey-erp/internal/accounting/reports"
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) List(ctx context.Context) ([]Account, error) {
	return s.repo.List(ctx)
}

func (s *Service) ListBalances(ctx context.Context) ([]reports.AccountBalance, error) {
	return s.repo.ListBalances(ctx)
}
