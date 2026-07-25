package dimensions

import (
	"context"
	"errors"
	"strings"
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) ListDepartments(ctx context.Context, companyID int64) ([]Department, error) {
	return s.repo.ListDepartments(ctx, companyID)
}

func (s *Service) ListCostCenters(ctx context.Context, companyID int64) ([]CostCenter, error) {
	return s.repo.ListCostCenters(ctx, companyID)
}

func (s *Service) CreateDepartment(ctx context.Context, companyID int64, code, name string) error {
	code = strings.TrimSpace(code)
	name = strings.TrimSpace(name)
	if companyID == 0 || code == "" || name == "" {
		return errors.New("company, code, and name are required")
	}
	return s.repo.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
		return tx.CreateDepartment(ctx, CreateDepartmentInput{CompanyID: companyID, Code: code, Name: name})
	})
}

func (s *Service) CreateCostCenter(ctx context.Context, companyID int64, code, name string, departmentID int64) error {
	code = strings.TrimSpace(code)
	name = strings.TrimSpace(name)
	if companyID == 0 || code == "" || name == "" {
		return errors.New("company, code, and name are required")
	}
	return s.repo.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
		return tx.CreateCostCenter(ctx, CreateCostCenterInput{CompanyID: companyID, DepartmentID: departmentID, Code: code, Name: name})
	})
}