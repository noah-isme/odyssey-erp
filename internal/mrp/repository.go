package mrp

import (
	"context"
	"errors"
	"fmt"
	"strings"

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
	err = tx.QueryRow(ctx, `INSERT INTO mrp_boms(company_id,product_id,version,effective_from,scrap_pct,active,revision_status,change_reason,created_by) SELECT $1,p.id,$3,$4,$5,$6,'DRAFT',$7,$8 FROM products p WHERE p.id=$2 AND p.company_id=$1 RETURNING id,company_id,product_id,version,effective_from::timestamptz,scrap_pct::float8,active,revision_status,change_reason`, b.CompanyID, b.ProductID, b.Version, b.EffectiveFrom, b.ScrapPct, b.Active, nullIfBlank(b.ChangeReason), b.CreatedBy).Scan(&out.ID, &out.CompanyID, &out.ProductID, &out.Version, &out.EffectiveFrom, &out.ScrapPct, &out.Active, &out.RevisionStatus, &out.ChangeReason)
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

func (r *SQLRepository) CreateBOMRevision(ctx context.Context, companyID, sourceBOMID, actorID int64, in BOMRevisionInput) (BOM, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return BOM{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var out BOM
	err = tx.QueryRow(ctx, `INSERT INTO mrp_boms(company_id,product_id,version,effective_from,scrap_pct,active,revision_status,change_reason,created_by)
		SELECT company_id,product_id,$4,$5,scrap_pct,TRUE,'DRAFT',$6,$3 FROM mrp_boms WHERE id=$2 AND company_id=$1
		RETURNING id,company_id,product_id,version,effective_from::timestamptz,scrap_pct::float8,active,revision_status,change_reason`, companyID, sourceBOMID, actorID, strings.TrimSpace(in.Version), planningDay(in.EffectiveFrom), strings.TrimSpace(in.ChangeReason)).Scan(&out.ID, &out.CompanyID, &out.ProductID, &out.Version, &out.EffectiveFrom, &out.ScrapPct, &out.Active, &out.RevisionStatus, &out.ChangeReason)
	if errors.Is(err, pgx.ErrNoRows) {
		return BOM{}, ErrNotFound
	}
	if err != nil {
		return BOM{}, err
	}
	rows, err := tx.Query(ctx, `INSERT INTO mrp_bom_lines(bom_id,component_product_id,quantity,scrap_pct) SELECT $1,component_product_id,quantity,scrap_pct FROM mrp_bom_lines WHERE bom_id=$2 RETURNING component_product_id,quantity::float8,scrap_pct::float8`, out.ID, sourceBOMID)
	if err != nil {
		return BOM{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var line BOMLine
		if err := rows.Scan(&line.ProductID, &line.Quantity, &line.ScrapPct); err != nil {
			return BOM{}, err
		}
		out.Lines = append(out.Lines, line)
	}
	if err := rows.Err(); err != nil {
		return BOM{}, err
	}
	if len(out.Lines) == 0 {
		return BOM{}, ErrInvalidState
	}
	if err := tx.Commit(ctx); err != nil {
		return BOM{}, err
	}
	return out, nil
}

func (r *SQLRepository) ApproveBOM(ctx context.Context, companyID, bomID, actorID int64, reason string) (BOM, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return BOM{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var productID int64
	var effectiveFrom string
	var status string
	err = tx.QueryRow(ctx, `SELECT product_id,effective_from::text,revision_status FROM mrp_boms WHERE company_id=$1 AND id=$2 FOR UPDATE`, companyID, bomID).Scan(&productID, &effectiveFrom, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return BOM{}, ErrNotFound
	}
	if err != nil {
		return BOM{}, err
	}
	if status != BOMRevisionDraft {
		return BOM{}, ErrInvalidState
	}
	if _, err = tx.Exec(ctx, `SELECT id FROM mrp_boms WHERE company_id=$1 AND product_id=$2 FOR UPDATE`, companyID, productID); err != nil {
		return BOM{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE mrp_boms SET revision_status='SUPERSEDED',active=FALSE,effective_to=LEAST(COALESCE(effective_to, $3::date), $3::date) WHERE company_id=$1 AND product_id=$2 AND revision_status='APPROVED'`, companyID, productID, effectiveFrom); err != nil {
		return BOM{}, err
	}
	var out BOM
	err = tx.QueryRow(ctx, `UPDATE mrp_boms SET revision_status='APPROVED',approved_by=$3,approved_at=NOW(),change_reason=$4,active=TRUE WHERE company_id=$1 AND id=$2 RETURNING id,company_id,product_id,version,effective_from::timestamptz,COALESCE(effective_to::timestamptz,'0001-01-01'::timestamptz),scrap_pct::float8,active,revision_status,approved_by,approved_at,change_reason`, companyID, bomID, actorID, strings.TrimSpace(reason)).Scan(&out.ID, &out.CompanyID, &out.ProductID, &out.Version, &out.EffectiveFrom, &out.EffectiveTo, &out.ScrapPct, &out.Active, &out.RevisionStatus, &out.ApprovedBy, &out.ApprovedAt, &out.ChangeReason)
	if err != nil {
		return BOM{}, err
	}
	if out.EffectiveTo.Year() == 1 {
		out.EffectiveTo = out.EffectiveTo.UTC()
	}
	if err := tx.Commit(ctx); err != nil {
		return BOM{}, err
	}
	return out, nil
}

func (r *SQLRepository) ListBOMRevisions(ctx context.Context, companyID, productID int64) ([]BOM, error) {
	rows, err := r.pool.Query(ctx, `SELECT id,company_id,product_id,version,effective_from::timestamptz,COALESCE(effective_to::timestamptz,'0001-01-01'::timestamptz),scrap_pct::float8,active,revision_status,approved_by,approved_at,COALESCE(change_reason,'') FROM mrp_boms WHERE company_id=$1 AND product_id=$2 ORDER BY effective_from DESC,id DESC`, companyID, productID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []BOM
	for rows.Next() {
		var b BOM
		if err := rows.Scan(&b.ID, &b.CompanyID, &b.ProductID, &b.Version, &b.EffectiveFrom, &b.EffectiveTo, &b.ScrapPct, &b.Active, &b.RevisionStatus, &b.ApprovedBy, &b.ApprovedAt, &b.ChangeReason); err != nil {
			return nil, err
		}
		if b.EffectiveTo.Year() == 1 {
			b.EffectiveTo = b.EffectiveTo.UTC()
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (r *SQLRepository) CreateWorkOrder(ctx context.Context, o WorkOrder) (WorkOrder, error) {
	var out WorkOrder
	err := r.pool.QueryRow(ctx, `INSERT INTO mrp_work_orders(company_id,number,product_id,bom_id,warehouse_id,planned_qty,status,created_by) SELECT $1,'WO-'||to_char(NOW(),'YYYYMMDDHH24MISSMS')||'-'||nextval('mrp_work_orders_id_seq')::text,p.id,b.id,w.id,$5,'DRAFT',$6 FROM products p JOIN mrp_boms b ON b.id=$3 AND b.company_id=$1 AND b.product_id=p.id AND b.active AND b.revision_status='APPROVED' AND b.effective_from <= CURRENT_DATE AND (b.effective_to IS NULL OR b.effective_to >= CURRENT_DATE) JOIN warehouses w ON w.id=$4 JOIN branches branch ON branch.id=w.branch_id AND branch.company_id=$1 WHERE p.id=$2 AND p.company_id=$1 RETURNING id,company_id,product_id,bom_id,warehouse_id,planned_qty,completed_qty,status,created_by`, o.CompanyID, o.ProductID, o.BOMID, o.WarehouseID, o.PlannedQty, o.CreatedBy).Scan(&out.ID, &out.CompanyID, &out.ProductID, &out.BOMID, &out.WarehouseID, &out.PlannedQty, &out.CompletedQty, &out.Status, &out.CreatedBy)
	return out, err
}
func (r *SQLRepository) GetBOM(ctx context.Context, companyID, bomID int64) (BOM, error) {
	var b BOM
	err := r.pool.QueryRow(ctx, `SELECT id,company_id,product_id,version,effective_from::timestamptz,COALESCE(effective_to::timestamptz,'0001-01-01'::timestamptz),active,scrap_pct::float8,revision_status,approved_by,approved_at,COALESCE(change_reason,'') FROM mrp_boms WHERE company_id=$1 AND id=$2`, companyID, bomID).Scan(&b.ID, &b.CompanyID, &b.ProductID, &b.Version, &b.EffectiveFrom, &b.EffectiveTo, &b.Active, &b.ScrapPct, &b.RevisionStatus, &b.ApprovedBy, &b.ApprovedAt, &b.ChangeReason)
	if errors.Is(err, pgx.ErrNoRows) {
		return BOM{}, ErrNotFound
	}
	if err != nil {
		return BOM{}, err
	}
	if b.EffectiveTo.Year() == 1 {
		b.EffectiveTo = b.EffectiveTo.UTC()
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

// GenerateWorkOrderOperations snapshots the active routing at release. The
// work-order copy remains stable even when routing master data changes later.
func (r *SQLRepository) GenerateWorkOrderOperations(ctx context.Context, o WorkOrder) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var routingID int64
	err = tx.QueryRow(ctx, `SELECT id FROM mrp_routings WHERE company_id=$1 AND product_id=$2 AND active ORDER BY id DESC LIMIT 1`, o.CompanyID, o.ProductID).Scan(&routingID)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%w: no active routing for product", ErrInvalidState)
	}
	if err != nil {
		return err
	}
	result, err := tx.Exec(ctx, `INSERT INTO mrp_work_order_operations(company_id,work_order_id,routing_operation_id,work_center_id,sequence,code,name,status,planned_setup_minutes,planned_run_minutes)
		SELECT $1,$2,id,work_center_id,sequence,code,name,CASE WHEN sequence=(SELECT MIN(sequence) FROM mrp_routing_operations WHERE routing_id=$3) THEN 'READY' ELSE 'PENDING' END,setup_minutes,run_minutes
		FROM mrp_routing_operations WHERE routing_id=$3 ORDER BY sequence`, o.CompanyID, o.ID, routingID)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("%w: routing has no operations", ErrInvalidState)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO mrp_operation_dependencies(operation_id,predecessor_operation_id) SELECT current.id,previous.id FROM mrp_work_order_operations current JOIN mrp_work_order_operations previous ON previous.work_order_id=current.work_order_id AND previous.sequence=current.sequence-1 WHERE current.work_order_id=$1`, o.ID); err != nil { return err }
	if _, err = tx.Exec(ctx, `UPDATE mrp_work_orders SET routing_id=$1,updated_at=NOW() WHERE id=$2 AND company_id=$3`, routingID, o.ID, o.CompanyID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *SQLRepository) CreateWIPLocation(ctx context.Context, location WIPLocation) (WIPLocation, error) {
	var out WIPLocation
	err := r.pool.QueryRow(ctx, `INSERT INTO mrp_wip_locations(company_id,warehouse_id,wip_warehouse_id,work_center_id,name,active,created_by)
		SELECT $1,source.id,wip.id,$4,$5,TRUE,$6 FROM warehouses source JOIN branches sb ON sb.id=source.branch_id AND sb.company_id=$1 JOIN warehouses wip ON wip.id=$3 JOIN branches wb ON wb.id=wip.branch_id AND wb.company_id=$1 WHERE source.id=$2
		RETURNING id,company_id,warehouse_id,wip_warehouse_id,work_center_id,name,active,created_by`, location.CompanyID, location.WarehouseID, location.WIPWarehouseID, location.WorkCenterID, strings.TrimSpace(location.Name), location.CreatedBy).Scan(&out.ID, &out.CompanyID, &out.WarehouseID, &out.WIPWarehouseID, &out.WorkCenterID, &out.Name, &out.Active, &out.CreatedBy)
	if errors.Is(err, pgx.ErrNoRows) {
		return WIPLocation{}, ErrNotFound
	}
	return out, err
}
func (r *SQLRepository) ListWIPLocations(ctx context.Context, companyID int64) ([]WIPLocation, error) {
	rows, err := r.pool.Query(ctx, `SELECT id,company_id,warehouse_id,wip_warehouse_id,work_center_id,name,active,created_by FROM mrp_wip_locations WHERE company_id=$1 ORDER BY warehouse_id,work_center_id NULLS FIRST,id`, companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []WIPLocation
	for rows.Next() {
		var location WIPLocation
		if err := rows.Scan(&location.ID, &location.CompanyID, &location.WarehouseID, &location.WIPWarehouseID, &location.WorkCenterID, &location.Name, &location.Active, &location.CreatedBy); err != nil {
			return nil, err
		}
		out = append(out, location)
	}
	return out, rows.Err()
}
func (r *SQLRepository) DeactivateWIPLocation(ctx context.Context, companyID, id int64) error {
	result, err := r.pool.Exec(ctx, `UPDATE mrp_wip_locations SET active=FALSE WHERE company_id=$1 AND id=$2 AND active`, companyID, id)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
func (r *SQLRepository) ResolveWIPLocation(ctx context.Context, companyID, warehouseID, workCenterID int64) (WIPLocation, error) {
	var out WIPLocation
	err := r.pool.QueryRow(ctx, `SELECT id,company_id,warehouse_id,wip_warehouse_id,work_center_id,name,active,created_by FROM mrp_wip_locations WHERE company_id=$1 AND warehouse_id=$2 AND active AND (work_center_id=$3 OR work_center_id IS NULL) ORDER BY (work_center_id IS NOT NULL) DESC LIMIT 1`, companyID, warehouseID, workCenterID).Scan(&out.ID, &out.CompanyID, &out.WarehouseID, &out.WIPWarehouseID, &out.WorkCenterID, &out.Name, &out.Active, &out.CreatedBy)
	if errors.Is(err, pgx.ErrNoRows) {
		return WIPLocation{}, ErrNotFound
	}
	return out, err
}

func (r *SQLRepository) CreateWorkCenter(ctx context.Context, center WorkCenter) (WorkCenter, error) {
	var out WorkCenter
	err := r.pool.QueryRow(ctx, `INSERT INTO mrp_work_centers(company_id,code,name,capacity_hours_per_day,active,created_by) VALUES($1,$2,$3,$4,$5,$6) RETURNING id,company_id,code,name,capacity_hours_per_day::float8,active,created_by`, center.CompanyID, center.Code, center.Name, center.CapacityHoursPerDay, center.Active, center.CreatedBy).Scan(&out.ID, &out.CompanyID, &out.Code, &out.Name, &out.CapacityHoursPerDay, &out.Active, &out.CreatedBy)
	return out, err
}

func (r *SQLRepository) CreateRouting(ctx context.Context, routing Routing) (Routing, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Routing{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var out Routing
	err = tx.QueryRow(ctx, `INSERT INTO mrp_routings(company_id,product_id,code,version,active,created_by) SELECT $1,p.id,$3,$4,$5,$6 FROM products p WHERE p.id=$2 AND p.company_id=$1 RETURNING id,company_id,product_id,code,version,active,created_by`, routing.CompanyID, routing.ProductID, routing.Code, routing.Version, routing.Active, routing.CreatedBy).Scan(&out.ID, &out.CompanyID, &out.ProductID, &out.Code, &out.Version, &out.Active, &out.CreatedBy)
	if err != nil {
		return Routing{}, err
	}
	for _, operation := range routing.Operations {
		var outOperation RoutingOperation
		err = tx.QueryRow(ctx, `INSERT INTO mrp_routing_operations(routing_id,work_center_id,sequence,code,name,setup_minutes,run_minutes,yield_pct) SELECT $1,wc.id,$3,$4,$5,$6,$7,$8 FROM mrp_work_centers wc JOIN mrp_routings r ON r.id=$1 AND r.company_id=$9 WHERE wc.id=$2 AND wc.company_id=$9 AND wc.active RETURNING id,routing_id,work_center_id,sequence,code,name,setup_minutes::float8,run_minutes::float8,yield_pct::float8`, out.ID, operation.WorkCenterID, operation.Sequence, operation.Code, operation.Name, operation.SetupMinutes, operation.RunMinutes, operation.YieldPct, routing.CompanyID).Scan(&outOperation.ID, &outOperation.RoutingID, &outOperation.WorkCenterID, &outOperation.Sequence, &outOperation.Code, &outOperation.Name, &outOperation.SetupMinutes, &outOperation.RunMinutes, &outOperation.YieldPct)
		if err != nil {
			return Routing{}, err
		}
		out.Operations = append(out.Operations, outOperation)
	}
	if err := tx.Commit(ctx); err != nil {
		return Routing{}, err
	}
	return out, nil
}

var _ Repository = (*SQLRepository)(nil)

func nullIfBlank(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return strings.TrimSpace(value)
}
