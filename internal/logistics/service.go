package logistics

import (
	"context"
	"fmt"
	"math/big"
	"strconv"
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

	quantity, err := accountingmoney.Parse(fmt.Sprintf("%f", input.Quantity), 4)
	if err != nil {
		return nil, fmt.Errorf("failed to map shipment quantity: %w", err)
	}
	var weight, volume *accountingmoney.Money
	if input.WeightKg != nil {
		value, parseErr := accountingmoney.Parse(fmt.Sprintf("%f", *input.WeightKg), 4)
		if parseErr != nil {
			return nil, fmt.Errorf("failed to map shipment weight: %w", parseErr)
		}
		weight = &value
	}
	if input.VolumeCbm != nil {
		value, parseErr := accountingmoney.Parse(fmt.Sprintf("%f", *input.VolumeCbm), 4)
		if parseErr != nil {
			return nil, fmt.Errorf("failed to map shipment volume: %w", parseErr)
		}
		volume = &value
	}
	return &ShipmentLine{
		ID:         lineID,
		CompanyID:  input.CompanyID,
		ShipmentID: input.ShipmentID,
		ProductID:  input.ProductID,
		Quantity:   quantity,
		WeightKg:   weight,
		VolumeCbm:  volume,
	}, nil
}

// GetShipmentLines exposes shipment data through the logistics service
// boundary without leaking SQLC rows to callers.
func (s *Service) GetShipmentLines(ctx context.Context, shipmentID int64) ([]*ShipmentLine, error) {
	if shipmentID == 0 {
		return nil, fmt.Errorf("shipment ID is required")
	}
	return s.repo.GetShipmentLines(ctx, shipmentID)
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

// MarkShipmentInTransit advances a dispatched shipment once the vehicle or
// carrier has physically started the journey.
func (s *Service) MarkShipmentInTransit(ctx context.Context, shipmentID int64) error {
	shipment, err := s.repo.GetShipment(ctx, shipmentID)
	if err != nil {
		return fmt.Errorf("shipment not found: %w", err)
	}
	if shipment.Status != ShipmentStatusDispatched {
		return fmt.Errorf("can only start DISPATCHED shipments, current status: %s", shipment.Status)
	}
	if err := s.repo.UpdateShipmentStatus(ctx, shipmentID, ShipmentStatusInTransit); err != nil {
		return fmt.Errorf("failed to mark shipment in transit: %w", err)
	}
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

	if deliveredAt.IsZero() {
		deliveredAt = time.Now().UTC()
	}

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

	stops, err := s.repo.GetTripStops(ctx, input.TripID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch created trip stop: %w", err)
	}
	for _, stop := range stops {
		if stop.ID == stopID {
			return stop, nil
		}
	}
	return nil, fmt.Errorf("created trip stop %d could not be loaded", stopID)
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

	if err := s.repo.UpdateVehicleStatus(ctx, trip.VehicleID, VehicleStatusInUse); err != nil {
		return fmt.Errorf("failed to mark vehicle in use: %w", err)
	}
	for _, stop := range stops {
		if stop.ShipmentID == nil {
			continue
		}
		shipment, err := s.repo.GetShipment(ctx, *stop.ShipmentID)
		if err != nil {
			return fmt.Errorf("failed to load shipment %d: %w", *stop.ShipmentID, err)
		}
		if shipment.Status == ShipmentStatusDispatched || shipment.Status == ShipmentStatusConfirmed {
			if err := s.repo.UpdateShipmentStatus(ctx, shipment.ID, ShipmentStatusInTransit); err != nil {
				return fmt.Errorf("failed to mark shipment %d in transit: %w", shipment.ID, err)
			}
		}
	}

	return nil
}

// CompleteTrip marks trip as COMPLETED with actual times
func (s *Service) CompleteTrip(ctx context.Context, tripID int64, completedAt time.Time) error {
	trip, err := s.repo.GetTrip(ctx, tripID)
	if err != nil {
		return fmt.Errorf("trip not found: %w", err)
	}

	if trip.Status != TripStatusInProgress && trip.Status != TripStatusDispatched {
		return fmt.Errorf("can only complete DISPATCHED or IN_PROGRESS trips, current status: %s", trip.Status)
	}

	if completedAt.IsZero() {
		completedAt = time.Now().UTC()
	}
	err = s.repo.UpdateTripStatusAt(ctx, tripID, TripStatusCompleted, completedAt)
	if err != nil {
		return fmt.Errorf("failed to complete trip: %w", err)
	}

	if err := s.repo.UpdateVehicleStatus(ctx, trip.VehicleID, VehicleStatusAvailable); err != nil {
		return fmt.Errorf("failed to mark vehicle available: %w", err)
	}
	stops, err := s.repo.GetTripStops(ctx, tripID)
	if err != nil {
		return fmt.Errorf("failed to fetch trip stops: %w", err)
	}
	seenShipments := make(map[int64]struct{})
	for _, stop := range stops {
		if stop.ShipmentID == nil {
			continue
		}
		shipmentID := *stop.ShipmentID
		if _, seen := seenShipments[shipmentID]; seen {
			continue
		}
		seenShipments[shipmentID] = struct{}{}
		shipment, err := s.repo.GetShipment(ctx, shipmentID)
		if err != nil {
			return fmt.Errorf("failed to load shipment %d: %w", shipmentID, err)
		}
		if shipment.Status == ShipmentStatusInTransit {
			if err := s.repo.UpdateShipmentStatus(ctx, shipmentID, ShipmentStatusDelivered); err != nil {
				return fmt.Errorf("failed to mark shipment %d delivered: %w", shipmentID, err)
			}
		}
	}

	return nil
}

// ═══════════════════════════════════════════════════════════════════════════
// RATE CALCULATION
// ═══════════════════════════════════════════════════════════════════════════

// CalculateFreight calculates shipping cost based on rate card
func (s *Service) CalculateFreight(ctx context.Context, carrierID int64, fromCity, toCity string, weightKg *accountingmoney.Money, volumeCbm *accountingmoney.Money) (accountingmoney.Money, error) {
	if carrierID == 0 || fromCity == "" || toCity == "" {
		return accountingmoney.Money{}, fmt.Errorf("carrier ID and route cities are required")
	}
	var w, v float64
	if weightKg != nil {
		parsed, err := strconv.ParseFloat(weightKg.Amount, 64)
		if err != nil {
			return accountingmoney.Money{}, fmt.Errorf("invalid weight: %w", err)
		}
		w = parsed
	}
	if volumeCbm != nil {
		parsed, err := strconv.ParseFloat(volumeCbm.Amount, 64)
		if err != nil {
			return accountingmoney.Money{}, fmt.Errorf("invalid volume: %w", err)
		}
		v = parsed
	}

	rateCard, err := s.repo.GetApplicableRateCard(ctx, carrierID, fromCity, toCity, w, v)
	if err != nil {
		return accountingmoney.Money{}, fmt.Errorf("no applicable rate card found: %w", err)
	}

	var charge accountingmoney.Money
	switch rateCard.RateUnit {
	case RateUnitKG:
		if weightKg == nil {
			return accountingmoney.Money{}, fmt.Errorf("weight is required for KG rate")
		}
		charge = multiplyMoney(*weightKg, rateCard.RatePerUnit)
	case RateUnitCBM:
		if volumeCbm == nil {
			return accountingmoney.Money{}, fmt.Errorf("volume is required for CBM rate")
		}
		charge = multiplyMoney(*volumeCbm, rateCard.RatePerUnit)
	case RateUnitShipment:
		charge = rateCard.RatePerUnit
	default:
		return accountingmoney.Money{}, fmt.Errorf("unsupported rate unit %q", rateCard.RateUnit)
	}
	if rateCard.MinimumCharge != nil && charge.Cmp(*rateCard.MinimumCharge) < 0 {
		charge = *rateCard.MinimumCharge
	}
	if rateCard.FuelSurchargePct != nil && rateCard.FuelSurchargePct.Cmp(accountingmoney.Must("0", rateCard.FuelSurchargePct.Scale)) > 0 {
		surcharge := percentageOf(charge, *rateCard.FuelSurchargePct)
		charge = charge.Add(surcharge)
	}
	return charge, nil
}

func multiplyMoney(a, b accountingmoney.Money) accountingmoney.Money {
	left, ok := new(big.Rat).SetString(a.Amount)
	if !ok {
		return accountingmoney.Money{}
	}
	right, ok := new(big.Rat).SetString(b.Amount)
	if !ok {
		return accountingmoney.Money{}
	}
	scale := a.Scale
	if b.Scale > scale {
		scale = b.Scale
	}
	return accountingmoney.Money{Amount: new(big.Rat).Mul(left, right).FloatString(scale), Scale: scale}
}

func percentageOf(value, percentage accountingmoney.Money) accountingmoney.Money {
	percent, ok := new(big.Rat).SetString(percentage.Amount)
	if !ok {
		return accountingmoney.Money{}
	}
	percent.Quo(percent, big.NewRat(100, 1))
	amount, ok := new(big.Rat).SetString(value.Amount)
	if !ok {
		return accountingmoney.Money{}
	}
	return accountingmoney.Money{Amount: new(big.Rat).Mul(amount, percent).FloatString(value.Scale), Scale: value.Scale}
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
		ShipmentID:      shipmentID,
		Status:          shipment.Status,
		PlannedDelivery: shipment.PlannedDeliveryAt,
		ActualDelivery:  shipment.ActualDeliveryAt,
	}
	if !shipment.UpdatedAt.IsZero() {
		lastUpdate := shipment.UpdatedAt
		tracking.LastUpdate = &lastUpdate
	}
	tracking.CurrentLocation = shipment.DestinationCity

	trips, err := s.repo.ListTrips(ctx, shipment.CompanyID, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to query shipment trips: %w", err)
	}
	for _, trip := range trips {
		stops, err := s.repo.GetTripStops(ctx, trip.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to query trip %d stops: %w", trip.ID, err)
		}
		var shipmentStop *TripStop
		for _, stop := range stops {
			if stop.ShipmentID != nil && *stop.ShipmentID == shipmentID {
				shipmentStop = stop
				break
			}
		}
		if shipmentStop == nil {
			continue
		}

		tracking.Trip = trip
		tracking.Driver, err = s.repo.GetDriver(ctx, trip.DriverID)
		if err != nil {
			return nil, fmt.Errorf("failed to load trip driver: %w", err)
		}
		tracking.Vehicle, err = s.repo.GetVehicle(ctx, trip.VehicleID)
		if err != nil {
			return nil, fmt.Errorf("failed to load trip vehicle: %w", err)
		}
		tracking.CurrentLocation = shipmentStop.LocationCity
		if tracking.CurrentLocation == "" {
			tracking.CurrentLocation = shipmentStop.LocationAddress
		}
		tracking.LastUpdate = latestStopUpdate(shipmentStop, trip, tracking.LastUpdate)
		break
	}

	return tracking, nil
}

