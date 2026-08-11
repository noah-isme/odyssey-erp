package distribution

import (
	"context"
	"fmt"
	"time"

	accountingmoney "github.com/odyssey-erp/odyssey-erp/internal/accounting/money"
)

type fakeDistributionRepository struct {
	nextID    int64
	horizons  map[int64]*PlanningHorizon
	rules     map[int64]*PlanningRule
	loads     map[int64]*Load
	items     map[int64][]*LoadItem
	routes    map[int64]*DeliveryRoute
	stops     map[int64][]*RouteStop
	transfers map[int64]*TransferOrder
	lines     map[int64][]*TransferOrderLine

	routeOptimizationErr error
}

func newFakeDistributionRepository() *fakeDistributionRepository {
	return &fakeDistributionRepository{
		nextID:    1,
		horizons:  map[int64]*PlanningHorizon{},
		rules:     map[int64]*PlanningRule{},
		loads:     map[int64]*Load{},
		items:     map[int64][]*LoadItem{},
		routes:    map[int64]*DeliveryRoute{},
		stops:     map[int64][]*RouteStop{},
		transfers: map[int64]*TransferOrder{},
		lines:     map[int64][]*TransferOrderLine{},
	}
}

func (r *fakeDistributionRepository) id() int64 {
	id := r.nextID
	r.nextID++
	return id
}

func (r *fakeDistributionRepository) CreatePlanningHorizon(_ context.Context, input CreatePlanningHorizonInput) (int64, error) {
	id := r.id()
	r.horizons[id] = &PlanningHorizon{ID: id, CompanyID: input.CompanyID, WarehouseID: input.WarehouseID, PlanningStartDate: input.PlanningStartDate, PlanningEndDate: input.PlanningEndDate, FrozenUntilDate: input.FrozenUntilDate, Notes: input.Notes, Status: HorizonStatusActive, CreatedBy: input.CreatedBy}
	return id, nil
}

func (r *fakeDistributionRepository) GetPlanningHorizon(_ context.Context, id int64) (*PlanningHorizon, error) {
	value, ok := r.horizons[id]
	if !ok {
		return nil, fmt.Errorf("horizon not found")
	}
	return value, nil
}

func (r *fakeDistributionRepository) ListPlanningHorizons(_ context.Context, companyID int64) ([]*PlanningHorizon, error) {
	result := make([]*PlanningHorizon, 0)
	for _, horizon := range r.horizons {
		if horizon.CompanyID == companyID {
			result = append(result, horizon)
		}
	}
	return result, nil
}

func (r *fakeDistributionRepository) UpdatePlanningHorizonStatus(_ context.Context, id int64, status HorizonStatus) error {
	r.horizons[id].Status = status
	return nil
}

func (r *fakeDistributionRepository) CreatePlanningRule(_ context.Context, input CreatePlanningRuleInput) (int64, error) {
	id := r.id()
	r.rules[id] = &PlanningRule{ID: id, CompanyID: input.CompanyID, WarehouseID: input.WarehouseID, RuleName: input.RuleName, RuleType: input.RuleType, MaxLoadWeightKg: input.MaxLoadWeightKg, MaxLoadVolumeCbm: input.MaxLoadVolumeCbm, MaxItemsPerLoad: input.MaxItemsPerLoad, TimeWindowStart: input.TimeWindowStart, TimeWindowEnd: input.TimeWindowEnd, Priority: input.Priority, IsActive: true, CreatedBy: input.CreatedBy}
	return id, nil
}

func (r *fakeDistributionRepository) GetPlanningRule(_ context.Context, id int64) (*PlanningRule, error) {
	value, ok := r.rules[id]
	if !ok {
		return nil, fmt.Errorf("rule not found")
	}
	return value, nil
}

func (r *fakeDistributionRepository) ListPlanningRules(_ context.Context, warehouseID int64) ([]*PlanningRule, error) {
	result := make([]*PlanningRule, 0)
	for _, rule := range r.rules {
		if rule.WarehouseID == warehouseID && rule.IsActive {
			result = append(result, rule)
		}
	}
	return result, nil
}

func (r *fakeDistributionRepository) UpdateRuleActive(_ context.Context, id int64, active bool) error {
	r.rules[id].IsActive = active
	return nil
}

