package distribution

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository defines data access for distribution planning
type Repository interface {
	// Planning Horizons
	CreatePlanningHorizon(ctx context.Context, input CreatePlanningHorizonInput) (int64, error)
	GetPlanningHorizon(ctx context.Context, horizonID int64) (*PlanningHorizon, error)
	ListPlanningHorizons(ctx context.Context, companyID int64) ([]*PlanningHorizon, error)
	UpdatePlanningHorizonStatus(ctx context.Context, horizonID int64, status HorizonStatus) error

	// Planning Rules
	CreatePlanningRule(ctx context.Context, input CreatePlanningRuleInput) (int64, error)
	ListPlanningRules(ctx context.Context, warehouseID int64) ([]*PlanningRule, error)
	UpdateRuleActive(ctx context.Context, ruleID int64, isActive bool) error

	// Loads
	CreateLoad(ctx context.Context, input CreateLoadInput) (int64, error)
	GetLoad(ctx context.Context, loadID int64) (*Load, error)
	ListLoads(ctx context.Context, companyID int64, status *LoadStatus) ([]*Load, error)
	UpdateLoadStatus(ctx context.Context, loadID int64, status LoadStatus) error
	UpdateLoadDispatch(ctx context.Context, loadID int64, vehicleID *int64, driverID *int64, carrierID *int64, carrierService *string) error

	// Load Items
	AddLoadItem(ctx context.Context, input AddLoadItemInput) (int64, error)
	GetLoadItems(ctx context.Context, loadID int64) ([]*LoadItem, error)

	// Delivery Routes
	CreateRoute(ctx context.Context, input CreateRouteInput) (int64, error)
	GetRoute(ctx context.Context, routeID int64) (*DeliveryRoute, error)
	ListRoutes(ctx context.Context, companyID int64, status *RouteStatus) ([]*DeliveryRoute, error)
	UpdateRouteStatus(ctx context.Context, routeID int64, status RouteStatus) error

	// Route Stops
	AddRouteStop(ctx context.Context, input AddRouteStopInput) (int64, error)
	GetRouteStops(ctx context.Context, routeID int64) ([]*RouteStop, error)
	UpdateStopActualTimes(ctx context.Context, stopID int64, arrivedAt *interface{}, departedAt *interface{}) error

	// Transfer Orders
	CreateTransferOrder(ctx context.Context, input CreateTransferOrderInput) (int64, error)
	GetTransferOrder(ctx context.Context, transferID int64) (*TransferOrder, error)
	ListTransferOrders(ctx context.Context, companyID int64, status *TransferStatus) ([]*TransferOrder, error)
	UpdateTransferStatus(ctx context.Context, transferID int64, status TransferStatus) error
	UpdateTransferDispatch(ctx context.Context, transferID int64, vehicleID *int64, driverID *int64, carrierID *int64) error

	// Transfer Order Lines
	AddTransferLine(ctx context.Context, input AddTransferLineInput) (int64, error)
	GetTransferLines(ctx context.Context, transferID int64) ([]*TransferOrderLine, error)
	UpdateTransferLineReceipt(ctx context.Context, lineID int64, quantityReceived interface{}) error
}

// DistributionRepository implements Repository interface
type DistributionRepository struct {
	db *pgxpool.Pool
}

// NewRepository creates a new distribution repository
func NewRepository(db *pgxpool.Pool) *DistributionRepository {
	return &DistributionRepository{db: db}
}

// ═══════════════════════════════════════════════════════════════════════════
// PLANNING HORIZON OPERATIONS
// ═══════════════════════════════════════════════════════════════════════════

type CreatePlanningHorizonInput struct {
	CompanyID         int64
	WarehouseID       int64
	PlanningStartDate interface{} // time.Time
	PlanningEndDate   interface{} // time.Time
	FrozenUntilDate   *interface{}
	CreatedBy         int64
}