// ShipmentTracking represents current tracking information
type ShipmentTracking struct {
	ShipmentID      int64          `json:"shipment_id"`
	Status          ShipmentStatus `json:"status"`
	CurrentLocation string         `json:"current_location"`
	LastUpdate      *time.Time     `json:"last_update,omitempty"`
	PlannedDelivery *time.Time     `json:"planned_delivery,omitempty"`
	ActualDelivery  *time.Time     `json:"actual_delivery,omitempty"`
	Driver          *Driver        `json:"driver,omitempty"`
	Vehicle         *Vehicle       `json:"vehicle,omitempty"`
	Trip            *Trip          `json:"trip,omitempty"`
}

// ListActiveTrips returns all trips currently in progress
func (s *Service) ListActiveTrips(ctx context.Context, companyID int64) ([]*Trip, error) {
	if companyID == 0 {
		return nil, fmt.Errorf("company ID is required")
	}
	dispatched := TripStatusDispatched
	inProgress := TripStatusInProgress
	first, err := s.repo.ListTrips(ctx, companyID, &dispatched)
	if err != nil {
		return nil, fmt.Errorf("failed to list dispatched trips: %w", err)
	}
	second, err := s.repo.ListTrips(ctx, companyID, &inProgress)
	if err != nil {
		return nil, fmt.Errorf("failed to list in-progress trips: %w", err)
	}
	return append(first, second...), nil
}

