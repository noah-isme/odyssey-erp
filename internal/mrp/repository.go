package mrp

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type SQLRepository struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) *SQLRepository { return &SQLRepository{pool: pool} }

func (r *SQLRepository) CreateBOM(ctx context.Context, b BOM) (BOM, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return BOM{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var out BOM
	err = tx.QueryRow(ctx, `INSERT INTO mrp_boms(company_id,product_id,version,effective_from,active,created_by) SELECT $1,p.id,$3,CURRENT_DATE,$4,$5 FROM products p WHERE p.id=$2 AND p.company_id=$1 RETURNING id,company_id,product_id,version,active`, b.CompanyID, b.ProductID, b.Version, b.Active, b.CreatedBy).Scan(&out.ID, &out.CompanyID, &out.ProductID, &out.Version, &out.Active)
	if err != nil {
		return BOM{}, err
	}
	for _, line := range b.Lines {
		result, execErr := tx.Exec(ctx, `INSERT INTO mrp_bom_lines(bom_id,component_product_id,quantity,scrap_pct) SELECT $1,p.id,$3,$4 FROM products p JOIN mrp_boms b ON b.id=$1 AND b.company_id=$5 WHERE p.id=$2 AND p.company_id=$5`, out.ID, line.ProductID, line.Quantity, line.ScrapPct, b.CompanyID)
		if execErr != nil {
			err = execErr
			return BOM{}, err
		}
		if result.RowsAffected() == 0 {
			return BOM{}, errors.New("mrp: component product is outside company")
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return BOM{}, err
	}
	out.Lines = b.Lines
	return out, nil
}

func (r *SQLRepository) CreateWorkOrder(ctx context.Context, o WorkOrder) (WorkOrder, error) {
	var out WorkOrder
	err := r.pool.QueryRow(ctx, `INSERT INTO mrp_work_orders(company_id,number,product_id,bom_id,warehouse_id,planned_qty,status,created_by) SELECT $1,'WO-'||to_char(NOW(),'YYYYMMDDHH24MISSMS')||'-'||nextval('mrp_work_orders_id_seq')::text,p.id,b.id,w.id,$5,'DRAFT',$6 FROM products p JOIN mrp_boms b ON b.id=$3 AND b.company_id=$1 JOIN warehouses w ON w.id=$4 AND w.company_id=$1 WHERE p.id=$2 AND p.company_id=$1 RETURNING id,company_id,product_id,bom_id,warehouse_id,planned_qty,completed_qty,status,created_by`, o.CompanyID, o.ProductID, o.BOMID, o.WarehouseID, o.PlannedQty, o.CreatedBy).Scan(&out.ID, &out.CompanyID, &out.ProductID, &out.BOMID, &out.WarehouseID, &out.PlannedQty, &out.CompletedQty, &out.Status, &out.CreatedBy)
	return out, err
}
func (r *SQLRepository) GetBOM(ctx context.Context, companyID, bomID int64) (BOM, error) {
	var b BOM
	err := r.pool.QueryRow(ctx, `SELECT id,company_id,product_id,version,active FROM mrp_boms WHERE company_id=$1 AND id=$2`, companyID, bomID).Scan(&b.ID, &b.CompanyID, &b.ProductID, &b.Version, &b.Active)
	if errors.Is(err, pgx.ErrNoRows) {
		return BOM{}, ErrNotFound
	}
	if err != nil {
		return BOM{}, err
	}
	rows, err := r.pool.Query(ctx, `SELECT component_product_id,quantity,scrap_pct FROM mrp_bom_lines WHERE bom_id=$1 ORDER BY id`, bomID)
	if err != nil {
		return BOM{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var l BOMLine
		if err := rows.Scan(&l.ProductID, &l.Quantity, &l.ScrapPct); err != nil {
			return BOM{}, err
		}
		b.Lines = append(b.Lines, l)
	}
	return b, rows.Err()
}
func (r *SQLRepository) GetWorkOrder(ctx context.Context, companyID, orderID int64) (WorkOrder, error) {
	var o WorkOrder
	err := r.pool.QueryRow(ctx, `SELECT id,company_id,product_id,COALESCE(bom_id,0),COALESCE(warehouse_id,0),planned_qty,completed_qty,status,created_by FROM mrp_work_orders WHERE company_id=$1 AND id=$2`, companyID, orderID).Scan(&o.ID, &o.CompanyID, &o.ProductID, &o.BOMID, &o.WarehouseID, &o.PlannedQty, &o.CompletedQty, &o.Status, &o.CreatedBy)
	if errors.Is(err, pgx.ErrNoRows) {
		return WorkOrder{}, ErrNotFound
	}
	return o, err
}
func (r *SQLRepository) UpdateWorkOrder(ctx context.Context, o WorkOrder) error {
	res, err := r.pool.Exec(ctx, `UPDATE mrp_work_orders SET completed_qty=$1,status=$2,updated_at=NOW() WHERE company_id=$3 AND id=$4`, o.CompletedQty, o.Status, o.CompanyID, o.ID)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

var _ Repository = (*SQLRepository)(nil)