func (r *fakeDistributionRepository) CreateLoad(_ context.Context, input CreateLoadInput) (int64, error) {
	id := r.id()
	r.loads[id] = &Load{ID: id, CompanyID: input.CompanyID, LoadNumber: fmt.Sprintf("LOAD-%d", id), Status: LoadStatusDraft, OriginWarehouseID: input.OriginWarehouseID, DestinationWarehouseID: input.DestinationWarehouseID, DestinationAddress: input.DestinationAddress, DestinationCity: input.DestinationCity, DestinationCountry: input.DestinationCountry, PlannedPickupDate: input.PlannedPickupDate, PlannedDeliveryDate: input.PlannedDeliveryDate, Notes: input.Notes, CreatedBy: input.CreatedBy}
	return id, nil
}

func (r *fakeDistributionRepository) GetLoad(_ context.Context, id int64) (*Load, error) {
	value, ok := r.loads[id]
	if !ok {
		return nil, fmt.Errorf("load not found")
	}
	return value, nil
}

func (r *fakeDistributionRepository) ListLoads(_ context.Context, companyID int64, status *LoadStatus) ([]*Load, error) {
	result := make([]*Load, 0)
	for _, load := range r.loads {
		if load.CompanyID == companyID && (status == nil || load.Status == *status) {
			result = append(result, load)
		}
	}
	return result, nil
}

func (r *fakeDistributionRepository) UpdateLoadStatus(_ context.Context, id int64, status LoadStatus) error {
	load := r.loads[id]
	load.Status = status
	now := time.Now().UTC()
	if status == LoadStatusDispatched || status == LoadStatusInTransit {
		load.ActualDispatchAt = &now
	}
	if status == LoadStatusDelivered {
		load.ActualDeliveryAt = &now
	}
	return nil
}

func (r *fakeDistributionRepository) UpdateLoadDispatch(_ context.Context, id int64, vehicleID, driverID, carrierID *int64, carrierService *string) error {
	load := r.loads[id]
	load.VehicleID, load.DriverID, load.CarrierID, load.CarrierServiceType = vehicleID, driverID, carrierID, carrierService
	return nil
}

func (r *fakeDistributionRepository) AddLoadItem(_ context.Context, input AddLoadItemInput) (int64, error) {
	id := r.id()
	r.items[input.LoadID] = append(r.items[input.LoadID], &LoadItem{ID: id, CompanyID: input.CompanyID, LoadID: input.LoadID, ShipmentID: input.ShipmentID, ProductID: input.ProductID, Quantity: input.Quantity, WeightKg: input.WeightKg, VolumeCbm: input.VolumeCbm})
	return id, nil
}

func (r *fakeDistributionRepository) GetLoadItems(_ context.Context, loadID int64) ([]*LoadItem, error) {
	return r.items[loadID], nil
}

func (r *fakeDistributionRepository) CreateRoute(_ context.Context, input CreateRouteInput) (int64, error) {
	id := r.id()
	r.routes[id] = &DeliveryRoute{ID: id, CompanyID: input.CompanyID, LoadID: input.LoadID, RouteNumber: fmt.Sprintf("ROUTE-%d", id), Status: RouteStatusDraft, TotalDistanceKm: input.TotalDistanceKm, EstimatedDurationMinutes: input.EstimatedDurationMinutes, OptimizationScore: input.OptimizationScore, CreatedBy: input.CreatedBy}
	return id, nil
}

func (r *fakeDistributionRepository) GetRoute(_ context.Context, id int64) (*DeliveryRoute, error) {
	value, ok := r.routes[id]
	if !ok {
		return nil, fmt.Errorf("route not found")
	}
	return value, nil
}

func (r *fakeDistributionRepository) ListRoutes(_ context.Context, companyID int64, status *RouteStatus) ([]*DeliveryRoute, error) {
	result := make([]*DeliveryRoute, 0)
	for _, route := range r.routes {
		if route.CompanyID == companyID && (status == nil || route.Status == *status) {
			result = append(result, route)
		}
	}
	return result, nil
}

func (r *fakeDistributionRepository) UpdateRouteStatus(_ context.Context, id int64, status RouteStatus) error {
	r.routes[id].Status = status
	return nil
}

