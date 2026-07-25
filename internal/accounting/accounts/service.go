package accounts

import (
	"context"
	"github.com/odyssey-erp/odyssey-erp/internal/accounting/reports"
	"time"
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

// ListBalancesForPeriod returns posted journal activity for one calendar month.
func (s *Service) ListBalancesForPeriod(ctx context.Context, year int, month time.Month) ([]reports.AccountBalance, error) {
	return s.repo.ListBalancesForPeriod(ctx, year, month)
}

func (s *Service) ListBalancesForPeriodAndDimensions(ctx context.Context, year int, month time.Month, filter reports.DimensionFilter) ([]reports.AccountBalance, error) {
	return s.repo.ListBalancesForPeriodAndDimensions(ctx, year, month, filter)
}
