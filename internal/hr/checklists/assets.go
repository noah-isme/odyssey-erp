package checklists

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/odyssey-erp/odyssey-erp/internal/hr/employees"
)

// Asset is a fixed asset in an employee's custody.
type Asset struct {
	ID                  int64
	CompanyID           int64
	Number              string
	Name                string
	Status              string
	CustodianEmployeeID *int64
}

// AssetsHeldBy returns the company's fixed assets whose custodian is the
// employee, by number.
func (s *Service) AssetsHeldBy(ctx context.Context, companyID, employeeID int64) ([]Asset, error) {
	if companyID <= 0 {
		return nil, ErrCompanyRequired
	}
	rows, err := s.pool.Query(ctx, `SELECT fa.id, fa.company_id, fa.number, fa.name, fa.status, fa.custodian_employee_id FROM fixed_assets fa WHERE fa.company_id=$1 AND fa.custodian_employee_id=$2 ORDER BY fa.number`, companyID, employeeID)
	if err != nil {
		return nil, fmt.Errorf("assets held by employee: %w", err)
	}
	defer rows.Close()
	var out []Asset
	for rows.Next() {
		var a Asset
		if err := rows.Scan(&a.ID, &a.CompanyID, &a.Number, &a.Name, &a.Status, &a.CustodianEmployeeID); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func lockAsset(ctx context.Context, tx pgx.Tx, companyID, assetID int64) (Asset, error) {
	a := Asset{ID: assetID, CompanyID: companyID}
	err := tx.QueryRow(ctx, `SELECT number, name, status, custodian_employee_id FROM fixed_assets WHERE id=$1 AND company_id=$2 FOR UPDATE`, assetID, companyID).
		Scan(&a.Number, &a.Name, &a.Status, &a.CustodianEmployeeID)
	if errors.Is(err, pgx.ErrNoRows) {
		return a, ErrAssetNotFound
	}
	if err != nil {
		return a, fmt.Errorf("lock asset: %w", err)
	}
	return a, nil
}

// AssignAsset puts a company asset in an ACTIVE company employee's custody,
// audited ASSET_ASSIGNED. Assigning to the current custodian is a no-op.
func (s *Service) AssignAsset(ctx context.Context, companyID, assetID, employeeID, actorID int64) error {
	if companyID <= 0 {
		return ErrCompanyRequired
	}
	if assetID <= 0 {
		return ErrAssetNotFound
	}
	if employeeID <= 0 {
		return ErrEmployeeNotFound
	}
	return s.inTx(ctx, func(tx pgx.Tx) error {
		a, err := lockAsset(ctx, tx, companyID, assetID)
		if err != nil {
			return err
		}
		if a.Status == "DISPOSED" {
			return ErrAssetDisposed
		}
		if a.CustodianEmployeeID != nil {
			if *a.CustodianEmployeeID == employeeID {
				return nil
			}
			return ErrAssetHeldByOther
		}
		var status string
		err = tx.QueryRow(ctx, `SELECT status FROM hr_employees WHERE id=$1 AND company_id=$2`, employeeID, companyID).Scan(&status)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrEmployeeNotFound
		}
		if err != nil {
			return fmt.Errorf("check employee: %w", err)
		}
		if status != employees.StatusActive {
			return ErrEmployeeInactive
		}
		if _, err := tx.Exec(ctx, `UPDATE fixed_assets SET custodian_employee_id=$1, updated_at=NOW() WHERE id=$2 AND company_id=$3`, employeeID, assetID, companyID); err != nil {
			return fmt.Errorf("assign asset: %w", err)
		}
		return s.record(ctx, tx, actorID, ActionAssetAssigned, entityAsset, assetID, map[string]any{"employee_id": employeeID})
	})
}

// ReturnAsset clears a company asset's custodian, audited ASSET_RETURNED.
func (s *Service) ReturnAsset(ctx context.Context, companyID, assetID, actorID int64) error {
	if companyID <= 0 {
		return ErrCompanyRequired
	}
	if assetID <= 0 {
		return ErrAssetNotFound
	}
	return s.inTx(ctx, func(tx pgx.Tx) error {
		a, err := lockAsset(ctx, tx, companyID, assetID)
		if err != nil {
			return err
		}
		if a.CustodianEmployeeID == nil {
			return ErrAssetNotAssigned
		}
		if _, err := tx.Exec(ctx, `UPDATE fixed_assets SET custodian_employee_id=NULL, updated_at=NOW() WHERE id=$1 AND company_id=$2`, assetID, companyID); err != nil {
			return fmt.Errorf("return asset: %w", err)
		}
		return s.record(ctx, tx, actorID, ActionAssetReturned, entityAsset, assetID, map[string]any{"employee_id": *a.CustodianEmployeeID})
	})
}

// AssignableAssets returns the company's fixed assets that are not disposed
// and have no custodian, by number (the employee detail "Assets" tab picker).
func (s *Service) AssignableAssets(ctx context.Context, companyID int64) ([]Asset, error) {
	if companyID <= 0 {
		return nil, ErrCompanyRequired
	}
	rows, err := s.pool.Query(ctx, `SELECT fa.id, fa.company_id, fa.number, fa.name, fa.status, fa.custodian_employee_id FROM fixed_assets fa WHERE fa.company_id=$1 AND fa.custodian_employee_id IS NULL AND fa.status<>'DISPOSED' ORDER BY fa.number LIMIT 500`, companyID)
	if err != nil {
		return nil, fmt.Errorf("assignable assets: %w", err)
	}
	defer rows.Close()
	var out []Asset
	for rows.Next() {
		var a Asset
		if err := rows.Scan(&a.ID, &a.CompanyID, &a.Number, &a.Name, &a.Status, &a.CustodianEmployeeID); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