func (r *DistributionRepository) CreatePlanningHorizon(ctx context.Context, input CreatePlanningHorizonInput) (int64, error) {
	query := `
		INSERT INTO planning_horizons (company_id, warehouse_id, planning_start_date, planning_end_date, frozen_until_date, status, created_by, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NOW(), NOW())
		RETURNING id
	`
	var horizonID int64
	err := r.db.QueryRow(ctx, query,
		input.CompanyID,
		input.WarehouseID,
		input.PlanningStartDate,
		input.PlanningEndDate,
		input.FrozenUntilDate,
		"ACTIVE",
		input.CreatedBy,
	).Scan(&horizonID)
	return horizonID, err
}

func (r *DistributionRepository) GetPlanningHorizon(ctx context.Context, horizonID int64) (*PlanningHorizon, error) {
	query := `SELECT id, company_id, warehouse_id, planning_start_date, planning_end_date, frozen_until_date, status, created_by, created_at, updated_at FROM planning_horizons WHERE id = $1`
	var h PlanningHorizon
	err := r.db.QueryRow(ctx, query, horizonID).Scan(
		&h.ID, &h.CompanyID, &h.WarehouseID, &h.PlanningStartDate, &h.PlanningEndDate,
		&h.FrozenUntilDate, &h.Status, &h.CreatedBy, &h.CreatedAt, &h.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &h, nil
}

func (r *DistributionRepository) ListPlanningHorizons(ctx context.Context, companyID int64) ([]*PlanningHorizon, error) {
	query := `SELECT id, company_id, warehouse_id, planning_start_date, planning_end_date, frozen_until_date, status, created_by, created_at, updated_at FROM planning_horizons WHERE company_id = $1 AND status = 'ACTIVE' ORDER BY planning_start_date DESC`
	rows, err := r.db.Query(ctx, query, companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var horizons []*PlanningHorizon
	for rows.Next() {
		var h PlanningHorizon
		err := rows.Scan(
			&h.ID, &h.CompanyID, &h.WarehouseID, &h.PlanningStartDate, &h.PlanningEndDate,
			&h.FrozenUntilDate, &h.Status, &h.CreatedBy, &h.CreatedAt, &h.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		horizons = append(horizons, &h)
	}
	return horizons, rows.Err()
}

func (r *DistributionRepository) UpdatePlanningHorizonStatus(ctx context.Context, horizonID int64, status HorizonStatus) error {
	query := `UPDATE planning_horizons SET status = $1, updated_at = NOW() WHERE id = $2`
	_, err := r.db.Exec(ctx, query, string(status), horizonID)
	return err
}

// ═══════════════════════════════════════════════════════════════════════════
// PLANNING RULE OPERATIONS
// ═══════════════════════════════════════════════════════════════════════════

type CreatePlanningRuleInput struct {
	CompanyID               int64
	WarehouseID             int64
	RuleName                string
	RuleType                RuleType
	MaxLoadWeightKg         *interface{}
	MaxLoadVolumeCbm        *interface{}
	MaxItemsPerLoad         *int
	TimeWindowStart         *interface{}
	TimeWindowEnd           *interface{}
	VehicleTypeRequired     string
	CustomRuleExpression    string
	Priority                int
	CreatedBy               int64
}

func (r *DistributionRepository) CreatePlanningRule(ctx context.Context, input CreatePlanningRuleInput) (int64, error) {
	query := `
		INSERT INTO planning_rules (company_id, warehouse_id, rule_name, rule_type, max_load_weight_kg, max_load_volume_cbm, max_items_per_load, time_window_start, time_window_end, vehicle_type_required, custom_rule_expression, priority, is_active, created_by, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, TRUE, $13, NOW(), NOW())
		RETURNING id
	`
	var ruleID int64
	err := r.db.QueryRow(ctx, query,
		input.CompanyID, input.WarehouseID, input.RuleName, string(input.RuleType),
		input.MaxLoadWeightKg, input.MaxLoadVolumeCbm, input.MaxItemsPerLoad,
		input.TimeWindowStart, input.TimeWindowEnd, input.VehicleTypeRequired,
		input.CustomRuleExpression, input.Priority, input.CreatedBy,
	).Scan(&ruleID)
	return ruleID, err
}

func (r *DistributionRepository) ListPlanningRules(ctx context.Context, warehouseID int64) ([]*PlanningRule, error) {
	query := `SELECT id, company_id, warehouse_id, rule_name, rule_type, max_load_weight_kg, max_load_volume_cbm, max_items_per_load, time_window_start, time_window_end, vehicle_type_required, custom_rule_expression, priority, is_active, created_by, created_at, updated_at FROM planning_rules WHERE warehouse_id = $1 AND is_active = TRUE ORDER BY priority ASC`
	rows, err := r.db.Query(ctx, query, warehouseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []*PlanningRule
	for rows.Next() {
		var rule PlanningRule
		err := rows.Scan(
			&rule.ID, &rule.CompanyID, &rule.WarehouseID, &rule.RuleName, &rule.RuleType,
			&rule.MaxLoadWeightKg, &rule.MaxLoadVolumeCbm, &rule.MaxItemsPerLoad,
			&rule.TimeWindowStart, &rule.TimeWindowEnd, &rule.VehicleTypeRequired,
			&rule.CustomRuleExpression, &rule.Priority, &rule.IsActive,
			&rule.CreatedBy, &rule.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		rules = append(rules, &rule)
	}
	return rules, rows.Err()
}

func (r *DistributionRepository) UpdateRuleActive(ctx context.Context, ruleID int64, isActive bool) error {
	query := `UPDATE planning_rules SET is_active = $1, updated_at = NOW() WHERE id = $2`
	_, err := r.db.Exec(ctx, query, isActive, ruleID)
	return err
}

// ═══════════════════════════════════════════════════════════════════════════
// LOAD OPERATIONS
// ═══════════════════════════════════════════════════════════════════════════

type CreateLoadInput struct {
	CompanyID             int64
	OriginWarehouseID     int64
	DestinationWarehouseID *int64
	DestinationAddress    string
	DestinationCity       string
	DestinationCountry    string
	PlannedPickupDate     *interface{}
	PlannedDeliveryDate   *interface{}
	CreatedBy             int64
}

func (r *DistributionRepository) CreateLoad(ctx context.Context, input CreateLoadInput) (int64, error) {
	query := `
		INSERT INTO loads (company_id, load_number, origin_warehouse_id, destination_warehouse_id, destination_address, destination_city, destination_country, planned_pickup_date, planned_delivery_date, status, created_by, created_at, updated_at)
		VALUES ($1, 'LOAD-' || TO_CHAR(NOW(), 'YYYYMMDD') || '-' || LPAD(NEXTVAL('load_sequence')::TEXT, 4, '0'), $2, $3, $4, $5, $6, $7, $8, 'DRAFT', $9, NOW(), NOW())
		RETURNING id
	`
	var loadID int64
	err := r.db.QueryRow(ctx, query,
		input.CompanyID, input.OriginWarehouseID, input.DestinationWarehouseID,
		input.DestinationAddress, input.DestinationCity, input.DestinationCountry,
		input.PlannedPickupDate, input.PlannedDeliveryDate, input.CreatedBy,
	).Scan(&loadID)
	return loadID, err
}

func (r *DistributionRepository) GetLoad(ctx context.Context, loadID int64) (*Load, error) {
	query := `SELECT id, company_id, load_number, origin_warehouse_id, destination_warehouse_id, destination_address, destination_city, destination_country, planned_pickup_date, planned_delivery_date, actual_pickup_date, actual_delivery_date, vehicle_id, driver_id, carrier_id, carrier_service_type, status, total_weight_kg, total_volume_cbm, created_by, created_at, updated_at FROM loads WHERE id = $1`
	var l Load
	err := r.db.QueryRow(ctx, query, loadID).Scan(
		&l.ID, &l.CompanyID, &l.LoadNumber, &l.OriginWarehouseID, &l.DestinationWarehouseID,
		&l.DestinationAddress, &l.DestinationCity, &l.DestinationCountry,
		&l.PlannedPickupDate, &l.PlannedDeliveryDate, &l.ActualDispatchAt, &l.ActualDeliveryAt,
		&l.VehicleID, &l.DriverID, &l.CarrierID, &l.CarrierServiceType,
		&l.Status, &l.TotalWeightKg, &l.TotalVolumeCbm, &l.CreatedBy, &l.CreatedAt, &l.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &l, nil
}

func (r *DistributionRepository) ListLoads(ctx context.Context, companyID int64, status *LoadStatus) ([]*Load, error) {
	query := `SELECT id, company_id, load_number, origin_warehouse_id, destination_warehouse_id, destination_address, destination_city, destination_country, planned_pickup_date, planned_delivery_date, actual_pickup_date, actual_delivery_date, vehicle_id, driver_id, carrier_id, carrier_service_type, status, total_weight_kg, total_volume_cbm, created_by, created_at, updated_at FROM loads WHERE company_id = $1 AND (status = $2 OR $2 IS NULL) ORDER BY created_at DESC`
	rows, err := r.db.Query(ctx, query, companyID, (*string)(status))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var loads []*Load
	for rows.Next() {
		var l Load
		err := rows.Scan(
			&l.ID, &l.CompanyID, &l.LoadNumber, &l.OriginWarehouseID, &l.DestinationWarehouseID,
			&l.DestinationAddress, &l.DestinationCity, &l.DestinationCountry,
			&l.PlannedPickupDate, &l.PlannedDeliveryDate, &l.ActualDispatchAt, &l.ActualDeliveryAt,
			&l.VehicleID, &l.DriverID, &l.CarrierID, &l.CarrierServiceType,
			&l.Status, &l.TotalWeightKg, &l.TotalVolumeCbm, &l.CreatedBy, &l.CreatedAt, &l.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		loads = append(loads, &l)
	}
	return loads, rows.Err()
}

func (r *DistributionRepository) UpdateLoadStatus(ctx context.Context, loadID int64, status LoadStatus) error {
	query := `UPDATE loads SET status = $1, updated_at = NOW() WHERE id = $2`
	_, err := r.db.Exec(ctx, query, string(status), loadID)
	return err
}

func (r *DistributionRepository) UpdateLoadDispatch(ctx context.Context, loadID int64, vehicleID *int64, driverID *int64, carrierID *int64, carrierService *string) error {
	query := `UPDATE loads SET vehicle_id = $1, driver_id = $2, carrier_id = $3, carrier_service_type = $4, status = 'CONFIRMED', updated_at = NOW() WHERE id = $5`
	_, err := r.db.Exec(ctx, query, vehicleID, driverID, carrierID, carrierService, loadID)
	return err
}

// ═══════════════════════════════════════════════════════════════════════════
// LOAD ITEM OPERATIONS
// ═══════════════════════════════════════════════════════════════════════════

type AddLoadItemInput struct {
	CompanyID  int64
	LoadID     int64
	ShipmentID *int64
	ProductID  int64
	Quantity   interface{}
	WeightKg   *interface{}
	VolumeCbm  *interface{}
}

func (r *DistributionRepository) AddLoadItem(ctx context.Context, input AddLoadItemInput) (int64, error) {
	query := `INSERT INTO load_items (company_id, load_id, shipment_id, product_id, quantity, weight_kg, volume_cbm, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6, $7, NOW(), NOW()) RETURNING id`
	var itemID int64
	err := r.db.QueryRow(ctx, query,
		input.CompanyID, input.LoadID, input.ShipmentID, input.ProductID,
		input.Quantity, input.WeightKg, input.VolumeCbm,
	).Scan(&itemID)
	return itemID, err
}

func (r *DistributionRepository) GetLoadItems(ctx context.Context, loadID int64) ([]*LoadItem, error) {
	query := `SELECT id, company_id, load_id, shipment_id, product_id, quantity, weight_kg, volume_cbm, created_at, updated_at FROM load_items WHERE load_id = $1 ORDER BY created_at ASC`
	rows, err := r.db.Query(ctx, query, loadID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []*LoadItem
	for rows.Next() {
		var item LoadItem
		err := rows.Scan(
			&item.ID, &item.CompanyID, &item.LoadID, &item.ShipmentID, &item.ProductID,
			&item.Quantity, &item.WeightKg, &item.VolumeCbm, &item.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		items = append(items, &item)
	}
	return items, rows.Err()
}

// ═══════════════════════════════════════════════════════════════════════════
// DELIVERY ROUTE OPERATIONS
// ═══════════════════════════════════════════════════════════════════════════

type CreateRouteInput struct {
	CompanyID              int64
	LoadID                 int64
	TotalDistanceKm        *float64
	EstimatedDurationMinutes *int
	OptimizationScore      *interface{}
	CreatedBy              int64
}

func (r *DistributionRepository) CreateRoute(ctx context.Context, input CreateRouteInput) (int64, error) {
	query := `
		INSERT INTO delivery_routes (company_id, load_id, route_number, total_distance_km, estimated_duration_minutes, optimization_score, status, created_by, created_at, updated_at)
		VALUES ($1, $2, 'ROUTE-' || TO_CHAR(NOW(), 'YYYYMMDD') || '-' || LPAD(NEXTVAL('route_sequence')::TEXT, 4, '0'), $3, $4, $5, 'PLANNED', $6, NOW(), NOW())
		RETURNING id
	`
	var routeID int64
	err := r.db.QueryRow(ctx, query,
		input.CompanyID, input.LoadID, input.TotalDistanceKm, input.EstimatedDurationMinutes, input.OptimizationScore, input.CreatedBy,
	).Scan(&routeID)
	return routeID, err
}

func (r *DistributionRepository) GetRoute(ctx context.Context, routeID int64) (*DeliveryRoute, error) {
	query := `SELECT id, company_id, load_id, route_number, total_distance_km, estimated_duration_minutes, actual_duration_minutes, optimization_score, status, created_by, created_at, updated_at FROM delivery_routes WHERE id = $1`
	var route DeliveryRoute
	err := r.db.QueryRow(ctx, query, routeID).Scan(
		&route.ID, &route.CompanyID, &route.LoadID, &route.RouteNumber, &route.TotalDistanceKm,
		&route.EstimatedDurationMinutes, &route.OptimizationScore,
		&route.Status, &route.CreatedBy, &route.CreatedAt, &route.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &route, nil
}

func (r *DistributionRepository) ListRoutes(ctx context.Context, companyID int64, status *RouteStatus) ([]*DeliveryRoute, error) {
	query := `SELECT id, company_id, load_id, route_number, total_distance_km, estimated_duration_minutes, actual_duration_minutes, optimization_score, status, created_by, created_at, updated_at FROM delivery_routes WHERE company_id = $1 AND (status = $2 OR $2 IS NULL) ORDER BY created_at DESC`
	rows, err := r.db.Query(ctx, query, companyID, (*string)(status))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var routes []*DeliveryRoute
	for rows.Next() {
		var route DeliveryRoute
		err := rows.Scan(
			&route.ID, &route.CompanyID, &route.LoadID, &route.RouteNumber, &route.TotalDistanceKm,
			&route.EstimatedDurationMinutes, &route.OptimizationScore,
			&route.Status, &route.CreatedBy, &route.CreatedAt, &route.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		routes = append(routes, &route)
	}
	return routes, rows.Err()
}

func (r *DistributionRepository) UpdateRouteStatus(ctx context.Context, routeID int64, status RouteStatus) error {
	query := `UPDATE delivery_routes SET status = $1, updated_at = NOW() WHERE id = $2`
	_, err := r.db.Exec(ctx, query, string(status), routeID)
	return err
}

// ═══════════════════════════════════════════════════════════════════════════
// ROUTE STOP OPERATIONS
// ═══════════════════════════════════════════════════════════════════════════

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
	PlannedArrivalTime   *interface{}
	PlannedDepartureTime *interface{}
}

func (r *DistributionRepository) AddRouteStop(ctx context.Context, input AddRouteStopInput) (int64, error) {
	query := `INSERT INTO route_stops (company_id, route_id, stop_sequence, stop_type, warehouse_id, customer_id, customer_address, customer_city, location_lat, location_lon, contact_name, contact_phone, planned_arrival_time, planned_departure_time, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, NOW(), NOW()) RETURNING id`
	var stopID int64
	err := r.db.QueryRow(ctx, query,
		input.CompanyID, input.RouteID, input.StopSequence, string(input.StopType),
		input.WarehouseID, input.CustomerID, input.CustomerAddress, input.CustomerCity,
		input.LocationLat, input.LocationLon, input.ContactName, input.ContactPhone,
		input.PlannedArrivalTime, input.PlannedDepartureTime,
	).Scan(&stopID)
	return stopID, err
}

func (r *DistributionRepository) GetRouteStops(ctx context.Context, routeID int64) ([]*RouteStop, error) {
	query := `SELECT id, company_id, route_id, stop_sequence, stop_type, warehouse_id, customer_id, customer_address, customer_city, location_lat, location_lon, contact_name, contact_phone, planned_arrival_time, planned_departure_time, actual_arrival_at, actual_departure_at, created_at, updated_at FROM route_stops WHERE route_id = $1 ORDER BY stop_sequence ASC`
	rows, err := r.db.Query(ctx, query, routeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stops []*RouteStop
	for rows.Next() {
		var stop RouteStop
		err := rows.Scan(
			&stop.ID, &stop.CompanyID, &stop.RouteID, &stop.StopSequence, &stop.StopType,
			&stop.WarehouseID, &stop.CustomerID, &stop.CustomerAddress, &stop.CustomerCity,
			&stop.LocationLat, &stop.LocationLon, &stop.ContactName, &stop.ContactPhone,
			&stop.PlannedArrivalTime, &stop.PlannedDepartureTime, &stop.ActualArrivalAt, &stop.ActualDepartureAt,
			&stop.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		stops = append(stops, &stop)
	}
	return stops, rows.Err()
}

func (r *DistributionRepository) UpdateStopActualTimes(ctx context.Context, stopID int64, arrivedAt *interface{}, departedAt *interface{}) error {
	query := `UPDATE route_stops SET actual_arrival_at = $1, actual_departure_at = $2, updated_at = NOW() WHERE id = $3`
	_, err := r.db.Exec(ctx, query, arrivedAt, departedAt, stopID)
	return err
}

// ═══════════════════════════════════════════════════════════════════════════
// TRANSFER ORDER OPERATIONS
// ═══════════════════════════════════════════════════════════════════════════

type CreateTransferOrderInput struct {
	CompanyID          int64
	FromWarehouseID    int64
	ToWarehouseID      int64
	PlannedDispatchDate *interface{}
	PlannedArrivalDate  *interface{}
	CreatedBy          int64
}

func (r *DistributionRepository) CreateTransferOrder(ctx context.Context, input CreateTransferOrderInput) (int64, error) {
	query := `
		INSERT INTO transfer_orders (company_id, transfer_number, from_warehouse_id, to_warehouse_id, planned_dispatch_date, planned_arrival_date, status, created_by, created_at, updated_at)
		VALUES ($1, 'TRANSFER-' || TO_CHAR(NOW(), 'YYYYMMDD') || '-' || LPAD(NEXTVAL('transfer_sequence')::TEXT, 4, '0'), $2, $3, $4, $5, 'DRAFT', $6, NOW(), NOW())
		RETURNING id
	`
	var transferID int64
	err := r.db.QueryRow(ctx, query,
		input.CompanyID, input.FromWarehouseID, input.ToWarehouseID,
		input.PlannedDispatchDate, input.PlannedArrivalDate, input.CreatedBy,
	).Scan(&transferID)
	return transferID, err
}

func (r *DistributionRepository) GetTransferOrder(ctx context.Context, transferID int64) (*TransferOrder, error) {
	query := `SELECT id, company_id, transfer_number, from_warehouse_id, to_warehouse_id, planned_dispatch_date, planned_arrival_date, actual_dispatch_date, actual_arrival_date, vehicle_id, driver_id, carrier_id, status, created_by, created_at, updated_at FROM transfer_orders WHERE id = $1`
	var t TransferOrder
	err := r.db.QueryRow(ctx, query, transferID).Scan(
		&t.ID, &t.CompanyID, &t.TransferNumber, &t.FromWarehouseID, &t.ToWarehouseID,
		&t.PlannedDispatchDate, &t.PlannedArrivalDate, &t.VehicleID, &t.DriverID, &t.CarrierID, &t.Status, &t.CreatedBy, &t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *DistributionRepository) ListTransferOrders(ctx context.Context, companyID int64, status *TransferStatus) ([]*TransferOrder, error) {
	query := `SELECT id, company_id, transfer_number, from_warehouse_id, to_warehouse_id, planned_dispatch_date, planned_arrival_date, actual_dispatch_date, actual_arrival_date, vehicle_id, driver_id, carrier_id, status, created_by, created_at, updated_at FROM transfer_orders WHERE company_id = $1 AND (status = $2 OR $2 IS NULL) ORDER BY created_at DESC`
	rows, err := r.db.Query(ctx, query, companyID, (*string)(status))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var transfers []*TransferOrder
	for rows.Next() {
		var t TransferOrder
		err := rows.Scan(
			&t.ID, &t.CompanyID, &t.TransferNumber, &t.FromWarehouseID, &t.ToWarehouseID,
			&t.PlannedDispatchDate, &t.PlannedArrivalDate, &t.VehicleID, &t.DriverID, &t.CarrierID, &t.Status, &t.CreatedBy, &t.CreatedAt, &t.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		transfers = append(transfers, &t)
	}
	return transfers, rows.Err()
}

func (r *DistributionRepository) UpdateTransferStatus(ctx context.Context, transferID int64, status TransferStatus) error {
	query := `UPDATE transfer_orders SET status = $1, updated_at = NOW() WHERE id = $2`
	_, err := r.db.Exec(ctx, query, string(status), transferID)
	return err
}

func (r *DistributionRepository) UpdateTransferDispatch(ctx context.Context, transferID int64, vehicleID *int64, driverID *int64, carrierID *int64) error {
	query := `UPDATE transfer_orders SET vehicle_id = $1, driver_id = $2, carrier_id = $3, status = 'CONFIRMED', updated_at = NOW() WHERE id = $4`
	_, err := r.db.Exec(ctx, query, vehicleID, driverID, carrierID, transferID)
	return err
}

// ═══════════════════════════════════════════════════════════════════════════
// TRANSFER ORDER LINE OPERATIONS
// ═══════════════════════════════════════════════════════════════════════════

type AddTransferLineInput struct {
	CompanyID         int64
	TransferOrderID   int64
	ProductID         int64
	QuantityRequested interface{}
	LotNumber         string
	SerialNumbers     []string
}

func (r *DistributionRepository) AddTransferLine(ctx context.Context, input AddTransferLineInput) (int64, error) {
	query := `INSERT INTO transfer_order_lines (company_id, transfer_order_id, product_id, quantity_requested, quantity_received, lot_number, serial_numbers, created_at, updated_at) VALUES ($1, $2, $3, $4, NULL, $5, $6, NOW(), NOW()) RETURNING id`
	var lineID int64
	err := r.db.QueryRow(ctx, query,
		input.CompanyID, input.TransferOrderID, input.ProductID, input.QuantityRequested,
		input.LotNumber, input.SerialNumbers,
	).Scan(&lineID)
	return lineID, err
}

func (r *DistributionRepository) GetTransferLines(ctx context.Context, transferID int64) ([]*TransferOrderLine, error) {
	query := `SELECT id, company_id, transfer_order_id, product_id, quantity_requested, quantity_received, lot_number, serial_numbers, created_at, updated_at FROM transfer_order_lines WHERE transfer_order_id = $1 ORDER BY created_at ASC`
	rows, err := r.db.Query(ctx, query, transferID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var lines []*TransferOrderLine
	for rows.Next() {
		var line TransferOrderLine
		err := rows.Scan(
			&line.ID, &line.CompanyID, &line.TransferOrderID, &line.ProductID,
			&line.QuantityRequested, &line.QuantityReceived, &line.LotNumber, &line.SerialNumbers,
			&line.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		lines = append(lines, &line)
	}
	return lines, rows.Err()
}

func (r *DistributionRepository) UpdateTransferLineReceipt(ctx context.Context, lineID int64, quantityReceived interface{}) error {
	query := `UPDATE transfer_order_lines SET quantity_received = $1, updated_at = NOW() WHERE id = $2`
	_, err := r.db.Exec(ctx, query, quantityReceived, lineID)
	return err
}
