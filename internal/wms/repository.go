package wms

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type SQLRepository struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) *SQLRepository { return &SQLRepository{pool: pool} }

func (r *SQLRepository) CreateBin(ctx context.Context, bin Bin) (Bin, error) {
	if r == nil || r.pool == nil {
		return Bin{}, errors.New("wms: database is required")
	}
	var out Bin
	err := r.pool.QueryRow(ctx, `
		INSERT INTO wms_bins(company_id, warehouse_id, code, name, capacity, active, created_by)
		VALUES($1,$2,$3,$4,$5,$6,$7)
		RETURNING id, company_id, warehouse_id, code, name, capacity, active`,
		bin.CompanyID, bin.WarehouseID, bin.Code, bin.Name, bin.Capacity, bin.Active, nilIfZero(bin.ID)).Scan(
		&out.ID, &out.CompanyID, &out.WarehouseID, &out.Code, &out.Name, &out.Capacity, &out.Active)
	return out, err
}

func nilIfZero(id int64) any {
	if id == 0 {
		return nil
	}
	return id
}

func (r *SQLRepository) CreateBarcode(ctx context.Context, companyID int64, barcode string, productID, binID int64) error {
	_, err := r.pool.Exec(ctx, `INSERT INTO wms_barcode_aliases(company_id,barcode,product_id,bin_id) VALUES($1,$2,$3,$4) ON CONFLICT (company_id,barcode) DO UPDATE SET product_id=EXCLUDED.product_id,bin_id=EXCLUDED.bin_id`, companyID, barcode, nilIfZero(productID), nilIfZero(binID))
	return err
}

func (r *SQLRepository) CreatePickTask(ctx context.Context, task PickTask) (PickTask, error) {
	var out PickTask
	err := r.pool.QueryRow(ctx, `INSERT INTO wms_pick_tasks(company_id,wave_id,product_id,requested_qty,status) VALUES($1,$2,$3,$4,'OPEN') RETURNING id,company_id,wave_id,product_id,requested_qty,picked_qty,status`, task.CompanyID, task.WaveID, task.ProductID, task.RequestedQty).Scan(&out.ID, &out.CompanyID, &out.WaveID, &out.ProductID, &out.RequestedQty, &out.PickedQty, &out.Status)
	return out, err
}

func (r *SQLRepository) ResolveBarcode(ctx context.Context, companyID int64, barcode string) (BarcodeTarget, error) {
	var target BarcodeTarget
	err := r.pool.QueryRow(ctx, `SELECT COALESCE(product_id,0), COALESCE(bin_id,0) FROM wms_barcode_aliases WHERE company_id=$1 AND barcode=$2`, companyID, barcode).Scan(&target.ProductID, &target.BinID)
	if errors.Is(err, pgx.ErrNoRows) {
		return BarcodeTarget{}, ErrNotFound
	}
	return target, err
}

func (r *SQLRepository) GetPickTask(ctx context.Context, companyID, taskID int64) (PickTask, error) {
	var task PickTask
	err := r.pool.QueryRow(ctx, `SELECT id, company_id, wave_id, product_id, requested_qty, picked_qty, status FROM wms_pick_tasks WHERE company_id=$1 AND id=$2`, companyID, taskID).Scan(&task.ID, &task.CompanyID, &task.WaveID, &task.ProductID, &task.RequestedQty, &task.PickedQty, &task.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		return PickTask{}, ErrNotFound
	}
	return task, err
}

func (r *SQLRepository) HasScan(ctx context.Context, companyID, taskID int64, key string) (bool, error) {
	var found bool
	err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM wms_pick_scans WHERE company_id=$1 AND task_id=$2 AND idempotency_key=$3)`, companyID, taskID, key).Scan(&found)
	return found, err
}

func (r *SQLRepository) RecordScan(ctx context.Context, companyID, taskID int64, barcode string, quantity float64, actorID int64, key string) (int64, bool, error) {
	var id int64
	err := r.pool.QueryRow(ctx, `INSERT INTO wms_pick_scans(company_id,task_id,barcode,quantity,actor_id,idempotency_key) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT (company_id,task_id,idempotency_key) DO NOTHING RETURNING id`, companyID, taskID, barcode, quantity, actorID, key).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		err = r.pool.QueryRow(ctx, `SELECT id FROM wms_pick_scans WHERE company_id=$1 AND task_id=$2 AND idempotency_key=$3`, companyID, taskID, key).Scan(&id)
		return id, err == nil, err
	}
	return id, false, err
}

func (r *SQLRepository) UpdatePickTask(ctx context.Context, task PickTask) error {
	if task.PickedQty > task.RequestedQty {
		return fmt.Errorf("%w: picked quantity exceeds requested quantity", ErrInvalidQuantity)
	}
	result, err := r.pool.Exec(ctx, `UPDATE wms_pick_tasks SET picked_qty=$1,status=$2,updated_at=NOW() WHERE company_id=$3 AND id=$4`, task.PickedQty, task.Status, task.CompanyID, task.ID)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

var _ Repository = (*SQLRepository)(nil)
