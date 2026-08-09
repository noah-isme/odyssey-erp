package distribution

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	accountingmoney "github.com/odyssey-erp/odyssey-erp/internal/accounting/money"
)

// Repository defines data access for distribution planning. The repository
// exposes distribution-owned types only; SQL and pgx types stay below this
// boundary.
type Repository interface {
	CreatePlanningHorizon(context.Context, CreatePlanningHorizonInput) (int64, error)
	GetPlanningHorizon(context.Context, int64) (*PlanningHorizon, error)
	ListPlanningHorizons(context.Context, int64) ([]*PlanningHorizon, error)
	UpdatePlanningHorizonStatus(context.Context, int64, HorizonStatus) error

	CreatePlanningRule(context.Context, CreatePlanningRuleInput) (int64, error)
	GetPlanningRule(context.Context, int64) (*PlanningRule, error)
	ListPlanningRules(context.Context, int64) ([]*PlanningRule, error)
	UpdateRuleActive(context.Context, int64, bool) error

	CreateLoad(context.Context, CreateLoadInput) (int64, error)
	GetLoad(context.Context, int64) (*Load, error)
	ListLoads(context.Context, int64, *LoadStatus) ([]*Load, error)
	UpdateLoadStatus(context.Context, int64, LoadStatus) error
	UpdateLoadDispatch(context.Context, int64, *int64, *int64, *int64, *string) error

	AddLoadItem(context.Context, AddLoadItemInput) (int64, error)
	GetLoadItems(context.Context, int64) ([]*LoadItem, error)

	CreateRoute(context.Context, CreateRouteInput) (int64, error)
	GetRoute(context.Context, int64) (*DeliveryRoute, error)
	ListRoutes(context.Context, int64, *RouteStatus) ([]*DeliveryRoute, error)
	UpdateRouteStatus(context.Context, int64, RouteStatus) error

	AddRouteStop(context.Context, AddRouteStopInput) (int64, error)
	GetRouteStop(context.Context, int64) (*RouteStop, error)
	GetRouteStops(context.Context, int64) ([]*RouteStop, error)
	UpdateStopActualTimes(context.Context, int64, *time.Time, *time.Time) error

	CreateTransferOrder(context.Context, CreateTransferOrderInput) (int64, error)
	GetTransferOrder(context.Context, int64) (*TransferOrder, error)
	ListTransferOrders(context.Context, int64, *TransferStatus) ([]*TransferOrder, error)
	UpdateTransferStatus(context.Context, int64, TransferStatus) error
	UpdateTransferDispatch(context.Context, int64, *int64, *int64, *int64) error

	AddTransferLine(context.Context, AddTransferLineInput) (int64, error)
	GetTransferLines(context.Context, int64) ([]*TransferOrderLine, error)
	UpdateTransferLineReceipt(context.Context, int64, accountingmoney.Money) error
}

type DistributionRepository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *DistributionRepository {
	return &DistributionRepository{db: db}
}

type CreatePlanningHorizonInput struct {
	CompanyID         int64
	WarehouseID       int64
	PlanningStartDate time.Time
	PlanningEndDate   time.Time
	FrozenUntilDate   *time.Time
	Notes             string
	CreatedBy         int64
}

func (r *DistributionRepository) CreatePlanningHorizon(ctx context.Context, input CreatePlanningHorizonInput) (int64, error) {
	const query = `
		INSERT INTO planning_horizons
			(company_id, warehouse_id, planning_start_date, planning_end_date,
			 frozen_until_date, status, notes, created_by)
		VALUES ($1, $2, $3, $4, $5, 'ACTIVE', $6, $7)
		RETURNING id`
	var id int64
	err := r.db.QueryRow(ctx, query,
		input.CompanyID,
		input.WarehouseID,
		dateValue(input.PlanningStartDate),
		dateValue(input.PlanningEndDate),
		dateValuePtr(input.FrozenUntilDate),
		input.Notes,
		input.CreatedBy,
	).Scan(&id)
	return id, err
}

func (r *DistributionRepository) GetPlanningHorizon(ctx context.Context, id int64) (*PlanningHorizon, error) {
	const query = `
		SELECT id, company_id, warehouse_id, planning_start_date, planning_end_date,
		       frozen_until_date, status, notes, created_by, created_at, updated_at
		FROM planning_horizons WHERE id = $1`
	return scanPlanningHorizon(r.db.QueryRow(ctx, query, id))
}