func (r *fakeDistributionRepository) UpdateRouteOptimization(_ context.Context, routeID int64, input RouteOptimizationUpdate) error {
	if r.routeOptimizationErr != nil {
		return r.routeOptimizationErr
	}
	route, ok := r.routes[routeID]
	if !ok {
		return fmt.Errorf("route not found")
	}
	if route.CompanyID != input.CompanyID {
		return fmt.Errorf("route company does not match optimization company")
	}
	if route.Status != RouteStatusDraft {
		return fmt.Errorf("can only optimize DRAFT routes, current status: %s", route.Status)
	}
	stops := r.stops[routeID]
	if len(stops) != len(input.OrderedStopIDs) {
		return fmt.Errorf("route stop set changed during optimization")
	}
	byID := make(map[int64]*RouteStop, len(stops))
	for _, stop := range stops {
		byID[stop.ID] = stop
	}
	ordered := make([]*RouteStop, len(input.OrderedStopIDs))
	seen := make(map[int64]struct{}, len(input.OrderedStopIDs))
	for i, stopID := range input.OrderedStopIDs {
		stop, ok := byID[stopID]
		if !ok {
			return fmt.Errorf("route stop %d was not found for route", stopID)
		}
		if _, ok := seen[stopID]; ok {
			return fmt.Errorf("route stop IDs must be unique")
		}
		seen[stopID] = struct{}{}
		ordered[i] = stop
	}

	for i, stop := range ordered {
		stop.StopSequence = i + 1
	}
	route.TotalDistanceKm = copyFloat64(input.TotalDistanceKm)
	route.EstimatedDurationMinutes = copyInt(input.EstimatedDurationMinutes)
	route.OptimizationScore = copyMoney(input.OptimizationScore)
	route.Status = RouteStatusOptimized
	return nil
}

func copyFloat64(value *float64) *float64 {
	if value == nil {
		return nil
	}
	result := *value
	return &result
}

func copyInt(value *int) *int {
	if value == nil {
		return nil
	}
	result := *value
	return &result
}

func copyMoney(value *accountingmoney.Money) *accountingmoney.Money {
	if value == nil {
		return nil
	}
	result := *value
	return &result
}

func (r *fakeDistributionRepository) AddRouteStop(_ context.Context, input AddRouteStopInput) (int64, error) {
	id := r.id()
	r.stops[input.RouteID] = append(r.stops[input.RouteID], &RouteStop{ID: id, CompanyID: input.CompanyID, RouteID: input.RouteID, StopSequence: input.StopSequence, StopType: input.StopType, WarehouseID: input.WarehouseID, CustomerID: input.CustomerID, CustomerAddress: input.CustomerAddress, CustomerCity: input.CustomerCity, LocationLat: input.LocationLat, LocationLon: input.LocationLon, PlannedArrivalTime: input.PlannedArrivalTime, PlannedDepartureTime: input.PlannedDepartureTime, Notes: input.Notes})
	return id, nil
}

func (r *fakeDistributionRepository) GetRouteStop(_ context.Context, id int64) (*RouteStop, error) {
	for _, stops := range r.stops {
		for _, stop := range stops {
			if stop.ID == id {
				return stop, nil
			}
		}
	}
	return nil, fmt.Errorf("stop not found")
}

func (r *fakeDistributionRepository) GetRouteStops(_ context.Context, routeID int64) ([]*RouteStop, error) {
	return r.stops[routeID], nil
}

func (r *fakeDistributionRepository) UpdateStopActualTimes(_ context.Context, id int64, arrivedAt, departedAt *time.Time) error {
	stop, err := r.GetRouteStop(context.Background(), id)
	if err != nil {
		return err
	}
	stop.ActualArrivalAt, stop.ActualDepartureAt = arrivedAt, departedAt
	return nil
}

func (r *fakeDistributionRepository) CreateTransferOrder(_ context.Context, input CreateTransferOrderInput) (int64, error) {
	id := r.id()
	r.transfers[id] = &TransferOrder{ID: id, CompanyID: input.CompanyID, TransferNumber: fmt.Sprintf("TRANSFER-%d", id), Status: TransferStatusDraft, FromWarehouseID: input.FromWarehouseID, ToWarehouseID: input.ToWarehouseID, PlannedDispatchDate: input.PlannedDispatchDate, PlannedArrivalDate: input.PlannedArrivalDate, Notes: input.Notes, CreatedBy: input.CreatedBy}
	return id, nil
}

