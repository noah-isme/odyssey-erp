package distribution

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	accountingmoney "github.com/odyssey-erp/odyssey-erp/internal/accounting/money"
)

// Service handles business logic for distribution planning
type Service struct {
	repo Repository
}

// NewService creates a new distribution service
func NewService(db *pgxpool.Pool) *Service {
	return &Service{
		repo: NewRepository(db),
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// PLANNING CONFIGURATION
// ═══════════════════════════════════════════════════════════════════════════

// SetupPlanningHorizon creates a new planning time window for a warehouse
func (s *Service) SetupPlanningHorizon(ctx context.Context, input CreatePlanningHorizonInput) (*PlanningHorizon, error) {
	if input.WarehouseID == 0 {
		return nil, fmt.Errorf("warehouse ID is required")
	}

	horizonID, err := s.repo.CreatePlanningHorizon(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to create planning horizon: %w", err)
	}

	return s.repo.GetPlanningHorizon(ctx, horizonID)
}

// AddPlanningRule adds a constraint for load planning
func (s *Service) AddPlanningRule(ctx context.Context, input CreatePlanningRuleInput) (*PlanningRule, error) {
	if input.RuleName == "" {
		return nil, fmt.Errorf("rule name is required")
	}
	if input.RuleType != RuleTypeCapacity && input.RuleType != RuleTypeWeight &&
		input.RuleType != RuleTypeTimeWindow && input.RuleType != RuleTypeVehicleType &&
		input.RuleType != RuleTypeCustom {
		return nil, fmt.Errorf("invalid rule type")
	}

	ruleID, err := s.repo.CreatePlanningRule(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to create planning rule: %w", err)
	}

	// TODO: Query and return created rule
	_ = ruleID
	return nil, nil
}

// ═══════════════════════════════════════════════════════════════════════════
// LOAD PLANNING & CONSOLIDATION
// ═══════════════════════════════════════════════════════════════════════════

// CreateLoad creates a new load (consolidated shipments)
func (s *Service) CreateLoad(ctx context.Context, input CreateLoadInput) (*Load, error) {
	if input.OriginWarehouseID == 0 {
		return nil, fmt.Errorf("origin warehouse is required")
	}
	if input.DestinationWarehouseID == nil && input.DestinationCity == "" {
		return nil, fmt.Errorf("destination warehouse or city is required")
	}

	loadID, err := s.repo.CreateLoad(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to create load: %w", err)
	}

	return s.repo.GetLoad(ctx, loadID)
}

// AddShipmentToLoad consolidates a shipment into a load
func (s *Service) AddShipmentToLoad(ctx context.Context, loadID int64, shipmentID int64, productID int64, quantity interface{}, weightKg *interface{}, volumeCbm *interface{}) error {
	load, err := s.repo.GetLoad(ctx, loadID)
	if err != nil {
		return fmt.Errorf("load not found: %w", err)
	}

	if load.Status != LoadStatusDraft && load.Status != LoadStatusPlanned {
		return fmt.Errorf("can only add items to DRAFT or PLANNED loads, current status: %s", load.Status)
	}

	// TODO: Validate quantity, weight, volume
	// TODO: Check planning rules (capacity, weight limits)
	// TODO: Update load totals

	input := AddLoadItemInput{
		CompanyID:  load.CompanyID,
		LoadID:     loadID,
		ShipmentID: &shipmentID,
		ProductID:  productID,
		Quantity:   quantity,
		WeightKg:   weightKg,
		VolumeCbm:  volumeCbm,
	}

	_, err = s.repo.AddLoadItem(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to add shipment to load: %w", err)
	}

	return nil
}

// ValidateLoadAgainstRules checks if load complies with all planning rules
func (s *Service) ValidateLoadAgainstRules(ctx context.Context, loadID int64, warehouseID int64) (bool, []string, error) {
	load, err := s.repo.GetLoad(ctx, loadID)
	if err != nil {
		return false, nil, fmt.Errorf("load not found: %w", err)
	}

	// TODO: Query planning rules for warehouse
	// TODO: Check each rule:
	// - Capacity: total_items <= max_items_per_load
	// - Weight: total_weight_kg <= max_load_weight_kg
	// - Volume: total_volume_cbm <= max_load_volume_cbm
	// - Time Window: planned_pickup within time_window
	// - Vehicle Type: if specified, vehicle type matches
	// - Custom: evaluate custom_rule_expression
	// TODO: Return violations list

	_ = load
	_ = warehouseID
	return true, []string{}, nil
}

// DispatchLoad assigns transport and transitions to DISPATCHED
func (s *Service) DispatchLoad(ctx context.Context, loadID int64, vehicleID *int64, driverID *int64, carrierID *int64, carrierService *string) error {
	load, err := s.repo.GetLoad(ctx, loadID)
	if err != nil {
		return fmt.Errorf("load not found: %w", err)
	}

	if load.Status != LoadStatusReady {
		return fmt.Errorf("can only dispatch READY loads, current status: %s", load.Status)
	}

	// Validate transport assignment: must have vehicle+driver XOR carrier+service
	hasOwnTransport := (vehicleID != nil && driverID != nil)
	hasCarrierService := (carrierID != nil && carrierService != nil)

	if !hasOwnTransport && !hasCarrierService {
		return fmt.Errorf("must assign either vehicle+driver or carrier+service")
	}
	if hasOwnTransport && hasCarrierService {
		return fmt.Errorf("cannot assign both vehicle+driver and carrier+service")
	}

	err = s.repo.UpdateLoadDispatch(ctx, loadID, vehicleID, driverID, carrierID, carrierService)
	if err != nil {
		return fmt.Errorf("failed to dispatch load: %w", err)
	}

	err = s.repo.UpdateLoadStatus(ctx, loadID, LoadStatusDispatched)
	if err != nil {
		return fmt.Errorf("failed to update load status: %w", err)
	}

	// TODO: Create associated trip if internal fleet
	// TODO: Update vehicle and driver status to IN_USE
	// TODO: Send notifications to driver/carrier
	// TODO: Record audit log entry

	return nil
}

// ═══════════════════════════════════════════════════════════════════════════
// ROUTE OPTIMIZATION & SEQUENCING
// ═══════════════════════════════════════════════════════════════════════════

// PlanDeliveryRoute creates an optimized delivery route for a load
func (s *Service) PlanDeliveryRoute(ctx context.Context, input CreateRouteInput) (*DeliveryRoute, error) {
	if input.LoadID == 0 {
		return nil, fmt.Errorf("load ID is required")
	}

	load, err := s.repo.GetLoad(ctx, input.LoadID)
	if err != nil {
		return nil, fmt.Errorf("load not found: %w", err)
	}

	if load.Status != LoadStatusPlanned && load.Status != LoadStatusReady {
		return nil, fmt.Errorf("can only create routes for PLANNED or READY loads, current status: %s", load.Status)
	}

	routeID, err := s.repo.CreateRoute(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to create route: %w", err)
	}

	return s.repo.GetRoute(ctx, routeID)
}

// AddDeliveryStop adds a pickup or delivery stop to a route
func (s *Service) AddDeliveryStop(ctx context.Context, input AddRouteStopInput) (*RouteStop, error) {
	route, err := s.repo.GetRoute(ctx, input.RouteID)
	if err != nil {
		return nil, fmt.Errorf("route not found: %w", err)
	}

	if route.Status != RouteStatusDraft && route.Status != RouteStatusOptimized {
		return nil, fmt.Errorf("can only add stops to DRAFT or OPTIMIZED routes, current status: %s", route.Status)
	}

	if input.StopType != StopTypeWarehouse && input.StopType != StopTypeCustomer && input.StopType != StopTypeDeliveryPoint {
		return nil, fmt.Errorf("invalid stop type")
	}

	// Get existing stops to validate sequence
	existingStops, err := s.repo.GetRouteStops(ctx, input.RouteID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch route stops: %w", err)
	}

	if input.StopSequence <= len(existingStops) {
		return nil, fmt.Errorf("stop sequence must be > number of existing stops")
	}

	stopID, err := s.repo.AddRouteStop(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to add route stop: %w", err)
	}

	// TODO: Query and return the created stop
	_ = stopID
	return nil, nil
}

// OptimizeRoute runs route optimization algorithm
func (s *Service) OptimizeRoute(ctx context.Context, routeID int64) error {
	route, err := s.repo.GetRoute(ctx, routeID)
	if err != nil {
		return fmt.Errorf("route not found: %w", err)
	}

	if route.Status != RouteStatusDraft {
		return fmt.Errorf("can only optimize DRAFT routes, current status: %s", route.Status)
	}

	// TODO: Implement route optimization algorithm:
	// 1. Fetch all stops for this route
	// 2. Calculate distances between all stop pairs
	// 3. Apply TSP (Traveling Salesman Problem) algorithm
	// 4. Resequence stops for optimal distance
	// 5. Update stop sequence numbers
	// 6. Calculate total_distance_km and estimated_duration_minutes
	// 7. Calculate optimization_score (efficiency metric)
	// 8. Update route status to OPTIMIZED

	err = s.repo.UpdateRouteStatus(ctx, routeID, RouteStatusOptimized)
	if err != nil {
		return fmt.Errorf("failed to update route status: %w", err)
	}

	// TODO: Record optimization result in audit log
	// TODO: Send notification to planner with results

	return nil
}

// ApproveRoute approves an optimized route for execution
func (s *Service) ApproveRoute(ctx context.Context, routeID int64) error {
	route, err := s.repo.GetRoute(ctx, routeID)
	if err != nil {
		return fmt.Errorf("route not found: %w", err)
	}

	if route.Status != RouteStatusOptimized {
		return fmt.Errorf("can only approve OPTIMIZED routes, current status: %s", route.Status)
	}

	err = s.repo.UpdateRouteStatus(ctx, routeID, RouteStatusApproved)
	if err != nil {
		return fmt.Errorf("failed to approve route: %w", err)
	}

	// TODO: Record audit log entry
	return nil
}

// ═══════════════════════════════════════════════════════════════════════════
// TRANSFER ORDER MANAGEMENT
// ═══════════════════════════════════════════════════════════════════════════

// CreateTransferOrder creates an inter-warehouse transfer
func (s *Service) CreateTransferOrder(ctx context.Context, input CreateTransferOrderInput) (*TransferOrder, error) {
	if input.FromWarehouseID == input.ToWarehouseID {
		return nil, fmt.Errorf("from and to warehouses cannot be the same")
	}

	transferID, err := s.repo.CreateTransferOrder(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to create transfer order: %w", err)
	}

	return s.repo.GetTransferOrder(ctx, transferID)
}

// AddTransferItem adds a product to a transfer order
func (s *Service) AddTransferItem(ctx context.Context, transferID int64, productID int64, quantity interface{}, lotNumber string, serialNumbers []string) error {
	transfer, err := s.repo.GetTransferOrder(ctx, transferID)
	if err != nil {
		return fmt.Errorf("transfer order not found: %w", err)
	}

	if transfer.Status != TransferStatusDraft {
		return fmt.Errorf("can only add items to DRAFT transfers, current status: %s", transfer.Status)
	}

	input := AddTransferLineInput{
		CompanyID:         transfer.CompanyID,
		TransferOrderID:   transferID,
		ProductID:         productID,
		QuantityRequested: quantity,
		LotNumber:         lotNumber,
		SerialNumbers:     serialNumbers,
	}

	_, err = s.repo.AddTransferLine(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to add transfer item: %w", err)
	}

	return nil
}

// ApproveTransfer approves a transfer order for shipment
func (s *Service) ApproveTransfer(ctx context.Context, transferID int64) error {
	transfer, err := s.repo.GetTransferOrder(ctx, transferID)
	if err != nil {
		return fmt.Errorf("transfer order not found: %w", err)
	}

	if transfer.Status != TransferStatusDraft {
		return fmt.Errorf("can only approve DRAFT transfers, current status: %s", transfer.Status)
	}

	// TODO: Validate all items have quantity_requested > 0
	// TODO: Check inventory availability at from_warehouse

	err = s.repo.UpdateTransferStatus(ctx, transferID, TransferStatusApproved)
	if err != nil {
		return fmt.Errorf("failed to approve transfer: %w", err)
	}

	// TODO: Reserve inventory at from_warehouse
	// TODO: Record audit log entry

	return nil
}

// DispatchTransfer assigns transport and transitions to DISPATCHED
func (s *Service) DispatchTransfer(ctx context.Context, transferID int64, vehicleID *int64, driverID *int64, carrierID *int64) error {
	transfer, err := s.repo.GetTransferOrder(ctx, transferID)
	if err != nil {
		return fmt.Errorf("transfer order not found: %w", err)
	}

	if transfer.Status != TransferStatusApproved {
		return fmt.Errorf("can only dispatch APPROVED transfers, current status: %s", transfer.Status)
	}

	// Validate transport assignment: must have vehicle+driver XOR carrier
	hasOwnTransport := (vehicleID != nil && driverID != nil)
	hasCarrier := (carrierID != nil)

	if !hasOwnTransport && !hasCarrier {
		return fmt.Errorf("must assign either vehicle+driver or carrier")
	}
	if hasOwnTransport && hasCarrier {
		return fmt.Errorf("cannot assign both vehicle+driver and carrier")
	}

	err = s.repo.UpdateTransferDispatch(ctx, transferID, vehicleID, driverID, carrierID)
	if err != nil {
		return fmt.Errorf("failed to dispatch transfer: %w", err)
	}

	err = s.repo.UpdateTransferStatus(ctx, transferID, TransferStatusDispatched)
	if err != nil {
		return fmt.Errorf("failed to update transfer status: %w", err)
	}

	// TODO: Post inventory as in-transit
	// TODO: Create trip if internal fleet
	// TODO: Send notifications

	return nil
}

// ReceiveTransfer marks transfer as received at destination warehouse
func (s *Service) ReceiveTransfer(ctx context.Context, transferID int64, receivedAt time.Time) error {
	transfer, err := s.repo.GetTransferOrder(ctx, transferID)
	if err != nil {
		return fmt.Errorf("transfer order not found: %w", err)
	}

	if transfer.Status != TransferStatusInTransit {
		return fmt.Errorf("can only receive IN_TRANSIT transfers, current status: %s", transfer.Status)
	}

	// TODO: Update all transfer lines with quantity_received
	// TODO: Post inventory movement to destination warehouse
	// TODO: Clear in-transit inventory

	err = s.repo.UpdateTransferStatus(ctx, transferID, TransferStatusReceived)
	if err != nil {
		return fmt.Errorf("failed to mark transfer received: %w", err)
	}

	// TODO: Record audit log entry
	// TODO: Post GL entry for landed cost

	return nil
}

// ═══════════════════════════════════════════════════════════════════════════
// PLANNING ANALYTICS & REPORTING
// ═══════════════════════════════════════════════════════════════════════════

// GetLoadUtilization returns capacity utilization metrics for a load
func (s *Service) GetLoadUtilization(ctx context.Context, loadID int64) (*LoadUtilization, error) {
	load, err := s.repo.GetLoad(ctx, loadID)
	if err != nil {
		return nil, fmt.Errorf("load not found: %w", err)
	}

	// TODO: Query planning rules for origin warehouse to get max capacity/weight
	// TODO: Calculate utilization percentages

	utilization := &LoadUtilization{
		LoadID:           loadID,
		CurrentWeight:    load.TotalWeightKg,
		CurrentVolume:    load.TotalVolumeCbm,
		CurrentItems:     load.TotalItems,
		CapacityUtilization: 0,   // TODO: Calculate
		WeightUtilization:   0,   // TODO: Calculate
		VolumeUtilization:   0,   // TODO: Calculate
	}

	return utilization, nil
}

type LoadUtilization struct {
	LoadID                   int64
	CurrentWeight            *accountingmoney.Money
	CurrentVolume            *accountingmoney.Money
	CurrentItems             *int
	MaxWeight                *accountingmoney.Money
	MaxVolume                *accountingmoney.Money
	MaxItems                 *int
	CapacityUtilization      float64 // 0-100%
	WeightUtilization        float64 // 0-100%
	VolumeUtilization        float64 // 0-100%
}

// ListPendingTransfers returns all transfers awaiting approval or dispatch
func (s *Service) ListPendingTransfers(ctx context.Context, companyID int64) ([]*TransferOrder, error) {
	// TODO: Query transfers where status IN ('DRAFT', 'APPROVED') for company
	_ = companyID
	return nil, fmt.Errorf("not implemented")
}

// GetRouteMetrics returns performance metrics for a delivery route
func (s *Service) GetRouteMetrics(ctx context.Context, routeID int64) (*RouteMetrics, error) {
	route, err := s.repo.GetRoute(ctx, routeID)
	if err != nil {
		return nil, fmt.Errorf("route not found: %w", err)
	}

	// TODO: Query route stops and calculate metrics
	// - Total stops
	// - Stops completed
	// - Average time between stops
	// - Route efficiency vs. optimized path

	metrics := &RouteMetrics{
		RouteID:           routeID,
		TotalDistance:     route.TotalDistanceKm,
		EstimatedDuration: route.EstimatedDurationMinutes,
		OptimizationScore: route.OptimizationScore,
	}

	return metrics, nil
}

type RouteMetrics struct {
	RouteID           int64
	TotalStops        int
	CompletedStops    int
	TotalDistance     *float64
	EstimatedDuration *int
	ActualDuration    *int
	OptimizationScore *accountingmoney.Money
	EfficiencyRating  float64 // 0-100%
}