func (r *DistributionRepository) ListPlanningHorizons(ctx context.Context, companyID int64) ([]*PlanningHorizon, error) {
	const query = `
		SELECT id, company_id, warehouse_id, planning_start_date, planning_end_date,
		       frozen_until_date, status, notes, created_by, created_at, updated_at
		FROM planning_horizons
		WHERE company_id = $1 AND status = 'ACTIVE'
		ORDER BY planning_start_date DESC`
	rows, err := r.db.Query(ctx, query, companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*PlanningHorizon
	for rows.Next() {
		horizon, err := scanPlanningHorizon(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, horizon)
	}
	return result, rows.Err()
}

func (r *DistributionRepository) UpdatePlanningHorizonStatus(ctx context.Context, id int64, status HorizonStatus) error {
	_, err := r.db.Exec(ctx, `UPDATE planning_horizons SET status = $1, updated_at = NOW() WHERE id = $2`, string(status), id)
	return err
}

type CreatePlanningRuleInput struct {
	CompanyID            int64
	WarehouseID          int64
	RuleName             string
	RuleType             RuleType
	MaxLoadWeightKg      *accountingmoney.Money
	MaxLoadVolumeCbm     *accountingmoney.Money
	MaxItemsPerLoad      *int
	TimeWindowStart      *time.Time
	TimeWindowEnd        *time.Time
	VehicleTypeRequired  string
	CustomRuleExpression string
	Priority             int
	CreatedBy            int64
}

func (r *DistributionRepository) CreatePlanningRule(ctx context.Context, input CreatePlanningRuleInput) (int64, error) {
	const query = `
		INSERT INTO planning_rules
			(company_id, warehouse_id, rule_name, rule_type, max_load_weight_kg,
			 max_load_volume_cbm, max_items_per_load, time_window_start,
			 time_window_end, vehicle_type_required, custom_rule_expression,
			 priority, is_active, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, TRUE, $13)
		RETURNING id`
	var id int64
	err := r.db.QueryRow(ctx, query,
		input.CompanyID,
		input.WarehouseID,
		input.RuleName,
		string(input.RuleType),
		moneyValue(input.MaxLoadWeightKg),
		moneyValue(input.MaxLoadVolumeCbm),
		intValue(input.MaxItemsPerLoad),
		timeValue(input.TimeWindowStart),
		timeValue(input.TimeWindowEnd),
		input.VehicleTypeRequired,
		input.CustomRuleExpression,
		input.Priority,
		input.CreatedBy,
	).Scan(&id)
	return id, err
}

func (r *DistributionRepository) GetPlanningRule(ctx context.Context, id int64) (*PlanningRule, error) {
	const query = `
		SELECT id, company_id, warehouse_id, rule_name, rule_type,
		       max_load_weight_kg, max_load_volume_cbm, max_items_per_load,
		       time_window_start, time_window_end, vehicle_type_required,
		       custom_rule_expression, priority, is_active, created_at, created_by
		FROM planning_rules WHERE id = $1`
	return scanPlanningRule(r.db.QueryRow(ctx, query, id))
}

func (r *DistributionRepository) ListPlanningRules(ctx context.Context, warehouseID int64) ([]*PlanningRule, error) {
	const query = `
		SELECT id, company_id, warehouse_id, rule_name, rule_type,
		       max_load_weight_kg, max_load_volume_cbm, max_items_per_load,
		       time_window_start, time_window_end, vehicle_type_required,
		       custom_rule_expression, priority, is_active, created_at, created_by
		FROM planning_rules
		WHERE warehouse_id = $1 AND is_active = TRUE
		ORDER BY priority ASC, id ASC`
	rows, err := r.db.Query(ctx, query, warehouseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*PlanningRule
	for rows.Next() {
		rule, err := scanPlanningRule(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, rule)
	}
	return result, rows.Err()
}

func (r *DistributionRepository) UpdateRuleActive(ctx context.Context, id int64, active bool) error {
	_, err := r.db.Exec(ctx, `UPDATE planning_rules SET is_active = $1 WHERE id = $2`, active, id)
	return err
}

type CreateLoadInput struct {
	CompanyID              int64
	OriginWarehouseID      int64
	DestinationWarehouseID *int64
	DestinationAddress     string
	DestinationCity        string
	DestinationCountry     string
	PlannedPickupDate      *time.Time
	PlannedDeliveryDate    *time.Time
	Notes                  string
	CreatedBy              int64
}

func (r *DistributionRepository) CreateLoad(ctx context.Context, input CreateLoadInput) (int64, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	const insert = `
		INSERT INTO loads
			(company_id, load_number, origin_warehouse_id, destination_warehouse_id,
			 destination_address, destination_city, destination_country,
			 planned_pickup_date, planned_delivery_date, status, notes, created_by)
		VALUES ($1, 'PENDING-' || md5(random()::text || clock_timestamp()::text),
				$2, $3, $4, $5, $6, $7, $8, 'DRAFT', $9, $10)
		RETURNING id`
	var id int64
	err = tx.QueryRow(ctx, insert,
		input.CompanyID,
		input.OriginWarehouseID,
		input.DestinationWarehouseID,
		input.DestinationAddress,
		input.DestinationCity,
		input.DestinationCountry,
		dateValuePtr(input.PlannedPickupDate),
		dateValuePtr(input.PlannedDeliveryDate),
		input.Notes,
		input.CreatedBy,
	).Scan(&id)
	if err != nil {
		return 0, err
	}
	if _, err = tx.Exec(ctx, `UPDATE loads SET load_number = 'LOAD-' || TO_CHAR(CURRENT_DATE, 'YYYYMMDD') || '-' || id WHERE id = $1`, id); err != nil {
		return 0, err
	}
	if err = tx.Commit(ctx); err != nil {
		return 0, err
	}
	return id, nil
}

const loadSelect = `
	id, company_id, load_number, status, origin_warehouse_id,
	destination_warehouse_id, destination_address, destination_city,
	destination_country, vehicle_id, driver_id, carrier_id,
	carrier_service_type, total_weight_kg, total_volume_cbm, total_items,
	planned_pickup_date, planned_delivery_date, actual_dispatch_at,
	actual_delivery_at, freight_charge, freight_currency, notes, created_at,
	updated_at, created_by`

func (r *DistributionRepository) GetLoad(ctx context.Context, id int64) (*Load, error) {
	return scanLoad(r.db.QueryRow(ctx, `SELECT `+loadSelect+` FROM loads WHERE id = $1`, id))
}

func (r *DistributionRepository) ListLoads(ctx context.Context, companyID int64, status *LoadStatus) ([]*Load, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+loadSelect+` FROM loads WHERE company_id = $1 AND (status = $2 OR $2 IS NULL) ORDER BY created_at DESC`,
		companyID, statusValue(status))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*Load
	for rows.Next() {
		load, err := scanLoad(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, load)
	}
	return result, rows.Err()
}

func (r *DistributionRepository) UpdateLoadStatus(ctx context.Context, id int64, status LoadStatus) error {
	const query = `
		UPDATE loads
		SET status = $1,
		    actual_dispatch_at = CASE
				WHEN $1 IN ('DISPATCHED', 'IN_TRANSIT') THEN COALESCE(actual_dispatch_at, NOW())
				ELSE actual_dispatch_at END,
		    actual_delivery_at = CASE
				WHEN $1 = 'DELIVERED' THEN COALESCE(actual_delivery_at, NOW())
				ELSE actual_delivery_at END,
		    updated_at = NOW()
		WHERE id = $2`
	_, err := r.db.Exec(ctx, query, string(status), id)
	return err
}

func (r *DistributionRepository) UpdateLoadDispatch(ctx context.Context, id int64, vehicleID, driverID, carrierID *int64, carrierService *string) error {
	const query = `
		UPDATE loads
		SET vehicle_id = $1, driver_id = $2, carrier_id = $3,
		    carrier_service_type = $4, updated_at = NOW()
		WHERE id = $5`
	_, err := r.db.Exec(ctx, query, vehicleID, driverID, carrierID, carrierService, id)
	return err
}

type AddLoadItemInput struct {
	CompanyID  int64
	LoadID     int64
	ShipmentID *int64
	ProductID  int64
	Quantity   accountingmoney.Money
	WeightKg   *accountingmoney.Money
	VolumeCbm  *accountingmoney.Money
}

func (r *DistributionRepository) AddLoadItem(ctx context.Context, input AddLoadItemInput) (int64, error) {
	const query = `
		INSERT INTO load_items
			(company_id, load_id, shipment_id, product_id, quantity, weight_kg, volume_cbm)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id`
	var id int64
	err := r.db.QueryRow(ctx, query,
		input.CompanyID,
		input.LoadID,
		input.ShipmentID,
		input.ProductID,
		moneyValue(&input.Quantity),
		moneyValue(input.WeightKg),
		moneyValue(input.VolumeCbm),
	).Scan(&id)
	return id, err
}

func (r *DistributionRepository) GetLoadItems(ctx context.Context, loadID int64) ([]*LoadItem, error) {
	const query = `
		SELECT id, company_id, load_id, shipment_id, product_id,
		       quantity, weight_kg, volume_cbm, created_at
		FROM load_items WHERE load_id = $1 ORDER BY created_at ASC`
	rows, err := r.db.Query(ctx, query, loadID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*LoadItem
	for rows.Next() {
		item, err := scanLoadItem(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

type CreateRouteInput struct {
	CompanyID                int64
	LoadID                   int64
	TotalDistanceKm          *float64
	EstimatedDurationMinutes *int
	OptimizationScore        *accountingmoney.Money
	CreatedBy                int64
}

func (r *DistributionRepository) CreateRoute(ctx context.Context, input CreateRouteInput) (int64, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	const insert = `
		INSERT INTO delivery_routes
			(company_id, load_id, route_number, total_distance_km,
			 estimated_duration_minutes, optimization_score, status, created_by)
		VALUES ($1, $2, 'PENDING-' || md5(random()::text || clock_timestamp()::text),
				$3, $4, $5, 'DRAFT', $6)
		RETURNING id`
	var id int64
	err = tx.QueryRow(ctx, insert,
		input.CompanyID,
		input.LoadID,
		input.TotalDistanceKm,
		input.EstimatedDurationMinutes,
		moneyValue(input.OptimizationScore),
		input.CreatedBy,
	).Scan(&id)
	if err != nil {
		return 0, err
	}
	if _, err = tx.Exec(ctx, `UPDATE delivery_routes SET route_number = 'ROUTE-' || TO_CHAR(CURRENT_DATE, 'YYYYMMDD') || '-' || id WHERE id = $1`, id); err != nil {
		return 0, err
	}
	if err = tx.Commit(ctx); err != nil {
		return 0, err
	}
	return id, nil
}

const routeSelect = `
	id, company_id, load_id, route_number, status, total_distance_km,
	estimated_duration_minutes, optimization_score, created_at, updated_at,
	created_by`

func (r *DistributionRepository) GetRoute(ctx context.Context, id int64) (*DeliveryRoute, error) {
	return scanRoute(r.db.QueryRow(ctx, `SELECT `+routeSelect+` FROM delivery_routes WHERE id = $1`, id))
}

func (r *DistributionRepository) ListRoutes(ctx context.Context, companyID int64, status *RouteStatus) ([]*DeliveryRoute, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+routeSelect+` FROM delivery_routes WHERE company_id = $1 AND (status = $2 OR $2 IS NULL) ORDER BY created_at DESC`,
		companyID, statusValue(status))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*DeliveryRoute
	for rows.Next() {
		route, err := scanRoute(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, route)
	}
	return result, rows.Err()
}

func (r *DistributionRepository) UpdateRouteStatus(ctx context.Context, id int64, status RouteStatus) error {
	_, err := r.db.Exec(ctx, `UPDATE delivery_routes SET status = $1, updated_at = NOW() WHERE id = $2`, string(status), id)
	return err
}

type AddRouteStopInput struct {
	CompanyID            int64
	RouteID              int64
	StopSequence         int
	StopType             StopType
	WarehouseID          *int64
	CustomerID           *int64
	CustomerAddress      string
	CustomerCity         string
	LocationLat          *float64
	LocationLon          *float64
	ContactName          string
	ContactPhone         string
	PlannedArrivalTime   *time.Time
	PlannedDepartureTime *time.Time
	Notes                string
}

func (r *DistributionRepository) AddRouteStop(ctx context.Context, input AddRouteStopInput) (int64, error) {
	const query = `
		INSERT INTO route_stops
			(company_id, route_id, stop_sequence, stop_type, warehouse_id,
			 customer_id, customer_address, customer_city, location_lat,
			 location_lon, contact_name, contact_phone, planned_arrival_time,
			 planned_departure_time, notes)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
		RETURNING id`
	var id int64
	err := r.db.QueryRow(ctx, query,
		input.CompanyID,
		input.RouteID,
		input.StopSequence,
		string(input.StopType),
		input.WarehouseID,
		input.CustomerID,
		input.CustomerAddress,
		input.CustomerCity,
		input.LocationLat,
		input.LocationLon,
		input.ContactName,
		input.ContactPhone,
		timeValue(input.PlannedArrivalTime),
		timeValue(input.PlannedDepartureTime),
		input.Notes,
	).Scan(&id)
	return id, err
}

const routeStopSelect = `
	id, company_id, route_id, stop_sequence, stop_type, warehouse_id,
	customer_id, customer_address, customer_city, location_lat, location_lon,
	contact_name, contact_phone, planned_arrival_time, planned_departure_time,
	actual_arrival_at, actual_departure_at, items_delivered, notes, created_at`

func (r *DistributionRepository) GetRouteStop(ctx context.Context, id int64) (*RouteStop, error) {
	return scanRouteStop(r.db.QueryRow(ctx, `SELECT `+routeStopSelect+` FROM route_stops WHERE id = $1`, id))
}

func (r *DistributionRepository) GetRouteStops(ctx context.Context, routeID int64) ([]*RouteStop, error) {
	rows, err := r.db.Query(ctx, `SELECT `+routeStopSelect+` FROM route_stops WHERE route_id = $1 ORDER BY stop_sequence ASC`, routeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*RouteStop
	for rows.Next() {
		stop, err := scanRouteStop(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, stop)
	}
	return result, rows.Err()
}

func (r *DistributionRepository) UpdateStopActualTimes(ctx context.Context, id int64, arrivedAt, departedAt *time.Time) error {
	_, err := r.db.Exec(ctx,
		`UPDATE route_stops SET actual_arrival_at = $1, actual_departure_at = $2 WHERE id = $3`,
		timeValue(arrivedAt), timeValue(departedAt), id)
	return err
}

type CreateTransferOrderInput struct {
	CompanyID           int64
	FromWarehouseID     int64
	ToWarehouseID       int64
	PlannedDispatchDate *time.Time
	PlannedArrivalDate  *time.Time
	Notes               string
	CreatedBy           int64
}

func (r *DistributionRepository) CreateTransferOrder(ctx context.Context, input CreateTransferOrderInput) (int64, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	const insert = `
		INSERT INTO transfer_orders
			(company_id, transfer_number, from_warehouse_id, to_warehouse_id,
			 planned_dispatch_date, planned_arrival_date, status, notes, created_by)
		VALUES ($1, 'PENDING-' || md5(random()::text || clock_timestamp()::text),
				$2, $3, $4, $5, 'DRAFT', $6, $7)
		RETURNING id`
	var id int64
	err = tx.QueryRow(ctx, insert,
		input.CompanyID,
		input.FromWarehouseID,
		input.ToWarehouseID,
		dateValuePtr(input.PlannedDispatchDate),
		dateValuePtr(input.PlannedArrivalDate),
		input.Notes,
		input.CreatedBy,
	).Scan(&id)
	if err != nil {
		return 0, err
	}
	if _, err = tx.Exec(ctx, `UPDATE transfer_orders SET transfer_number = 'TRANSFER-' || TO_CHAR(CURRENT_DATE, 'YYYYMMDD') || '-' || id WHERE id = $1`, id); err != nil {
		return 0, err
	}
	if err = tx.Commit(ctx); err != nil {
		return 0, err
	}
	return id, nil
}

const transferSelect = `
	id, company_id, transfer_number, status, from_warehouse_id, to_warehouse_id,
	load_id, vehicle_id, driver_id, carrier_id, carrier_service_type,
	planned_dispatch_date, planned_arrival_date, actual_dispatch_at,
	actual_arrival_at, total_weight_kg, total_volume_cbm, total_items,
	in_transit_quantity, transfer_cost, transfer_cost_currency, notes,
	created_at, updated_at, created_by`

func (r *DistributionRepository) GetTransferOrder(ctx context.Context, id int64) (*TransferOrder, error) {
	return scanTransfer(r.db.QueryRow(ctx, `SELECT `+transferSelect+` FROM transfer_orders WHERE id = $1`, id))
}

func (r *DistributionRepository) ListTransferOrders(ctx context.Context, companyID int64, status *TransferStatus) ([]*TransferOrder, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+transferSelect+` FROM transfer_orders WHERE company_id = $1 AND (status = $2 OR $2 IS NULL) ORDER BY created_at DESC`,
		companyID, statusValue(status))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*TransferOrder
	for rows.Next() {
		transfer, err := scanTransfer(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, transfer)
	}
	return result, rows.Err()
}

func (r *DistributionRepository) UpdateTransferStatus(ctx context.Context, id int64, status TransferStatus) error {
	const query = `
		UPDATE transfer_orders
		SET status = $1,
		    actual_dispatch_at = CASE
				WHEN $1 IN ('DISPATCHED', 'IN_TRANSIT') THEN COALESCE(actual_dispatch_at, NOW())
				ELSE actual_dispatch_at END,
		    actual_arrival_at = CASE
				WHEN $1 = 'RECEIVED' THEN COALESCE(actual_arrival_at, NOW())
				ELSE actual_arrival_at END,
		    updated_at = NOW()
		WHERE id = $2`
	_, err := r.db.Exec(ctx, query, string(status), id)
	return err
}

func (r *DistributionRepository) UpdateTransferDispatch(ctx context.Context, id int64, vehicleID, driverID, carrierID *int64) error {
	_, err := r.db.Exec(ctx,
		`UPDATE transfer_orders SET vehicle_id = $1, driver_id = $2, carrier_id = $3, updated_at = NOW() WHERE id = $4`,
		vehicleID, driverID, carrierID, id)
	return err
}

type AddTransferLineInput struct {
	CompanyID         int64
	TransferOrderID   int64
	ProductID         int64
	QuantityRequested accountingmoney.Money
	LotNumber         string
	SerialNumbers     []string
}

func (r *DistributionRepository) AddTransferLine(ctx context.Context, input AddTransferLineInput) (int64, error) {
	const query = `
		INSERT INTO transfer_order_lines
			(company_id, transfer_order_id, product_id, quantity_requested,
			 lot_number, serial_numbers)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id`
	var id int64
	err := r.db.QueryRow(ctx, query,
		input.CompanyID,
		input.TransferOrderID,
		input.ProductID,
		moneyValue(&input.QuantityRequested),
		input.LotNumber,
		input.SerialNumbers,
	).Scan(&id)
	return id, err
}

const transferLineSelect = `
	id, company_id, transfer_order_id, product_id, quantity_requested,
	quantity_shipped, quantity_received, lot_number, serial_numbers, created_at`

func (r *DistributionRepository) GetTransferLines(ctx context.Context, transferID int64) ([]*TransferOrderLine, error) {
	rows, err := r.db.Query(ctx, `SELECT `+transferLineSelect+` FROM transfer_order_lines WHERE transfer_order_id = $1 ORDER BY created_at ASC`, transferID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*TransferOrderLine
	for rows.Next() {
		line, err := scanTransferLine(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, line)
	}
	return result, rows.Err()
}

func (r *DistributionRepository) UpdateTransferLineReceipt(ctx context.Context, id int64, quantityReceived accountingmoney.Money) error {
	_, err := r.db.Exec(ctx,
		`UPDATE transfer_order_lines SET quantity_received = $1 WHERE id = $2`,
		moneyValue(&quantityReceived), id)
	return err
}

type rowScanner interface {
	Scan(...any) error
}

func scanPlanningHorizon(row rowScanner) (*PlanningHorizon, error) {
	var result PlanningHorizon
	var start, end, frozen pgtype.Date
	var status pgtype.Text
	var notes pgtype.Text
	var createdAt, updatedAt pgtype.Timestamptz
	err := row.Scan(
		&result.ID, &result.CompanyID, &result.WarehouseID, &start, &end, &frozen,
		&status, &notes, &result.CreatedBy, &createdAt, &updatedAt,
	)
	if err != nil {
		return nil, err
	}
	result.PlanningStartDate = start.Time
	result.PlanningEndDate = end.Time
	result.FrozenUntilDate = datePtr(frozen)
	result.Status = HorizonStatus(status.String)
	result.Notes = notes.String
	result.CreatedAt = createdAt.Time
	result.UpdatedAt = updatedAt.Time
	return &result, nil
}

func scanPlanningRule(row rowScanner) (*PlanningRule, error) {
	var result PlanningRule
	var weight, volume pgtype.Numeric
	var maxItems pgtype.Int4
	var start, end pgtype.Time
	var vehicle, custom pgtype.Text
	var createdAt pgtype.Timestamptz
	var ruleType pgtype.Text
	var active pgtype.Bool
	err := row.Scan(
		&result.ID, &result.CompanyID, &result.WarehouseID, &result.RuleName, &ruleType,
		&weight, &volume, &maxItems, &start, &end, &vehicle, &custom,
		&result.Priority, &active, &createdAt, &result.CreatedBy,
	)
	if err != nil {
		return nil, err
	}
	var conversionErr error
	if result.MaxLoadWeightKg, conversionErr = numericMoney(weight, 4); conversionErr != nil {
		return nil, conversionErr
	}
	if result.MaxLoadVolumeCbm, conversionErr = numericMoney(volume, 4); conversionErr != nil {
		return nil, conversionErr
	}
	result.MaxItemsPerLoad = intPtr(maxItems)
	result.TimeWindowStart = pgTimePtr(start)
	result.TimeWindowEnd = pgTimePtr(end)
	result.RuleType = RuleType(ruleType.String)
	result.VehicleTypeRequired = vehicle.String
	result.CustomRuleExpression = custom.String
	result.IsActive = active.Bool
	result.CreatedAt = createdAt.Time
	return &result, nil
}

func scanLoad(row rowScanner) (*Load, error) {
	var result Load
	var status pgtype.Text
	var destinationAddress, destinationCity, destinationCountry pgtype.Text
	var carrierService, currency, notes pgtype.Text
	var destinationWarehouse, vehicle, driver, carrier pgtype.Int8
	var weight, volume, freightCharge pgtype.Numeric
	var totalItems pgtype.Int4
	var pickup, delivery pgtype.Date
	var actualDispatch, actualDelivery, createdAt, updatedAt pgtype.Timestamptz
	err := row.Scan(
		&result.ID, &result.CompanyID, &result.LoadNumber, &status, &result.OriginWarehouseID,
		&destinationWarehouse, &destinationAddress, &destinationCity, &destinationCountry,
		&vehicle, &driver, &carrier, &carrierService, &weight, &volume, &totalItems,
		&pickup, &delivery, &actualDispatch, &actualDelivery, &freightCharge, &currency,
		&notes, &createdAt, &updatedAt, &result.CreatedBy,
	)
	if err != nil {
		return nil, err
	}
	var conversionErr error
	if result.TotalWeightKg, conversionErr = numericMoney(weight, 4); conversionErr != nil {
		return nil, conversionErr
	}
	if result.TotalVolumeCbm, conversionErr = numericMoney(volume, 4); conversionErr != nil {
		return nil, conversionErr
	}
	if result.FreightCharge, conversionErr = numericMoney(freightCharge, 4); conversionErr != nil {
		return nil, conversionErr
	}
	result.Status = LoadStatus(status.String)
	result.DestinationWarehouseID = int64Ptr(destinationWarehouse)
	result.DestinationAddress = destinationAddress.String
	result.DestinationCity = destinationCity.String
	result.DestinationCountry = destinationCountry.String
	result.VehicleID = int64Ptr(vehicle)
	result.DriverID = int64Ptr(driver)
	result.CarrierID = int64Ptr(carrier)
	result.CarrierServiceType = textPtr(carrierService)
	result.TotalItems = intPtr(totalItems)
	result.PlannedPickupDate = datePtr(pickup)
	result.PlannedDeliveryDate = datePtr(delivery)
	result.ActualDispatchAt = timestamptzPtr(actualDispatch)
	result.ActualDeliveryAt = timestamptzPtr(actualDelivery)
	result.FreightCurrency = currency.String
	result.Notes = notes.String
	result.CreatedAt = createdAt.Time
	result.UpdatedAt = updatedAt.Time
	return &result, nil
}

func scanLoadItem(row rowScanner) (*LoadItem, error) {
	var result LoadItem
	var quantity, weight, volume pgtype.Numeric
	err := row.Scan(&result.ID, &result.CompanyID, &result.LoadID, &result.ShipmentID,
		&result.ProductID, &quantity, &weight, &volume, &result.CreatedAt)
	if err != nil {
		return nil, err
	}
	var conversionErr error
	if result.Quantity, conversionErr = numericValue(quantity, 4); conversionErr != nil {
		return nil, conversionErr
	}
	if result.WeightKg, conversionErr = numericMoney(weight, 4); conversionErr != nil {
		return nil, conversionErr
	}
	if result.VolumeCbm, conversionErr = numericMoney(volume, 4); conversionErr != nil {
		return nil, conversionErr
	}
	return &result, nil
}

func scanRoute(row rowScanner) (*DeliveryRoute, error) {
	var result DeliveryRoute
	var status pgtype.Text
	var distance, score pgtype.Numeric
	var estimated pgtype.Int4
	var createdAt, updatedAt pgtype.Timestamptz
	err := row.Scan(&result.ID, &result.CompanyID, &result.LoadID, &result.RouteNumber,
		&status, &distance, &estimated, &score, &createdAt, &updatedAt, &result.CreatedBy)
	if err != nil {
		return nil, err
	}
	var conversionErr error
	if result.TotalDistanceKm, conversionErr = numericFloat(distance); conversionErr != nil {
		return nil, conversionErr
	}
	if result.OptimizationScore, conversionErr = numericMoney(score, 2); conversionErr != nil {
		return nil, conversionErr
	}
	result.Status = RouteStatus(status.String)
	result.EstimatedDurationMinutes = intPtr(estimated)
	result.CreatedAt = createdAt.Time
	result.UpdatedAt = updatedAt.Time
	return &result, nil
}

func scanRouteStop(row rowScanner) (*RouteStop, error) {
	var result RouteStop
	var stopType pgtype.Text
	var customerAddress, customerCity, contactName, contactPhone, notes pgtype.Text
	var warehouse, customer pgtype.Int8
	var lat, lon pgtype.Numeric
	var plannedArrival, plannedDeparture pgtype.Time
	var actualArrival, actualDeparture pgtype.Timestamptz
	var itemsDelivered pgtype.Int4
	err := row.Scan(&result.ID, &result.CompanyID, &result.RouteID, &result.StopSequence,
		&stopType, &warehouse, &customer, &customerAddress, &customerCity, &lat, &lon,
		&contactName, &contactPhone, &plannedArrival, &plannedDeparture,
		&actualArrival, &actualDeparture, &itemsDelivered, &notes, &result.CreatedAt)
	if err != nil {
		return nil, err
	}
	var conversionErr error
	if result.LocationLat, conversionErr = numericFloat(lat); conversionErr != nil {
		return nil, conversionErr
	}
	if result.LocationLon, conversionErr = numericFloat(lon); conversionErr != nil {
		return nil, conversionErr
	}
	result.StopType = StopType(stopType.String)
	result.WarehouseID = int64Ptr(warehouse)
	result.CustomerID = int64Ptr(customer)
	result.CustomerAddress = customerAddress.String
	result.CustomerCity = customerCity.String
	result.ContactName = contactName.String
	result.ContactPhone = contactPhone.String
	result.PlannedArrivalTime = pgTimePtr(plannedArrival)
	result.PlannedDepartureTime = pgTimePtr(plannedDeparture)
	result.ActualArrivalAt = timestamptzPtr(actualArrival)
	result.ActualDepartureAt = timestamptzPtr(actualDeparture)
	result.ItemsDelivered = intPtr(itemsDelivered)
	result.Notes = notes.String
	return &result, nil
}

func scanTransfer(row rowScanner) (*TransferOrder, error) {
	var result TransferOrder
	var status, carrierService, currency, notes pgtype.Text
	var load, vehicle, driver, carrier pgtype.Int8
	var dispatchDate, arrivalDate pgtype.Date
	var actualDispatch, actualArrival, createdAt, updatedAt pgtype.Timestamptz
	var weight, volume, inTransit, cost pgtype.Numeric
	var totalItems pgtype.Int4
	err := row.Scan(&result.ID, &result.CompanyID, &result.TransferNumber, &status,
		&result.FromWarehouseID, &result.ToWarehouseID, &load, &vehicle, &driver,
		&carrier, &carrierService, &dispatchDate, &arrivalDate, &actualDispatch,
		&actualArrival, &weight, &volume, &totalItems, &inTransit, &cost, &currency,
		&notes, &createdAt, &updatedAt, &result.CreatedBy)
	if err != nil {
		return nil, err
	}
	var conversionErr error
	if result.TotalWeightKg, conversionErr = numericMoney(weight, 4); conversionErr != nil {
		return nil, conversionErr
	}
	if result.TotalVolumeCbm, conversionErr = numericMoney(volume, 4); conversionErr != nil {
		return nil, conversionErr
	}
	if result.InTransitQuantity, conversionErr = numericMoney(inTransit, 4); conversionErr != nil {
		return nil, conversionErr
	}
	if result.TransferCost, conversionErr = numericMoney(cost, 4); conversionErr != nil {
		return nil, conversionErr
	}
	result.Status = TransferStatus(status.String)
	result.LoadID = int64Ptr(load)
	result.VehicleID = int64Ptr(vehicle)
	result.DriverID = int64Ptr(driver)
	result.CarrierID = int64Ptr(carrier)
	result.CarrierServiceType = textPtr(carrierService)
	result.PlannedDispatchDate = datePtr(dispatchDate)
	result.PlannedArrivalDate = datePtr(arrivalDate)
	result.ActualDispatchAt = timestamptzPtr(actualDispatch)
	result.ActualArrivalAt = timestamptzPtr(actualArrival)
	result.TotalItems = intPtr(totalItems)
	result.TransferCostCurrency = currency.String
	result.Notes = notes.String
	result.CreatedAt = createdAt.Time
	result.UpdatedAt = updatedAt.Time
	return &result, nil
}

func scanTransferLine(row rowScanner) (*TransferOrderLine, error) {
	var result TransferOrderLine
	var requested, shipped, received pgtype.Numeric
	var lotNumber pgtype.Text
	err := row.Scan(&result.ID, &result.CompanyID, &result.TransferOrderID, &result.ProductID,
		&requested, &shipped, &received, &lotNumber, &result.SerialNumbers, &result.CreatedAt)
	if err != nil {
		return nil, err
	}
	var conversionErr error
	if result.QuantityRequested, conversionErr = numericValue(requested, 4); conversionErr != nil {
		return nil, conversionErr
	}
	if result.QuantityShipped, conversionErr = numericMoney(shipped, 4); conversionErr != nil {
		return nil, conversionErr
	}
	if result.QuantityReceived, conversionErr = numericMoney(received, 4); conversionErr != nil {
		return nil, conversionErr
	}
	result.LotNumber = lotNumber.String
	return &result, nil
}

func moneyValue(value *accountingmoney.Money) any {
	if value == nil {
		return nil
	}
	return value.String()
}

func numericValue(value pgtype.Numeric, scale int) (accountingmoney.Money, error) {
	amount, err := numericString(value)
	if err != nil {
		return accountingmoney.Money{}, err
	}
	return accountingmoney.Parse(amount, scale)
}

func numericMoney(value pgtype.Numeric, scale int) (*accountingmoney.Money, error) {
	if !value.Valid {
		return nil, nil
	}
	result, err := numericValue(value, scale)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func numericFloat(value pgtype.Numeric) (*float64, error) {
	if !value.Valid {
		return nil, nil
	}
	result, err := value.Float64Value()
	if err != nil {
		return nil, err
	}
	if !result.Valid {
		return nil, nil
	}
	return &result.Float64, nil
}

func numericString(value pgtype.Numeric) (string, error) {
	if !value.Valid {
		return "0", nil
	}
	if value.NaN || value.InfinityModifier != pgtype.Finite || value.Int == nil {
		return "", fmt.Errorf("distribution: unsupported PostgreSQL numeric value")
	}
	digits := value.Int.String()
	if value.Exp >= 0 {
		return digits + strings.Repeat("0", int(value.Exp)), nil
	}
	sign := ""
	if strings.HasPrefix(digits, "-") || strings.HasPrefix(digits, "+") {
		sign, digits = digits[:1], digits[1:]
	}
	decimal := len(digits) + int(value.Exp)
	if decimal <= 0 {
		return sign + "0." + strings.Repeat("0", -decimal) + digits, nil
	}
	if decimal >= len(digits) {
		return sign + digits + strings.Repeat("0", decimal-len(digits)), nil
	}
	return sign + digits[:decimal] + "." + digits[decimal:], nil
}

func dateValue(value time.Time) pgtype.Date {
	if value.IsZero() {
		return pgtype.Date{}
	}
	return pgtype.Date{Time: value, Valid: true}
}

func dateValuePtr(value *time.Time) any {
	if value == nil {
		return nil
	}
	return dateValue(*value)
}

func datePtr(value pgtype.Date) *time.Time {
	if !value.Valid {
		return nil
	}
	result := value.Time
	return &result
}

func timeValue(value *time.Time) any {
	if value == nil {
		return nil
	}
	if value.Hour() == 0 && value.Minute() == 0 && value.Second() == 0 && value.Nanosecond() == 0 && value.Year() == 1 {
		return pgtype.Time{Valid: true}
	}
	return pgtype.Time{
		Microseconds: int64(value.Hour())*60*60*1_000_000 +
			int64(value.Minute())*60*1_000_000 +
			int64(value.Second())*1_000_000 + int64(value.Nanosecond()/1_000),
		Valid: true,
	}
}

func pgTimePtr(value pgtype.Time) *time.Time {
	if !value.Valid {
		return nil
	}
	const microsPerSecond = int64(1_000_000)
	seconds := value.Microseconds / microsPerSecond
	micros := value.Microseconds % microsPerSecond
	result := time.Date(0, time.January, 1, int(seconds/3600), int((seconds%3600)/60), int(seconds%60), int(micros)*1_000, time.UTC)
	return &result
}

func timestamptzPtr(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	result := value.Time
	return &result
}

func int64Ptr(value pgtype.Int8) *int64 {
	if !value.Valid {
		return nil
	}
	result := value.Int64
	return &result
}

func intPtr(value pgtype.Int4) *int {
	if !value.Valid {
		return nil
	}
	result := int(value.Int32)
	return &result
}

func textPtr(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	result := value.String
	return &result
}

func statusValue[T ~string](status *T) any {
	if status == nil {
		return nil
	}
	return string(*status)
}

func intValue(value *int) any {
	if value == nil {
		return nil
	}
	return *value
}