func (r *fakeDistributionRepository) GetTransferOrder(_ context.Context, id int64) (*TransferOrder, error) {
	value, ok := r.transfers[id]
	if !ok {
		return nil, fmt.Errorf("transfer not found")
	}
	return value, nil
}

func (r *fakeDistributionRepository) ListTransferOrders(_ context.Context, companyID int64, status *TransferStatus) ([]*TransferOrder, error) {
	result := make([]*TransferOrder, 0)
	for _, transfer := range r.transfers {
		if transfer.CompanyID == companyID && (status == nil || transfer.Status == *status) {
			result = append(result, transfer)
		}
	}
	return result, nil
}

func (r *fakeDistributionRepository) UpdateTransferStatus(_ context.Context, id int64, status TransferStatus) error {
	r.transfers[id].Status = status
	return nil
}

func (r *fakeDistributionRepository) UpdateTransferDispatch(_ context.Context, id int64, vehicleID, driverID, carrierID *int64) error {
	transfer := r.transfers[id]
	transfer.VehicleID, transfer.DriverID, transfer.CarrierID = vehicleID, driverID, carrierID
	return nil
}

func (r *fakeDistributionRepository) AddTransferLine(_ context.Context, input AddTransferLineInput) (int64, error) {
	id := r.id()
	r.lines[input.TransferOrderID] = append(r.lines[input.TransferOrderID], &TransferOrderLine{ID: id, CompanyID: input.CompanyID, TransferOrderID: input.TransferOrderID, ProductID: input.ProductID, QuantityRequested: input.QuantityRequested, LotNumber: input.LotNumber, SerialNumbers: input.SerialNumbers})
	return id, nil
}

func (r *fakeDistributionRepository) GetTransferLines(_ context.Context, transferID int64) ([]*TransferOrderLine, error) {
	return r.lines[transferID], nil
}

func (r *fakeDistributionRepository) UpdateTransferLineReceipt(_ context.Context, id int64, quantityReceived accountingmoney.Money) error {
	for _, lines := range r.lines {
		for _, line := range lines {
			if line.ID == id {
				line.QuantityReceived = &quantityReceived
				return nil
			}
		}
	}
	return fmt.Errorf("line not found")
}

type fakeShipmentGateway struct {
	nextID   int64
	statuses map[int64]string
	lines    map[int64][]ShipmentLine
	created  []ShipmentCreateInput
}

func newFakeShipmentGateway() *fakeShipmentGateway {
	return &fakeShipmentGateway{nextID: 100, statuses: map[int64]string{}, lines: map[int64][]ShipmentLine{}}
}

func (g *fakeShipmentGateway) CreateShipment(_ context.Context, input ShipmentCreateInput) (int64, error) {
	id := g.nextID
	g.nextID++
	g.statuses[id] = "DRAFT"
	g.created = append(g.created, input)
	return id, nil
}

func (g *fakeShipmentGateway) AddShipmentLine(_ context.Context, input ShipmentLineInput) error {
	g.lines[input.ShipmentID] = append(g.lines[input.ShipmentID], ShipmentLine{ProductID: input.ProductID, Quantity: input.Quantity})
	return nil
}

func (g *fakeShipmentGateway) GetShipmentLines(_ context.Context, shipmentID int64) ([]ShipmentLine, error) {
	return g.lines[shipmentID], nil
}

func (g *fakeShipmentGateway) DispatchShipment(_ context.Context, shipmentID int64, _, _, _ *int64, _ *string) error {
	g.statuses[shipmentID] = "DISPATCHED"
	return nil
}

func (g *fakeShipmentGateway) MarkShipmentInTransit(_ context.Context, shipmentID int64) error {
	g.statuses[shipmentID] = "IN_TRANSIT"
	return nil
}

func (g *fakeShipmentGateway) MarkShipmentDelivered(_ context.Context, shipmentID int64, _ time.Time) error {
	g.statuses[shipmentID] = "DELIVERED"
	return nil
}

type fakeInventoryGateway struct {
	adjustments []InventoryAdjustmentInput
}

func (g *fakeInventoryGateway) PostAdjustment(_ context.Context, input InventoryAdjustmentInput) error {
	g.adjustments = append(g.adjustments, input)
	return nil
}
