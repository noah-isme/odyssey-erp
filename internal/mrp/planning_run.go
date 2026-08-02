package mrp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PlanningPolicyInput struct {
	ProductID   int64             `json:"product_id"`
	WarehouseID int64             `json:"warehouse_id"`
	OrderType   PlanningOrderType `json:"order_type"`
	LeadDays    int               `json:"lead_days"`
	SafetyStock float64           `json:"safety_stock"`
	LotSizing   LotSizingRule     `json:"lot_sizing"`
	LotQuantity float64           `json:"lot_quantity"`
}

type PlanningRun struct {
	ID              int64
	CompanyID       int64
	AsOfDate        time.Time
	Status          string
	CreatedBy       int64
	CreatedAt       time.Time
	Recommendations []PlanningRecommendation
}

// FirmedRecommendation is the durable result of converting a planning
// recommendation into an executable purchasing or production document.
// Repeating the same request returns these links without creating duplicates.
type FirmedRecommendation struct {
	RecommendationID  int64  `json:"recommendation_id"`
	Status            string `json:"status"`
	WorkOrderID       *int64 `json:"work_order_id,omitempty"`
	PurchaseRequestID *int64 `json:"purchase_request_id,omitempty"`
}

// PlanningRunService snapshots operational demand and supply before using the
// pure netting engine. The snapshot makes each recommendation explainable and
// reproducible after inventory or sales data later changes.
type PlanningRunService struct{ pool *pgxpool.Pool }

func NewPlanningRunService(pool *pgxpool.Pool) *PlanningRunService {
	return &PlanningRunService{pool: pool}
}

func (s *PlanningRunService) CreatePolicy(ctx context.Context, companyID, actorID int64, input PlanningPolicyInput) (PlanningPolicy, error) {
	policy := PlanningPolicy{
		PlanningKey: PlanningKey{ProductID: input.ProductID, WarehouseID: input.WarehouseID},
		OrderType:   input.OrderType, LeadDays: input.LeadDays, SafetyStock: input.SafetyStock,
		LotSizing: input.LotSizing, LotQuantity: input.LotQuantity,
	}
	if s == nil || s.pool == nil || companyID <= 0 || actorID <= 0 || !validPlanningPolicy(policy) {
		return PlanningPolicy{}, ErrInvalidPlanningInput
	}

	var out PlanningPolicy
	err := s.pool.QueryRow(ctx, `
		INSERT INTO mrp_product_planning_policies(company_id,product_id,warehouse_id,order_type,lead_days,safety_stock,lot_sizing,lot_quantity,created_by)
		SELECT $1,p.id,w.id,$4,$5,$6,$7,$8,$9
		FROM products p
		JOIN warehouses w ON w.id=$3
		JOIN branches b ON b.id=w.branch_id AND b.company_id=$1
		WHERE p.id=$2 AND p.company_id=$1
		ON CONFLICT (company_id,product_id,warehouse_id) DO UPDATE
		SET order_type=EXCLUDED.order_type,lead_days=EXCLUDED.lead_days,safety_stock=EXCLUDED.safety_stock,
		    lot_sizing=EXCLUDED.lot_sizing,lot_quantity=EXCLUDED.lot_quantity,active=TRUE,updated_at=NOW()
		RETURNING product_id,warehouse_id,order_type,lead_days,safety_stock::float8,lot_sizing,lot_quantity::float8`,
		companyID, input.ProductID, input.WarehouseID, input.OrderType, input.LeadDays, input.SafetyStock, input.LotSizing, input.LotQuantity, actorID,
	).Scan(&out.ProductID, &out.WarehouseID, &out.OrderType, &out.LeadDays, &out.SafetyStock, &out.LotSizing, &out.LotQuantity)
	if errors.Is(err, pgx.ErrNoRows) {
		return PlanningPolicy{}, ErrNotFound
	}
	return out, err
}