// GetFleetUtilization returns capacity and utilization metrics for a fleet
func (s *Service) GetFleetUtilization(ctx context.Context, fleetID int64) (*FleetUtilization, error) {
	if fleetID == 0 {
		return nil, fmt.Errorf("fleet ID is required")
	}
	fleet, err := s.repo.GetFleet(ctx, fleetID)
	if err != nil {
		return nil, fmt.Errorf("fleet not found: %w", err)
	}
	vehicles, err := s.repo.ListVehiclesByFleet(ctx, fleetID)
	if err != nil {
		return nil, fmt.Errorf("failed to list fleet vehicles: %w", err)
	}
	metrics := &FleetUtilization{FleetID: fleetID, TotalVehicles: len(vehicles)}
	for _, vehicle := range vehicles {
		switch vehicle.Status {
		case VehicleStatusAvailable:
			metrics.AvailableVehicles++
		case VehicleStatusInUse:
			metrics.InUseVehicles++
		case VehicleStatusMaintenance:
			metrics.MaintenanceVehicles++
		}
	}
	if metrics.TotalVehicles > 0 {
		metrics.UtilizationPct = float64(metrics.InUseVehicles) / float64(metrics.TotalVehicles) * 100
	}
	trips, err := s.repo.ListTrips(ctx, fleet.CompanyID, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list fleet trips: %w", err)
	}
	vehicleIDs := make(map[int64]struct{}, len(vehicles))
	for _, vehicle := range vehicles {
		vehicleIDs[vehicle.ID] = struct{}{}
	}
	shipmentIDs := make(map[int64]struct{})
	for _, trip := range trips {
		if _, ok := vehicleIDs[trip.VehicleID]; !ok || (trip.Status != TripStatusDispatched && trip.Status != TripStatusInProgress) {
			continue
		}
		metrics.ActiveTrips++
		stops, err := s.repo.GetTripStops(ctx, trip.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to list stops for trip %d: %w", trip.ID, err)
		}
		for _, stop := range stops {
			if stop.ShipmentID != nil {
				shipmentIDs[*stop.ShipmentID] = struct{}{}
			}
		}
	}
	metrics.ActiveShipments = len(shipmentIDs)
	return metrics, nil
}

func latestStopUpdate(stop *TripStop, trip *Trip, fallback *time.Time) *time.Time {
	latest := fallback
	for _, candidate := range []*time.Time{stop.ActualArrivalAt, stop.ActualDepartureAt, trip.ActualStartAt} {
		if candidate != nil && (latest == nil || candidate.After(*latest)) {
			value := *candidate
			latest = &value
		}
	}
	return latest
}

// FleetUtilization represents fleet capacity metrics
type FleetUtilization struct {
	FleetID             int64
	TotalVehicles       int
	AvailableVehicles   int
	InUseVehicles       int
	MaintenanceVehicles int
	UtilizationPct      float64
	ActiveTrips         int
	ActiveShipments     int
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
