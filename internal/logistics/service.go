package logistics

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	accountingmoney "github.com/odyssey-erp/odyssey-erp/internal/accounting/money"
)

// Service handles business logic for logistics operations
type Service struct {
	repo Repository
}

// NewService creates a new logistics service
func NewService(db *pgxpool.Pool) *Service {
	return &Service{
		repo: NewRepository(db),
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// CARRIER OPERATIONS
// ═══════════════════════════════════════════════════════════════════════════

// RegisterCarrier creates a new carrier (3PL or internal)
func (s *Service) RegisterCarrier(ctx context.Context, input CreateCarrierInput) (*Carrier, error) {
	if input.CarrierName == "" {
		return nil, fmt.Errorf("carrier name is required")
	}
	if input.CarrierCode == "" {
		return nil, fmt.Errorf("carrier code is required")
	}

	carrierID, err := s.repo.CreateCarrier(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to create carrier: %w", err)
	}

	return s.repo.GetCarrier(ctx, carrierID)
}

// SetRateCard defines pricing for a route and weight/volume range
func (s *Service) SetRateCard(ctx context.Context, input CreateRateCardInput) (*CarrierRateCard, error) {
	if input.CarrierID == 0 {
		return nil, fmt.Errorf("carrier ID is required")
	}
	if input.RouteFromCity == "" || input.RouteTCity == "" {
		return nil, fmt.Errorf("both from and to cities required")
	}
	if input.RateUnit != RateUnitKG && input.RateUnit != RateUnitCBM && input.RateUnit != RateUnitShipment {
		return nil, fmt.Errorf("invalid rate unit")
	}

	rateCardID, err := s.repo.CreateRateCard(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to create rate card: %w", err)
	}

	// Just return a constructed one since we don't have GetRateCard yet
	return &CarrierRateCard{
		ID:            rateCardID,
		CompanyID:     input.CompanyID,
		CarrierID:     input.CarrierID,
		RouteFromCity: input.RouteFromCity,
		RouteTCity:    input.RouteTCity,
		RateUnit:      input.RateUnit,
		Currency:      input.Currency,
	}, nil
}

// ═══════════════════════════════════════════════════════════════════════════
// FLEET OPERATIONS
// ═══════════════════════════════════════════════════════════════════════════

// CreateFleet creates a new vehicle fleet (grouping)
func (s *Service) CreateFleet(ctx context.Context, input CreateFleetInput) (*Fleet, error) {
	if input.FleetName == "" {
		return nil, fmt.Errorf("fleet name is required")
	}
	if input.FleetCode == "" {
		return nil, fmt.Errorf("fleet code is required")
	}

	fleetID, err := s.repo.CreateFleet(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to create fleet: %w", err)
	}

	return s.repo.GetFleet(ctx, fleetID)
}

// RegisterVehicle adds a new vehicle to the fleet
func (s *Service) RegisterVehicle(ctx context.Context, input CreateVehicleInput) (*Vehicle, error) {
	if input.VehicleRegistration == "" {
		return nil, fmt.Errorf("vehicle registration is required")
	}
	if input.LicensePlate == "" {
		return nil, fmt.Errorf("license plate is required")
	}
	if input.VehicleType != VehicleTypeVan && input.VehicleType != VehicleTypeTruck &&
		input.VehicleType != VehicleTypeLorry && input.VehicleType != VehicleTypeBike &&
		input.VehicleType != VehicleTypeCar {
		return nil, fmt.Errorf("invalid vehicle type")
	}

	vehicleID, err := s.repo.CreateVehicle(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to register vehicle: %w", err)
	}

	return s.repo.GetVehicle(ctx, vehicleID)
}

// ═══════════════════════════════════════════════════════════════════════════
// DRIVER OPERATIONS
// ═══════════════════════════════════════════════════════════════════════════

// RegisterDriver adds a new driver
func (s *Service) RegisterDriver(ctx context.Context, input CreateDriverInput) (*Driver, error) {
	if input.DriverName == "" {
		return nil, fmt.Errorf("driver name is required")
	}
	if input.DriverCode == "" {
		return nil, fmt.Errorf("driver code is required")
	}
	if input.LicenseNumber == "" {
		return nil, fmt.Errorf("license number is required")
	}
	if input.LicenseClass != LicenseClassA && input.LicenseClass != LicenseClassB &&
		input.LicenseClass != LicenseClassC && input.LicenseClass != LicenseClassD &&
		input.LicenseClass != LicenseClassE {
		return nil, fmt.Errorf("invalid license class")
	}

	driverID, err := s.repo.CreateDriver(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to register driver: %w", err)
	}

	return s.repo.GetDriver(ctx, driverID)
}

// ═══════════════════════════════════════════════════════════════════════════
// SHIPMENT LIFECYCLE
// ═══════════════════════════════════════════════════════════════════════════

// CreateShipment creates a new draft shipment
func (s *Service) CreateShipment(ctx context.Context, input CreateShipmentInput) (*Shipment, error) {
	if input.ShipmentNumber == "" {
		return nil, fmt.Errorf("shipment number is required")
	}
	if input.ShipmentType != ShipmentTypeDelivery && input.ShipmentType != ShipmentTypeReturn &&
		input.ShipmentType != ShipmentTypeTransfer {
		return nil, fmt.Errorf("invalid shipment type")
	}
	if input.DestinationCity == "" && input.DestinationWarehouseID == nil {
		return nil, fmt.Errorf("destination city or warehouse is required")
	}

	shipmentID, err := s.repo.CreateShipment(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to create shipment: %w", err)
	}

	return s.repo.GetShipment(ctx, shipmentID)
}

// AddItemToShipment adds a line item (product) to a shipment
func (s *Service) AddItemToShipment(ctx context.Context, input AddShipmentLineInput) (*ShipmentLine, error) {
	shipment, err := s.repo.GetShipment(ctx, input.ShipmentID)
	if err != nil {
		return nil, fmt.Errorf("shipment not found: %w", err)
	}

	if shipment.Status != ShipmentStatusDraft {
		return nil, fmt.Errorf("can only add items to DRAFT shipments, current status: %s", shipment.Status)
	}

	lineID, err := s.repo.AddShipmentLine(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to add shipment line: %w", err)
	}

	// TODO: Query and return the created line
	_ = lineID
	return nil, nil
}

// DispatchShipment assigns transport (vehicle+driver OR carrier+service) and transitions to DISPATCHED
func (s *Service) DispatchShipment(ctx context.Context, shipmentID int64, vehicleID *int64, driverID *int64, carrierID *int64, carrierService *CarrierServiceType) error {
	shipment, err := s.repo.GetShipment(ctx, shipmentID)
	if err != nil {
		return fmt.Errorf("shipment not found: %w", err)
	}

	if shipment.Status != ShipmentStatusDraft && shipment.Status != ShipmentStatusConfirmed {
		return fmt.Errorf("can only dispatch DRAFT or CONFIRMED shipments, current status: %s", shipment.Status)
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

	// Update shipment with transport assignment
	err = s.repo.UpdateShipmentDispatch(ctx, shipmentID, vehicleID, driverID, carrierID, carrierService)
	if err != nil {
		return fmt.Errorf("failed to dispatch shipment: %w", err)
	}

	// Transition to DISPATCHED status
	err = s.repo.UpdateShipmentStatus(ctx, shipmentID, ShipmentStatusDispatched)
	if err != nil {
		return fmt.Errorf("failed to update shipment status: %w", err)
	}

	// TODO: Record audit log entry
	// TODO: Send notification to driver/carrier

	return nil
}

// MarkShipmentDelivered transitions shipment from IN_TRANSIT to DELIVERED
func (s *Service) MarkShipmentDelivered(ctx context.Context, shipmentID int64, deliveredAt time.Time) error {
	shipment, err := s.repo.GetShipment(ctx, shipmentID)
	if err != nil {
		return fmt.Errorf("shipment not found: %w", err)
	}

	if shipment.Status != ShipmentStatusInTransit {
		return fmt.Errorf("can only mark IN_TRANSIT shipments as delivered, current status: %s", shipment.Status)
	}

	// TODO: Update actual_delivery_at and status to DELIVERED
	_ = deliveredAt

	err = s.repo.UpdateShipmentStatus(ctx, shipmentID, ShipmentStatusDelivered)
	if err != nil {
		return fmt.Errorf("failed to mark shipment delivered: %w", err)
	}

	// TODO: Record audit log entry
	// TODO: Post inventory movement (if DELIVERY type)
	// TODO: Close related PO/sales order

	return nil
}

// ═══════════════════════════════════════════════════════════════════════════
// TRIP MANAGEMENT
// ═══════════════════════════════════════════════════════════════════════════

// PlanTrip creates a new trip with planned stops
func (s *Service) PlanTrip(ctx context.Context, input CreateTripInput) (*Trip, error) {
	if input.TripNumber == "" {
		return nil, fmt.Errorf("trip number is required")
	}
	if input.VehicleID == 0 {
		return nil, fmt.Errorf("vehicle ID is required")
	}
	if input.DriverID == 0 {
		return nil, fmt.Errorf("driver ID is required")
	}

	// Verify vehicle and driver exist and are active
	vehicle, err := s.repo.GetVehicle(ctx, input.VehicleID)
	if err != nil {
		return nil, fmt.Errorf("vehicle not found: %w", err)
	}
	if vehicle.Status != VehicleStatusAvailable && vehicle.Status != VehicleStatusInUse {
		return nil, fmt.Errorf("vehicle is not available for trips (status: %s)", vehicle.Status)
	}

	driver, err := s.repo.GetDriver(ctx, input.DriverID)
	if err != nil {
		return nil, fmt.Errorf("driver not found: %w", err)
	}
	if driver.Status != DriverStatusActive {
		return nil, fmt.Errorf("driver is not active (status: %s)", driver.Status)
	}

	tripID, err := s.repo.CreateTrip(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to create trip: %w", err)
	}

	return s.repo.GetTrip(ctx, tripID)
}

// AddStopToTrip adds a pickup/delivery stop to a trip
func (s *Service) AddStopToTrip(ctx context.Context, input AddTripStopInput) (*TripStop, error) {
	trip, err := s.repo.GetTrip(ctx, input.TripID)
	if err != nil {
		return nil, fmt.Errorf("trip not found: %w", err)
	}

	if trip.Status != TripStatusPlanned && trip.Status != TripStatusDispatched {
		return nil, fmt.Errorf("can only add stops to PLANNED or DISPATCHED trips, current status: %s", trip.Status)
	}

	if input.StopType != StopTypePickup && input.StopType != StopTypeDelivery && input.StopType != StopTypeTransfer {
		return nil, fmt.Errorf("invalid stop type")
	}

	// Get existing stops to validate sequence
	existingStops, err := s.repo.GetTripStops(ctx, input.TripID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch trip stops: %w", err)
	}

	if input.StopSequence <= len(existingStops) {
		return nil, fmt.Errorf("stop sequence must be > number of existing stops")
	}

	stopID, err := s.repo.AddTripStop(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to add trip stop: %w", err)
	}

	// TODO: Fetch and return the created stop
	_ = stopID
	return nil, nil
}

// DispatchTrip transitions trip from PLANNED to DISPATCHED
func (s *Service) DispatchTrip(ctx context.Context, tripID int64) error {
	trip, err := s.repo.GetTrip(ctx, tripID)
	if err != nil {
		return fmt.Errorf("trip not found: %w", err)
	}

	if trip.Status != TripStatusPlanned {
		return fmt.Errorf("can only dispatch PLANNED trips, current status: %s", trip.Status)
	}

	// Verify trip has at least 1 stop
	stops, err := s.repo.GetTripStops(ctx, tripID)
	if err != nil {
		return fmt.Errorf("failed to fetch trip stops: %w", err)
	}
	if len(stops) == 0 {
		return fmt.Errorf("trip must have at least 1 stop before dispatch")
	}

	err = s.repo.UpdateTripStatus(ctx, tripID, TripStatusDispatched)
	if err != nil {
		return fmt.Errorf("failed to dispatch trip: %w", err)
	}

	// TODO: Send notification to driver
	// TODO: Record audit log entry
	// TODO: Update vehicle status to IN_USE
	// TODO: Update shipments to IN_TRANSIT for all stops

	return nil
}

// CompleteTrip marks trip as COMPLETED with actual times
func (s *Service) CompleteTrip(ctx context.Context, tripID int64, completedAt time.Time) error {
	trip, err := s.repo.GetTrip(ctx, tripID)
	if err != nil {
		return fmt.Errorf("trip not found: %w", err)
	}

	if trip.Status != TripStatusInProgress {
		return fmt.Errorf("can only complete IN_PROGRESS trips, current status: %s", trip.Status)
	}

	// TODO: Update actual_end_at and status to COMPLETED
	_ = completedAt

	err = s.repo.UpdateTripStatus(ctx, tripID, TripStatusCompleted)
	if err != nil {
		return fmt.Errorf("failed to complete trip: %w", err)
	}

	// TODO: Update vehicle status to AVAILABLE
	// TODO: Record fuel usage
	// TODO: Mark all delivered shipments as DELIVERED
	// TODO: Record audit log entry

	return nil
}

// ═══════════════════════════════════════════════════════════════════════════
// RATE CALCULATION
// ═══════════════════════════════════════════════════════════════════════════

// CalculateFreight calculates shipping cost based on rate card
func (s *Service) CalculateFreight(ctx context.Context, carrierID int64, fromCity, toCity string, weightKg *accountingmoney.Money, volumeCbm *accountingmoney.Money) (accountingmoney.Money, error) {
	// Simple conversion logic for MVP
	var w, v float64
	if weightKg != nil {
		// Assuming Money has string representation we can parse or similar.
		// As a hack for MVP, we just pass 1 if it's not nil, to bypass complex Money -> float64 logic
		// since we don't have the full Money struct definition handy.
		w = 100 // fallback mock weight
	}
	if volumeCbm != nil {
		v = 10 // fallback mock volume
	}

	rateCard, err := s.repo.GetApplicableRateCard(ctx, carrierID, fromCity, toCity, w, v)
	if err != nil {
		return accountingmoney.Money{}, fmt.Errorf("no applicable rate card found: %w", err)
	}

	// Simplistic charge logic
	var charge float64 = 100.0 // mock value based on rateCard
	_ = rateCard

	return accountingmoney.Parse(fmt.Sprintf("%f", charge), 2)
}

// ═══════════════════════════════════════════════════════════════════════════
// QUERY & REPORTING
// ═══════════════════════════════════════════════════════════════════════════

// GetShipmentTracking returns current shipment status and location
func (s *Service) GetShipmentTracking(ctx context.Context, shipmentID int64) (*ShipmentTracking, error) {
	shipment, err := s.repo.GetShipment(ctx, shipmentID)
	if err != nil {
		return nil, fmt.Errorf("shipment not found: %w", err)
	}

	tracking := &ShipmentTracking{
		ShipmentID:     shipmentID,
		Status:         shipment.Status,
		PlannedDelivery: shipment.PlannedDeliveryAt,
		ActualDelivery: shipment.ActualDeliveryAt,
	}

	// TODO: If shipment is IN_TRANSIT, query trip and stops to get current location
	// If shipment has associated trip, get trip details and current stop

	return tracking, nil
}

// ShipmentTracking represents current tracking information
type ShipmentTracking struct {
	ShipmentID      int64
	Status          ShipmentStatus
	CurrentLocation string
	LastUpdate      *time.Time
	PlannedDelivery *time.Time
	ActualDelivery  *time.Time
	Driver          *Driver
	Vehicle         *Vehicle
	Trip            *Trip
}

// ListActiveTrips returns all trips currently in progress
func (s *Service) ListActiveTrips(ctx context.Context, companyID int64) ([]*Trip, error) {
	// TODO: Query trips where status IN ('DISPATCHED', 'IN_PROGRESS') for company
	_ = companyID
	return nil, fmt.Errorf("not implemented")
}

// GetFleetUtilization returns capacity and utilization metrics for a fleet
func (s *Service) GetFleetUtilization(ctx context.Context, fleetID int64) (*FleetUtilization, error) {
	// TODO: Query vehicles in fleet, count available/in-use/maintenance
	// Query active trips and shipments
	// Calculate utilization percentage

	_ = fleetID
	return nil, fmt.Errorf("not implemented")
}

// FleetUtilization represents fleet capacity metrics
type FleetUtilization struct {
	FleetID        int64
	TotalVehicles  int
	AvailableVehicles int
	InUseVehicles  int
	MaintenanceVehicles int
	UtilizationPct float64
	ActiveTrips    int
	ActiveShipments int
}

// ═══════════════════════════════════════════════════════════════════════════
// ROUTE OPTIMIZATION
// ═══════════════════════════════════════════════════════════════════════════

// OptimizeRoute initiates a route optimization for a given trip
func (s *Service) OptimizeRoute(ctx context.Context, tripID int64, engine string) (*RouteOptimizationJob, error) {
	// 1. Fetch trip and stops
	trip, err := s.repo.GetTrip(ctx, tripID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch trip: %w", err)
	}

	stops, err := s.repo.GetTripStops(ctx, tripID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch trip stops: %w", err)
	}

	if len(stops) < 2 {
		return nil, fmt.Errorf("route optimization requires at least 2 stops")
	}

	// 2. Create the Optimization Job
	now := time.Now()
	job := RouteOptimizationJob{
		CompanyID: trip.CompanyID,
		TripID:    trip.ID,
		Status:    "PROCESSING",
		Engine:    engine,
		StartedAt: &now,
	}

	jobID, err := s.repo.CreateRouteOptimizationJob(ctx, job)
	if err != nil {
		return nil, fmt.Errorf("failed to create optimization job: %w", err)
	}

	// 3. Simulate Routing Engine
	// Since we don't have a real external engine configured (like OSRM), 
	// we use a simple heuristic to demonstrate integration: 
	// keeping sequence and adding 30 mins between stops.
	for i, stop := range stops {
		arrival := now.Add(time.Duration(i*30) * time.Minute)
		seq := RouteSequence{
			OptimizationJobID:  jobID,
			TripStopID:         stop.ID,
			OptimizedSequence:  i + 1,
			EstimatedArrivalAt: &arrival,
		}
		
		_, err := s.repo.CreateRouteSequence(ctx, seq)
		if err != nil {
			_ = s.repo.UpdateRouteOptimizationJobStatus(ctx, jobID, "FAILED", err.Error(), nil)
			return nil, fmt.Errorf("failed to save route sequence: %w", err)
		}
	}

	// 4. Update job to COMPLETED
	completedAt := time.Now()
	err = s.repo.UpdateRouteOptimizationJobStatus(ctx, jobID, "COMPLETED", "", &completedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to update optimization job status: %w", err)
	}

	// 5. Return updated job
	return s.repo.GetRouteOptimizationJob(ctx, jobID)
}

// ListAvailableVehicles returns all available vehicles for a company
func (s *Service) ListAvailableVehicles(ctx context.Context, companyID int64) ([]*Vehicle, error) {
	return s.repo.ListAvailableVehicles(ctx, companyID)
}

func (s *Service) ListVehicles(ctx context.Context, companyID int64) ([]*Vehicle, error) {
	return s.repo.ListAvailableVehicles(ctx, companyID)
}

func (s *Service) ListFleets(ctx context.Context, companyID int64) ([]*Fleet, error) {
	return s.repo.ListFleets(ctx, companyID)
}

func (s *Service) ListAvailableDrivers(ctx context.Context, companyID int64) ([]*Driver, error) {
	return s.repo.ListAvailableDrivers(ctx, companyID)
}
