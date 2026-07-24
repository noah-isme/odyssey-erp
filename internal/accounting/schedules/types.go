package schedules

import (
	"context"
	"time"
)

type Schedule struct {
	ID              int64
	Type            string
	Recipients      []string
	Frequency       string
	Active          bool
	LastSentAt      *time.Time
	PeriodOffset    int
	DepartmentID    int64
	CostCenterID    int64
}

type CreateScheduleInput struct {
	CompanyID     int64
	Type          string
	Recipients    []string
	Frequency     string
	DepartmentID  int64
	CostCenterID  int64
	PeriodOffset  int
}

type Repository interface {
	List(ctx context.Context, companyID int64) ([]Schedule, error)
	Create(ctx context.Context, input CreateScheduleInput) error
	Toggle(ctx context.Context, id, companyID int64) error
	Retry(ctx context.Context, id, companyID int64) error
	WithTx(ctx context.Context, fn func(context.Context, TxRepository) error) error
}

type TxRepository interface {
	Create(ctx context.Context, input CreateScheduleInput) error
	Toggle(ctx context.Context, id, companyID int64) error
	Retry(ctx context.Context, id, companyID int64) error
}