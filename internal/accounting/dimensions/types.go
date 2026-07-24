package dimensions

import "context"

type Department struct {
	ID     int64
	Code   string
	Name   string
	Active bool
}

type CostCenter struct {
	ID        int64
	Code      string
	Name      string
	Department string
	Active    bool
}

type CreateDepartmentInput struct {
	CompanyID int64
	Code      string
	Name      string
}

type CreateCostCenterInput struct {
	CompanyID    int64
	DepartmentID int64
	Code         string
	Name         string
}

type Repository interface {
	ListDepartments(ctx context.Context, companyID int64) ([]Department, error)
	ListCostCenters(ctx context.Context, companyID int64) ([]CostCenter, error)
	CreateDepartment(ctx context.Context, input CreateDepartmentInput) error
	CreateCostCenter(ctx context.Context, input CreateCostCenterInput) error
	WithTx(ctx context.Context, fn func(context.Context, TxRepository) error) error
}

type TxRepository interface {
	CreateDepartment(ctx context.Context, input CreateDepartmentInput) error
	CreateCostCenter(ctx context.Context, input CreateCostCenterInput) error
}