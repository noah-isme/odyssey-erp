package schedules

import (
	"context"
	"errors"
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) List(ctx context.Context, companyID int64) ([]Schedule, error) {
	if companyID == 0 {
		return nil, errors.New("company required")
	}
	return s.repo.List(ctx, companyID)
}

func (s *Service) Create(ctx context.Context, companyID int64, typ, frequency string, departmentID, costCenterID int64, periodOffset int, recipients []string) error {
	if companyID == 0 || len(recipients) == 0 || (typ != "PNL" && typ != "BUDGET_VS_ACTUAL") || (frequency != "DAILY" && frequency != "WEEKLY" && frequency != "MONTHLY") {
		return errors.New("invalid schedule configuration")
	}
	return s.repo.Create(ctx, CreateScheduleInput{
		CompanyID:    companyID,
		Type:         typ,
		Recipients:   recipients,
		Frequency:    frequency,
		DepartmentID: departmentID,
		CostCenterID: costCenterID,
		PeriodOffset: periodOffset,
	})
}

func (s *Service) Toggle(ctx context.Context, id, companyID int64) error {
	if id == 0 || companyID == 0 {
		return errors.New("id and company required")
	}
	return s.repo.Toggle(ctx, id, companyID)
}

func (s *Service) Retry(ctx context.Context, id, companyID int64) error {
	if id == 0 || companyID == 0 {
		return errors.New("id and company required")
	}
	return s.repo.Retry(ctx, id, companyID)
}