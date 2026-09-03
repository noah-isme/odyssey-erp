package dimensions

import (
	"context"
	"testing"
)

type dimensionTxFake struct {
	department  CreateDepartmentInput
	costCenter  CreateCostCenterInput
	departments int
	costCenters int
}

func (r *dimensionTxFake) CreateDepartment(_ context.Context, in CreateDepartmentInput) error {
	r.departments++
	r.department = in
	return nil
}
func (r *dimensionTxFake) CreateCostCenter(_ context.Context, in CreateCostCenterInput) error {
	r.costCenters++
	r.costCenter = in
	return nil
}

type serviceRepo struct{ tx dimensionTxFake }

func (r *serviceRepo) ListDepartments(context.Context, int64) ([]Department, error)  { return nil, nil }
func (r *serviceRepo) ListCostCenters(context.Context, int64) ([]CostCenter, error)  { return nil, nil }
func (r *serviceRepo) CreateDepartment(context.Context, CreateDepartmentInput) error { return nil }
func (r *serviceRepo) CreateCostCenter(context.Context, CreateCostCenterInput) error { return nil }
func (r *serviceRepo) WithTx(ctx context.Context, fn func(context.Context, TxRepository) error) error {
	return fn(ctx, &r.tx)
}

func TestCreateDimensionValidationAndNormalization(t *testing.T) {
	repo := &serviceRepo{}
	svc := NewService(repo)
	ctx := context.Background()
	if err := svc.CreateDepartment(ctx, 0, "D", "Department"); err == nil {
		t.Fatal("CreateDepartment() accepted a missing company")
	}
	if err := svc.CreateCostCenter(ctx, 1, " ", "Cost center", 2); err == nil {
		t.Fatal("CreateCostCenter() accepted a blank code")
	}
	if err := svc.CreateDepartment(ctx, 3, "  SALES ", "  Sales  "); err != nil {
		t.Fatal(err)
	}
	if got := repo.tx.department; got.Code != "SALES" || got.Name != "Sales" || got.CompanyID != 3 {
		t.Fatalf("department input = %#v", got)
	}
	if err := svc.CreateCostCenter(ctx, 3, " CC-1 ", "  Retail ", 7); err != nil {
		t.Fatal(err)
	}
	if got := repo.tx.costCenter; got.Code != "CC-1" || got.Name != "Retail" || got.DepartmentID != 7 {
		t.Fatalf("cost center input = %#v", got)
	}
}
