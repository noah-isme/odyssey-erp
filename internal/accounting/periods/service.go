package periods

import (
	"context"
	"time"
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) FindOpenPeriodByDate(ctx context.Context, date time.Time) (Period, error) {
	return s.repo.FindOpenPeriodByDate(ctx, date)
}
