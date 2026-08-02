package mrp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/odyssey-erp/odyssey-erp/internal/inventory"
)

// CompletionInput identifies one retry-safe finished-goods receipt.
type CompletionInput struct {
	CompanyID                       int64
	ActorID                         int64
	WorkOrderID                     int64
	Quantity                        float64
	IdempotencyKey                  string
	ProducedLotID, ProducedSerialID int64
}

type OperationReportInput struct {
	CompanyID, ActorID, WorkOrderID, OperationID          int64
	SetupMinutes, RunMinutes, GoodQuantity, ScrapQuantity float64
	Complete                                              bool
}

type MaterialMovementInput struct {
	CompanyID, ActorID, WorkOrderID, OperationID, ProductID int64
	Quantity                                                float64
	IdempotencyKey                                          string
	Return                                                  bool
	LotID, SerialID                                         int64
}

// ProductionExecutor owns the transaction that records production and its
// corresponding inventory movements. Keeping this outside the HTTP handler
// makes retries and rollbacks consistent for every caller.
type ProductionExecutor struct {
	pool       *pgxpool.Pool
	stock      *inventory.Service
	accounting ManufacturingAccounting
}
type ManufacturingAccounting interface {
	PostWIPToFinishedGoods(context.Context, int64, float64) error
}

func NewProductionExecutor(pool *pgxpool.Pool, stock *inventory.Service) *ProductionExecutor {
	return &ProductionExecutor{pool: pool, stock: stock}
}
func (e *ProductionExecutor) SetAccounting(accounting ManufacturingAccounting) {
	e.accounting = accounting
}