func (s *PlanningRunService) Run(ctx context.Context, companyID, actorID int64, asOf time.Time) (PlanningRun, error) {
	if s == nil || s.pool == nil || companyID <= 0 || actorID <= 0 {
		return PlanningRun{}, ErrInvalidPlanningInput
	}
	asOf = planningDay(asOf)
	if asOf.IsZero() {
		return PlanningRun{}, ErrInvalidPlanningInput
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return PlanningRun{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	input, err := loadPlanningInput(ctx, tx, companyID, asOf)
	if err != nil {
		return PlanningRun{}, err
	}
	boms, err := loadPlanningBOMs(ctx, tx, companyID, asOf)
	if err != nil {
		return PlanningRun{}, err
	}
	input, err = ExplodeBOMDemand(input, boms)
	if err != nil {
		return PlanningRun{}, err
	}
	recommendations, err := Plan(input)
	if err != nil {
		return PlanningRun{}, err
	}
	snapshot, err := json.Marshal(input)
	if err != nil {
		return PlanningRun{}, err
	}

	var run PlanningRun
	err = tx.QueryRow(ctx, `INSERT INTO mrp_planning_runs(company_id,as_of_date,status,input_snapshot,created_by) VALUES($1,$2,'COMPLETED',$3::jsonb,$4) RETURNING id,company_id,as_of_date::timestamptz,status,created_by,created_at`, companyID, asOf, string(snapshot), actorID).Scan(&run.ID, &run.CompanyID, &run.AsOfDate, &run.Status, &run.CreatedBy, &run.CreatedAt)
	if err != nil {
		return PlanningRun{}, err
	}
	for _, recommendation := range recommendations {
		if _, err := tx.Exec(ctx, `INSERT INTO mrp_planning_recommendations(run_id,product_id,warehouse_id,order_type,quantity,release_date,due_date,demand_source_ref,late) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, run.ID, recommendation.ProductID, recommendation.WarehouseID, recommendation.OrderType, recommendation.Quantity, recommendation.ReleaseDate, recommendation.DueDate, recommendation.DemandSourceRef, recommendation.Late); err != nil {
			return PlanningRun{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return PlanningRun{}, err
	}
	for _, recommendation := range recommendations {
		if err := NewExceptionService(s.pool).Upsert(ctx, PlanningException(companyID, nil, recommendation)); err != nil {
			return PlanningRun{}, err
		}
	}
	run.Recommendations = recommendations
	return run, nil
}

// FirmRecommendation converts one planned recommendation to a draft work
// order (MAKE) or purchase request (BUY). Locking the recommendation makes
// the conversion safe to retry and prevents duplicate downstream documents.
func (s *PlanningRunService) FirmRecommendation(ctx context.Context, companyID, actorID, recommendationID int64) (FirmedRecommendation, error) {
	if s == nil || s.pool == nil || companyID <= 0 || actorID <= 0 || recommendationID <= 0 {
		return FirmedRecommendation{}, ErrInvalidPlanningInput
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return FirmedRecommendation{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var recommendation struct {
		ProductID, WarehouseID int64
		OrderType              PlanningOrderType
		Quantity               float64
		ReleaseDate, DueDate   time.Time
		Status                 string
		WorkOrderID, PRID      *int64
	}
	err = tx.QueryRow(ctx, `
		SELECT recommendation.product_id,recommendation.warehouse_id,recommendation.order_type,
		       recommendation.quantity::float8,recommendation.release_date::timestamptz,
		       recommendation.due_date::timestamptz,recommendation.status,
		       recommendation.firmed_work_order_id,recommendation.firmed_pr_id
		FROM mrp_planning_recommendations recommendation
		JOIN mrp_planning_runs run ON run.id=recommendation.run_id
		WHERE recommendation.id=$1 AND run.company_id=$2
		FOR UPDATE`, recommendationID, companyID,
	).Scan(
		&recommendation.ProductID, &recommendation.WarehouseID, &recommendation.OrderType,
		&recommendation.Quantity, &recommendation.ReleaseDate, &recommendation.DueDate, &recommendation.Status,
		&recommendation.WorkOrderID, &recommendation.PRID,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return FirmedRecommendation{}, ErrNotFound
	}
	if err != nil {
		return FirmedRecommendation{}, err
	}
	out := FirmedRecommendation{
		RecommendationID:  recommendationID,
		Status:            recommendation.Status,
		WorkOrderID:       recommendation.WorkOrderID,
		PurchaseRequestID: recommendation.PRID,
	}
	if recommendation.Status == "FIRMED" {
		if err := tx.Commit(ctx); err != nil {
			return FirmedRecommendation{}, err
		}
		return out, nil
	}
	if recommendation.Status != "PLANNED" || recommendation.Quantity <= 0 {
		return FirmedRecommendation{}, ErrInvalidState
	}

	switch recommendation.OrderType {
	case PlanningOrderMake:
		var workOrderID int64
		err = tx.QueryRow(ctx, `
			WITH effective_bom AS (
				SELECT id
				FROM mrp_boms
				WHERE company_id=$1 AND product_id=$2 AND active AND revision_status='APPROVED'
				  AND effective_from <= $3 AND (effective_to IS NULL OR effective_to >= $3)
				ORDER BY effective_from DESC,id DESC
				LIMIT 1
			)
			INSERT INTO mrp_work_orders(
				company_id,number,product_id,bom_id,warehouse_id,planned_qty,status,created_by,
				planned_start_date,planned_due_date,planning_recommendation_id
			)
			SELECT $1,'WO-MRP-'||to_char(NOW(),'YYYYMMDDHH24MISSMS')||'-'||nextval('mrp_work_orders_id_seq')::text,
			       product.id,bom.id,warehouse.id,$5,'DRAFT',$6,$3,$4,$7
			FROM products product
			JOIN effective_bom bom ON TRUE
			JOIN warehouses warehouse ON warehouse.id=$8
			JOIN branches branch ON branch.id=warehouse.branch_id AND branch.company_id=$1
			WHERE product.id=$2 AND product.company_id=$1
			RETURNING id`,
			companyID, recommendation.ProductID, recommendation.ReleaseDate, recommendation.DueDate,
			recommendation.Quantity, actorID, recommendationID, recommendation.WarehouseID,
		).Scan(&workOrderID)
		if errors.Is(err, pgx.ErrNoRows) {
			return FirmedRecommendation{}, fmt.Errorf("%w: no active effective BOM or warehouse for MAKE recommendation", ErrInvalidState)
		}
		if err != nil {
			return FirmedRecommendation{}, err
		}
		if _, err = tx.Exec(ctx, `UPDATE mrp_planning_recommendations SET status='FIRMED',firmed_work_order_id=$1 WHERE id=$2`, workOrderID, recommendationID); err != nil {
			return FirmedRecommendation{}, err
		}
		out.Status = "FIRMED"
		out.WorkOrderID = &workOrderID

	case PlanningOrderBuy:
		var prID int64
		err = tx.QueryRow(ctx, `
			INSERT INTO prs(number,request_by,status,note,company_id)
			SELECT 'PR-MRP-'||to_char(NOW(),'YYYYMMDDHH24MISSMS')||'-'||nextval('prs_id_seq')::text,
			       $3,'DRAFT','MRP recommendation '||$4::text,$1
			FROM products product
			WHERE product.id=$2 AND product.company_id=$1
			RETURNING id`, companyID, recommendation.ProductID, actorID, recommendationID).Scan(&prID)
		if errors.Is(err, pgx.ErrNoRows) {
			return FirmedRecommendation{}, ErrNotFound
		}
		if err != nil {
			return FirmedRecommendation{}, err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO pr_lines(pr_id,product_id,qty,note) VALUES($1,$2,$3,$4)`, prID, recommendation.ProductID, recommendation.Quantity, fmt.Sprintf("MRP recommendation %d for warehouse %d, due %s", recommendationID, recommendation.WarehouseID, planningDay(recommendation.DueDate).Format("2006-01-02"))); err != nil {
			return FirmedRecommendation{}, err
		}
		if _, err = tx.Exec(ctx, `UPDATE mrp_planning_recommendations SET status='FIRMED',firmed_pr_id=$1 WHERE id=$2`, prID, recommendationID); err != nil {
			return FirmedRecommendation{}, err
		}
		out.Status = "FIRMED"
		out.PurchaseRequestID = &prID

	default:
		return FirmedRecommendation{}, ErrInvalidState
	}
	if err := tx.Commit(ctx); err != nil {
		return FirmedRecommendation{}, err
	}
	return out, nil
}

func loadPlanningBOMs(ctx context.Context, db planningQuerier, companyID int64, asOf time.Time) (map[int64]PlanningBOM, error) {
	rows, err := db.Query(ctx, `
		WITH effective_boms AS (
			SELECT DISTINCT ON (product_id) id,product_id,scrap_pct
			FROM mrp_boms
			WHERE company_id=$1 AND active AND revision_status='APPROVED' AND effective_from <= $2
			  AND (effective_to IS NULL OR effective_to >= $2)
			ORDER BY product_id,effective_from DESC,id DESC
		)
		SELECT bom.product_id,bom.scrap_pct::float8,line.component_product_id,
		       line.quantity::float8,line.scrap_pct::float8
		FROM effective_boms bom
		JOIN mrp_bom_lines line ON line.bom_id=bom.id
		ORDER BY bom.product_id,line.id`, companyID, asOf)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	boms := make(map[int64]PlanningBOM)
	for rows.Next() {
		var productID int64
		var headerScrap float64
		var line PlanningBOMLine
		if err := rows.Scan(&productID, &headerScrap, &line.ComponentProductID, &line.Quantity, &line.ScrapPct); err != nil {
			return nil, err
		}
		bom := boms[productID]
		if bom.ProductID == 0 {
			bom = PlanningBOM{ProductID: productID, ScrapPct: headerScrap}
		}
		bom.Lines = append(bom.Lines, line)
		boms[productID] = bom
	}
	return boms, rows.Err()
}

type planningQuerier interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func loadPlanningInput(ctx context.Context, db planningQuerier, companyID int64, asOf time.Time) (PlanningInput, error) {
	input := PlanningInput{AsOf: asOf}
	policies, err := db.Query(ctx, `
		SELECT policy.product_id,policy.warehouse_id,policy.order_type,policy.lead_days,
		       policy.safety_stock::float8,policy.lot_sizing,policy.lot_quantity::float8
		FROM mrp_product_planning_policies policy
		JOIN products p ON p.id=policy.product_id AND p.company_id=policy.company_id
		JOIN warehouses w ON w.id=policy.warehouse_id
		JOIN branches b ON b.id=w.branch_id AND b.company_id=policy.company_id
		WHERE policy.company_id=$1 AND policy.active`, companyID)
	if err != nil {
		return PlanningInput{}, err
	}
	for policies.Next() {
		var policy PlanningPolicy
		if err := policies.Scan(&policy.ProductID, &policy.WarehouseID, &policy.OrderType, &policy.LeadDays, &policy.SafetyStock, &policy.LotSizing, &policy.LotQuantity); err != nil {
			policies.Close()
			return PlanningInput{}, err
		}
		input.Policies = append(input.Policies, policy)
	}
	if err := policies.Err(); err != nil {
		policies.Close()
		return PlanningInput{}, err
	}
	policies.Close()

	demands, err := db.Query(ctx, `
		SELECT line.product_id,line.fulfillment_warehouse_id,
		       COALESCE(order_header.expected_delivery_date,order_header.order_date)::timestamptz,
		       (line.quantity-line.quantity_delivered)::float8,'SO-LINE-'||line.id::text
		FROM sales_order_lines line
		JOIN sales_orders order_header ON order_header.id=line.sales_order_id
		WHERE order_header.company_id=$1
		  AND order_header.status IN ('CONFIRMED','PROCESSING')
		  AND line.fulfillment_warehouse_id IS NOT NULL
		  AND line.quantity > line.quantity_delivered`, companyID)
	if err != nil {
		return PlanningInput{}, err
	}
	for demands.Next() {
		var demand PlanningDemand
		if err := demands.Scan(&demand.ProductID, &demand.WarehouseID, &demand.DueDate, &demand.Quantity, &demand.SourceRef); err != nil {
			demands.Close()
			return PlanningInput{}, err
		}
		input.Demands = append(input.Demands, demand)
	}
	if err := demands.Err(); err != nil {
		demands.Close()
		return PlanningInput{}, err
	}
	demands.Close()

	supplies, err := db.Query(ctx, `
		WITH ordered_po_supply AS (
			SELECT po.id,po.expected_warehouse_id,po.expected_date,line.product_id,SUM(line.qty) AS ordered_qty
			FROM pos po
			JOIN po_lines line ON line.po_id=po.id
			WHERE po.company_id=$1 AND po.status='APPROVED' AND po.expected_warehouse_id IS NOT NULL
			GROUP BY po.id,po.expected_warehouse_id,po.expected_date,line.product_id
		), received_po_supply AS (
			SELECT receipt.po_id,receipt_line.product_id,SUM(receipt_line.qty) AS received_qty
			FROM grns receipt
			JOIN grn_lines receipt_line ON receipt_line.grn_id=receipt.id
			WHERE receipt.status='POSTED'
			GROUP BY receipt.po_id,receipt_line.product_id
		), approved_po_supply AS (
			SELECT ordered.id,ordered.expected_warehouse_id,ordered.expected_date,ordered.product_id,
			       ordered.ordered_qty,COALESCE(received.received_qty,0) AS received_qty
			FROM ordered_po_supply ordered
			LEFT JOIN received_po_supply received ON received.po_id=ordered.id AND received.product_id=ordered.product_id
		)
		SELECT balance.product_id,balance.warehouse_id,$2::timestamptz,balance.qty::float8,'ON-HAND'
		FROM inventory_balances balance
		JOIN mrp_product_planning_policies policy
		  ON policy.company_id=$1 AND policy.active
		 AND policy.product_id=balance.product_id AND policy.warehouse_id=balance.warehouse_id
		JOIN warehouses w ON w.id=balance.warehouse_id
		JOIN branches b ON b.id=w.branch_id AND b.company_id=$1
		WHERE balance.qty > 0
		UNION ALL
		SELECT po_supply.product_id,po_supply.expected_warehouse_id,
		       COALESCE(po_supply.expected_date,CURRENT_DATE)::timestamptz,
		       (po_supply.ordered_qty-po_supply.received_qty)::float8,'PO-'||po_supply.id::text
		FROM approved_po_supply po_supply
		JOIN mrp_product_planning_policies policy
		  ON policy.company_id=$1 AND policy.active
		 AND policy.product_id=po_supply.product_id AND policy.warehouse_id=po_supply.expected_warehouse_id
		WHERE po_supply.ordered_qty > po_supply.received_qty`, companyID, asOf)
	if err != nil {
		return PlanningInput{}, err
	}
	for supplies.Next() {
		var supply PlanningSupply
		if err := supplies.Scan(&supply.ProductID, &supply.WarehouseID, &supply.AvailableDate, &supply.Quantity, &supply.SourceRef); err != nil {
			supplies.Close()
			return PlanningInput{}, err
		}
		input.Supplies = append(input.Supplies, supply)
	}
	if err := supplies.Err(); err != nil {
		supplies.Close()
		return PlanningInput{}, err
	}
	supplies.Close()

	for _, policy := range input.Policies {
		if !validPlanningPolicy(policy) {
			return PlanningInput{}, fmt.Errorf("%w: invalid planning policy for product %d and warehouse %d", ErrInvalidPlanningInput, policy.ProductID, policy.WarehouseID)
		}
	}
	return input, nil
}

func validPlanningPolicy(policy PlanningPolicy) bool {
	if policy.ProductID <= 0 || policy.WarehouseID <= 0 || policy.LeadDays < 0 || policy.SafetyStock < 0 || policy.LotQuantity < 0 || (policy.OrderType != PlanningOrderBuy && policy.OrderType != PlanningOrderMake) {
		return false
	}
	switch policy.LotSizing {
	case LotForLot:
		return true
	case LotMinimum, LotFixed, LotMultiple:
		return policy.LotQuantity > 0
	default:
		return false
	}
}
