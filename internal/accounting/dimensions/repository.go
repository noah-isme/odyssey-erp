package dimensions

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type repositoryImpl struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *repositoryImpl {
	return &repositoryImpl{pool: pool}
}

func (r *repositoryImpl) ListDepartments(ctx context.Context, companyID int64) ([]Department, error) {
	rows, err := r.pool.Query(ctx, `SELECT id, code, name, is_active FROM departments WHERE company_id=$1 ORDER BY code`, companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var deps []Department
	for rows.Next() {
		var d Department
		if err := rows.Scan(&d.ID, &d.Code, &d.Name, &d.Active); err != nil {
			return nil, err
		}
		deps = append(deps, d)
	}
	return deps, nil
}

func (r *repositoryImpl) ListCostCenters(ctx context.Context, companyID int64) ([]CostCenter, error) {
	rows, err := r.pool.Query(ctx, `SELECT c.id, c.code, c.name, COALESCE(d.name,''), c.is_active FROM cost_centers c LEFT JOIN departments d ON d.id=c.department_id WHERE c.company_id=$1 ORDER BY c.code`, companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var centers []CostCenter
	for rows.Next() {
		var c CostCenter
		if err := rows.Scan(&c.ID, &c.Code, &c.Name, &c.Department, &c.Active); err != nil {
			return nil, err
		}
		centers = append(centers, c)
	}
	return centers, nil
}

func (r *repositoryImpl) CreateDepartment(ctx context.Context, input CreateDepartmentInput) error {
	_, err := r.pool.Exec(ctx, `INSERT INTO departments (company_id, code, name) VALUES ($1,$2,$3)`, input.CompanyID, input.Code, input.Name)
	return err
}

func (r *repositoryImpl) CreateCostCenter(ctx context.Context, input CreateCostCenterInput) error {
	_, err := r.pool.Exec(ctx, `INSERT INTO cost_centers (company_id, department_id, code, name) VALUES ($1,NULLIF($2,0),$3,$4)`, input.CompanyID, input.DepartmentID, input.Code, input.Name)
	return err
}

func (r *repositoryImpl) WithTx(ctx context.Context, fn func(context.Context, TxRepository) error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	txRepo := &txRepo{tx: tx}
	if err := fn(ctx, txRepo); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

type txRepo struct {
	tx pgx.Tx
}

func (r *txRepo) CreateDepartment(ctx context.Context, input CreateDepartmentInput) error {
	_, err := r.tx.Exec(ctx, `INSERT INTO departments (company_id, code, name) VALUES ($1,$2,$3)`, input.CompanyID, input.Code, input.Name)
	return err
}

func (r *txRepo) CreateCostCenter(ctx context.Context, input CreateCostCenterInput) error {
	_, err := r.tx.Exec(ctx, `INSERT INTO cost_centers (company_id, department_id, code, name) VALUES ($1,NULLIF($2,0),$3,$4)`, input.CompanyID, input.DepartmentID, input.Code, input.Name)
	return err
}