func (e *ProductionExecutor) Complete(ctx context.Context, input CompletionInput) (WorkOrder, error) {
	if err := validateCompletionInput(input); err != nil {
		return WorkOrder{}, err
	}
	if e == nil || e.pool == nil || e.stock == nil {
		return WorkOrder{}, errors.New("mrp: production inventory service is unavailable")
	}

	tx, err := e.pool.Begin(ctx)
	if err != nil {
		return WorkOrder{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	key := strings.TrimSpace(input.IdempotencyKey)
	var eventID int64
	err = tx.QueryRow(ctx, `
		INSERT INTO mrp_production_events(company_id,work_order_id,event_type,idempotency_key,quantity,created_by)
		VALUES($1,$2,'COMPLETE',$3,$4,$5)
		ON CONFLICT (company_id,work_order_id,event_type,idempotency_key) DO NOTHING
		RETURNING id`, input.CompanyID, input.WorkOrderID, key, input.Quantity, input.ActorID).Scan(&eventID)
	if errors.Is(err, pgx.ErrNoRows) {
		var stored []byte
		if err := tx.QueryRow(ctx, `SELECT response FROM mrp_production_events WHERE company_id=$1 AND work_order_id=$2 AND event_type='COMPLETE' AND idempotency_key=$3`, input.CompanyID, input.WorkOrderID, key).Scan(&stored); err != nil {
			return WorkOrder{}, err
		}
		var completed WorkOrder
		if err := json.Unmarshal(stored, &completed); err != nil || completed.ID == 0 {
			return WorkOrder{}, fmt.Errorf("mrp: invalid stored completion response")
		}
		return completed, nil
	}
	if err != nil {
		return WorkOrder{}, err
	}

	var order WorkOrder
	var bomProductID int64
	var bomScrapPct float64
	err = tx.QueryRow(ctx, `
		SELECT wo.id,wo.company_id,wo.product_id,COALESCE(wo.bom_id,0),COALESCE(wo.warehouse_id,0),
		       wo.planned_qty,wo.completed_qty,wo.status,wo.created_by,
		       b.product_id,b.scrap_pct::float8
		FROM mrp_work_orders wo
		JOIN mrp_boms b ON b.id=wo.bom_id AND b.company_id=wo.company_id
		WHERE wo.id=$1 AND wo.company_id=$2
		FOR UPDATE`, input.WorkOrderID, input.CompanyID).Scan(
		&order.ID, &order.CompanyID, &order.ProductID, &order.BOMID, &order.WarehouseID,
		&order.PlannedQty, &order.CompletedQty, &order.Status, &order.CreatedBy,
		&bomProductID, &bomScrapPct,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return WorkOrder{}, ErrNotFound
	}
	if err != nil {
		return WorkOrder{}, err
	}
	if order.Status != "IN_PROGRESS" || order.WarehouseID == 0 || order.ProductID == 0 || order.BOMID == 0 ||
		bomProductID != order.ProductID || order.CompletedQty+input.Quantity > order.PlannedQty {
		return WorkOrder{}, ErrInvalidState
	}
	if held, err := qualityHoldOpen(ctx, tx, input.CompanyID, order.ID, 0); err != nil {
		return WorkOrder{}, err
	} else if held {
		return WorkOrder{}, fmt.Errorf("%w: quality hold is open", ErrInvalidState)
	}
	var wipWarehouseID int64
	err = tx.QueryRow(ctx, `SELECT location.wip_warehouse_id FROM mrp_wip_locations location
		WHERE location.company_id=$1 AND location.warehouse_id=$2 AND location.active
		AND (location.work_center_id=(SELECT work_center_id FROM mrp_work_order_operations WHERE work_order_id=$3 ORDER BY sequence LIMIT 1) OR location.work_center_id IS NULL)
		ORDER BY (location.work_center_id IS NOT NULL) DESC LIMIT 1 FOR UPDATE`, order.CompanyID, order.WarehouseID, order.ID).Scan(&wipWarehouseID)
	if errors.Is(err, pgx.ErrNoRows) {
		return WorkOrder{}, fmt.Errorf("%w: no WIP location", ErrInvalidState)
	}
	if err != nil {
		return WorkOrder{}, err
	}

	rows, err := tx.Query(ctx, `SELECT component_product_id,quantity::float8,scrap_pct::float8 FROM mrp_bom_lines WHERE bom_id=$1 ORDER BY id`, order.BOMID)
	if err != nil {
		return WorkOrder{}, err
	}
	type materialLine struct {
		componentID int64
		perUnit     float64
		scrapPct    float64
	}
	lines := make([]materialLine, 0)
	for rows.Next() {
		var line materialLine
		if err := rows.Scan(&line.componentID, &line.perUnit, &line.scrapPct); err != nil {
			rows.Close()
			return WorkOrder{}, err
		}
		lines = append(lines, line)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return WorkOrder{}, err
	}
	rows.Close()
	if len(lines) == 0 {
		return WorkOrder{}, ErrInvalidState
	}

	materialCost := 0.0
	for _, line := range lines {
		quantity := materialQuantity(input.Quantity, line.perUnit, bomScrapPct, line.scrapPct)
		if quantity <= 0 {
			return WorkOrder{}, ErrInvalidState
		}
		var issuedNet float64
		if err := tx.QueryRow(ctx, `SELECT COALESCE(SUM(CASE WHEN movement_type='ISSUE' THEN quantity ELSE -quantity END),0)::float8 FROM mrp_material_movements WHERE company_id=$1 AND work_order_id=$2 AND product_id=$3`, input.CompanyID, order.ID, line.componentID).Scan(&issuedNet); err != nil {
			return WorkOrder{}, err
		}
		var previouslyConsumed float64
		if err := tx.QueryRow(ctx, `SELECT COALESCE(SUM(quantity),0)::float8 FROM mrp_production_receipt_costs WHERE company_id=$1 AND component_product_id=$2 AND production_event_id IN (SELECT id FROM mrp_production_events WHERE company_id=$1 AND work_order_id=$3 AND event_type='COMPLETE')`, input.CompanyID, line.componentID, order.ID).Scan(&previouslyConsumed); err != nil {
			return WorkOrder{}, err
		}
		if issuedNet-previouslyConsumed+1e-9 < quantity {
			return WorkOrder{}, fmt.Errorf("%w: insufficient issued WIP for component %d", ErrInvalidState, line.componentID)
		}
		entry, err := e.stock.PostAdjustmentTx(ctx, tx, inventory.AdjustmentInput{
			Code:            fmt.Sprintf("MRP-WIP-CONSUME-%d-%d-%d", order.ID, eventID, line.componentID),
			WarehouseID:     wipWarehouseID,
			ProductID:       line.componentID,
			Qty:             -quantity,
			Note:            "MRP WIP material consumption",
			ActorID:         input.ActorID,
			RefModule:       "MRP",
			SkipIntegration: true,
		})
		if err != nil {
			return WorkOrder{}, err
		}
		materialCost += entry.UnitCost * quantity
		if _, err = tx.Exec(ctx, `INSERT INTO mrp_production_receipt_costs(company_id,production_event_id,component_product_id,quantity,unit_cost,extended_amount) VALUES($1,$2,$3,$4,$5,$6)`, input.CompanyID, eventID, line.componentID, quantity, entry.UnitCost, entry.UnitCost*quantity); err != nil {
			return WorkOrder{}, err
		}
		if input.ProducedLotID > 0 || input.ProducedSerialID > 0 {
			if _, err = tx.Exec(ctx, `INSERT INTO mrp_genealogy(company_id,work_order_id,component_product_id,produced_lot_id,produced_serial_id,quantity) VALUES($1,$2,$3,NULLIF($4,0),NULLIF($5,0),$6)`, input.CompanyID, order.ID, line.componentID, input.ProducedLotID, input.ProducedSerialID, input.Quantity); err != nil {
				return WorkOrder{}, err
			}
		}
	}

	if _, err := e.stock.PostAdjustmentTx(ctx, tx, inventory.AdjustmentInput{
		Code:            fmt.Sprintf("MRP-RECEIPT-%d-%d", order.ID, eventID),
		WarehouseID:     order.WarehouseID,
		ProductID:       order.ProductID,
		Qty:             input.Quantity,
		UnitCost:        materialCost / input.Quantity,
		Note:            "MRP finished goods receipt",
		ActorID:         input.ActorID,
		RefModule:       "MRP",
		SkipIntegration: true,
	}); err != nil {
		return WorkOrder{}, err
	}

	order.CompletedQty += input.Quantity
	if order.CompletedQty == order.PlannedQty {
		order.Status = "COMPLETED"
	}
	if _, err := tx.Exec(ctx, `UPDATE mrp_work_orders SET completed_qty=$1,status=$2,updated_at=NOW() WHERE id=$3 AND company_id=$4`, order.CompletedQty, order.Status, order.ID, order.CompanyID); err != nil {
		return WorkOrder{}, err
	}
	response, err := json.Marshal(order)
	if err != nil {
		return WorkOrder{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE mrp_production_events SET response=$1::jsonb WHERE id=$2`, string(response), eventID); err != nil {
		return WorkOrder{}, err
	}
	if e.accounting != nil {
		if err := e.accounting.PostWIPToFinishedGoods(ctx, eventID, materialCost); err != nil {
			return WorkOrder{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return WorkOrder{}, err
	}
	return order, nil
}

func materialQuantity(completedQty, quantityPerUnit, bomScrapPct, lineScrapPct float64) float64 {
	return completedQty * quantityPerUnit * (1 + bomScrapPct/100) * (1 + lineScrapPct/100)
}

func (e *ProductionExecutor) ReportOperation(ctx context.Context, in OperationReportInput) (WorkOrderOperation, error) {
	if e == nil || e.pool == nil || in.CompanyID <= 0 || in.ActorID <= 0 || in.WorkOrderID <= 0 || in.OperationID <= 0 || in.SetupMinutes < 0 || in.RunMinutes < 0 || in.GoodQuantity < 0 || in.ScrapQuantity < 0 || in.SetupMinutes+in.RunMinutes+in.GoodQuantity+in.ScrapQuantity == 0 {
		return WorkOrderOperation{}, ErrInvalidState
	}
	tx, err := e.pool.Begin(ctx)
	if err != nil {
		return WorkOrderOperation{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if held, err := qualityHoldOpen(ctx, tx, in.CompanyID, in.WorkOrderID, in.OperationID); err != nil {
		return WorkOrderOperation{}, err
	} else if held {
		return WorkOrderOperation{}, fmt.Errorf("%w: quality hold is open", ErrInvalidState)
	}
	var out WorkOrderOperation
	err = tx.QueryRow(ctx, `UPDATE mrp_work_order_operations op SET status=CASE WHEN $5 THEN 'COMPLETED' ELSE 'IN_PROGRESS' END,actual_setup_minutes=actual_setup_minutes+$6,actual_run_minutes=actual_run_minutes+$7,good_quantity=good_quantity+$8,scrap_quantity=scrap_quantity+$9,operator_id=$4,started_at=COALESCE(started_at,NOW()),completed_at=CASE WHEN $5 THEN NOW() ELSE completed_at END,updated_at=NOW() FROM mrp_work_orders wo WHERE op.id=$1 AND op.work_order_id=$2 AND op.company_id=$3 AND wo.id=op.work_order_id AND wo.status IN ('RELEASED','IN_PROGRESS') AND op.status IN ('READY','IN_PROGRESS') RETURNING op.id,op.company_id,op.work_order_id,COALESCE(op.routing_operation_id,0),op.work_center_id,op.sequence,op.code,op.name,op.status,op.planned_setup_minutes::float8,op.planned_run_minutes::float8,op.actual_setup_minutes::float8,op.actual_run_minutes::float8,op.good_quantity::float8,op.scrap_quantity::float8,op.operator_id`, in.OperationID, in.WorkOrderID, in.CompanyID, in.ActorID, in.Complete, in.SetupMinutes, in.RunMinutes, in.GoodQuantity, in.ScrapQuantity).Scan(&out.ID, &out.CompanyID, &out.WorkOrderID, &out.RoutingOperationID, &out.WorkCenterID, &out.Sequence, &out.Code, &out.Name, &out.Status, &out.PlannedSetupMinutes, &out.PlannedRunMinutes, &out.ActualSetupMinutes, &out.ActualRunMinutes, &out.GoodQuantity, &out.ScrapQuantity, &out.OperatorID)
	if errors.Is(err, pgx.ErrNoRows) {
		return WorkOrderOperation{}, ErrInvalidState
	}
	if err != nil {
		return WorkOrderOperation{}, err
	}
	if out.Status == "COMPLETED" {
		_, err = tx.Exec(ctx, `UPDATE mrp_work_order_operations SET status='READY',updated_at=NOW() WHERE work_order_id=$1 AND status='PENDING' AND sequence=(SELECT MIN(sequence) FROM mrp_work_order_operations WHERE work_order_id=$1 AND status='PENDING')`, in.WorkOrderID)
		if err != nil {
			return WorkOrderOperation{}, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return WorkOrderOperation{}, err
	}
	return out, nil
}

func (e *ProductionExecutor) MoveMaterial(ctx context.Context, in MaterialMovementInput) error {
	if e == nil || e.pool == nil || e.stock == nil || in.CompanyID <= 0 || in.ActorID <= 0 || in.WorkOrderID <= 0 || in.ProductID <= 0 || in.Quantity <= 0 || strings.TrimSpace(in.IdempotencyKey) == "" {
		return ErrInvalidState
	}
	tx, err := e.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var source, destination int64
	err = tx.QueryRow(ctx, `SELECT CASE WHEN $5 THEN location.wip_warehouse_id ELSE wo.warehouse_id END,CASE WHEN $5 THEN wo.warehouse_id ELSE location.wip_warehouse_id END FROM mrp_work_orders wo JOIN mrp_wip_locations location ON location.company_id=wo.company_id AND location.warehouse_id=wo.warehouse_id AND location.active WHERE wo.id=$1 AND wo.company_id=$2 AND wo.status IN ('RELEASED','IN_PROGRESS') AND EXISTS (SELECT 1 FROM mrp_bom_lines line WHERE line.bom_id=wo.bom_id AND line.component_product_id=$4) AND ($3=0 OR EXISTS (SELECT 1 FROM mrp_work_order_operations op WHERE op.id=$3 AND op.work_order_id=wo.id)) AND (location.work_center_id=(SELECT work_center_id FROM mrp_work_order_operations WHERE id=$3 AND work_order_id=wo.id) OR location.work_center_id IS NULL) ORDER BY (location.work_center_id IS NOT NULL) DESC LIMIT 1`, in.WorkOrderID, in.CompanyID, in.OperationID, in.ProductID, in.Return).Scan(&source, &destination)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrInvalidState
	}
	if err != nil {
		return err
	}
	typ := "ISSUE"
	if in.Return {
		typ = "RETURN"
	}
	var movementID int64
	err = tx.QueryRow(ctx, `INSERT INTO mrp_material_movements(company_id,work_order_id,operation_id,product_id,source_warehouse_id,destination_warehouse_id,quantity,movement_type,idempotency_key,created_by) VALUES($1,$2,NULLIF($3,0),$4,$5,$6,$7,$8,$9,$10) ON CONFLICT (company_id,work_order_id,movement_type,idempotency_key) DO NOTHING RETURNING id`, in.CompanyID, in.WorkOrderID, in.OperationID, in.ProductID, source, destination, in.Quantity, typ, strings.TrimSpace(in.IdempotencyKey), in.ActorID).Scan(&movementID)
	if errors.Is(err, pgx.ErrNoRows) {
		return tx.Commit(ctx)
	}
	if err != nil {
		return err
	}
	_, _, err = e.stock.PostTransferTx(ctx, tx, inventory.TransferInput{Code: fmt.Sprintf("MRP-%s-%d", typ, movementID), ProductID: in.ProductID, Qty: in.Quantity, SrcWarehouse: source, DstWarehouse: destination, Note: "MRP material " + strings.ToLower(typ), ActorID: in.ActorID, RefModule: "MRP", RefID: fmt.Sprintf("%d", in.WorkOrderID)})
	if err != nil {
		return err
	}
	if !in.Return && (in.LotID > 0 || in.SerialID > 0) {
		if _, err = tx.Exec(ctx, `INSERT INTO mrp_genealogy(company_id,work_order_id,operation_id,component_product_id,consumed_lot_id,consumed_serial_id,quantity) VALUES($1,$2,NULLIF($3,0),$4,NULLIF($5,0),NULLIF($6,0),$7)`, in.CompanyID, in.WorkOrderID, in.OperationID, in.ProductID, in.LotID, in.SerialID, in.Quantity); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func qualityHoldOpen(ctx context.Context, tx pgx.Tx, companyID, workOrderID, operationID int64) (bool, error) {
	var held bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM mrp_quality_holds WHERE company_id=$1 AND work_order_id=$2 AND status='OPEN' AND ($3=0 OR operation_id IS NULL OR operation_id=$3))`, companyID, workOrderID, operationID).Scan(&held)
	return held, err
}
