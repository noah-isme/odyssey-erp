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
		SELECT $1,id,$3,$4,$5,$6,$7 FROM warehouses WHERE id=$2 AND company_id=$1
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
	result, err := r.pool.Exec(ctx, `INSERT INTO wms_barcode_aliases(company_id,barcode,product_id,bin_id) SELECT $1,$2,CASE WHEN $3::bigint IS NULL THEN NULL ELSE p.id END,CASE WHEN $4::bigint IS NULL THEN NULL ELSE b.id END FROM (SELECT 1) x LEFT JOIN products p ON p.id=$3 AND p.company_id=$1 LEFT JOIN wms_bins b ON b.id=$4 AND b.company_id=$1 WHERE ($3::bigint IS NULL OR p.id IS NOT NULL) AND ($4::bigint IS NULL OR b.id IS NOT NULL) ON CONFLICT (company_id,barcode) DO UPDATE SET product_id=EXCLUDED.product_id,bin_id=EXCLUDED.bin_id`, companyID, barcode, nilIfZero(productID), nilIfZero(binID))
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *SQLRepository) CreatePickTask(ctx context.Context, task PickTask) (PickTask, error) {
	var out PickTask
	err := r.pool.QueryRow(ctx, `INSERT INTO wms_pick_tasks(company_id,wave_id,product_id,requested_qty,status) SELECT $1,w.id,p.id,$4,'OPEN' FROM wms_pick_waves w JOIN products p ON p.id=$3 AND p.company_id=$1 WHERE w.id=$2 AND w.company_id=$1 RETURNING id,company_id,wave_id,product_id,requested_qty,picked_qty,status`, task.CompanyID, task.WaveID, task.ProductID, task.RequestedQty).Scan(&out.ID, &out.CompanyID, &out.WaveID, &out.ProductID, &out.RequestedQty, &out.PickedQty, &out.Status)
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

// =============================================================================
// Advanced WMS Features (Put-away, Cross-docking, MHE)
// =============================================================================

func (r *SQLRepository) CreatePutAwayTask(ctx context.Context, task PutAwayTask) (PutAwayTask, error) {
	err := r.pool.QueryRow(ctx, `INSERT INTO wms_putaway_tasks (company_id, product_id, bin_id, quantity, status) VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		task.CompanyID, task.ProductID, task.BinID, task.Quantity, task.Status).Scan(&task.ID)
	return task, err
}

func (r *SQLRepository) CreateCrossDockingPlan(ctx context.Context, plan CrossDockingPlan) (CrossDockingPlan, error) {
	err := r.pool.QueryRow(ctx, `INSERT INTO wms_cross_docking_plans (company_id, product_id, source_receipt_id, target_order_id, quantity, status) VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
		plan.CompanyID, plan.ProductID, plan.SourceReceiptID, plan.TargetOrderID, plan.Quantity, plan.Status).Scan(&plan.ID)
	return plan, err
}

func (r *SQLRepository) CreateMHEEquipment(ctx context.Context, equip MHEEquipment) (MHEEquipment, error) {
	err := r.pool.QueryRow(ctx, `INSERT INTO wms_mhe_equipment (company_id, name, type, status) VALUES ($1, $2, $3, $4) RETURNING id`,
		equip.CompanyID, equip.Name, equip.Type, equip.Status).Scan(&equip.ID)
	return equip, err
}

var _ Repository = (*SQLRepository)(nil)

