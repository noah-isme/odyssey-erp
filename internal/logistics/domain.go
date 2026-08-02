package logistics

import (
	"time"

	accountingmoney "github.com/odyssey-erp/odyssey-erp/internal/accounting/money"
)

// ═══════════════════════════════════════════════════════════════════════════
// CARRIER DOMAIN TYPES
// ═══════════════════════════════════════════════════════════════════════════

// Carrier represents a third-party or internal transport provider
type Carrier struct {
	ID                    int64
	CompanyID             int64
	CarrierName           string
	CarrierCode           string
	Status                CarrierStatus
	ContactName           string
	ContactEmail          string
	ContactPhone          string
	InsuranceProvider     string
	InsurancePolicyNumber string
	InsuranceExpiresAt    *time.Time
	CreatedAt             time.Time
	UpdatedAt             time.Time
	CreatedBy             int64
	UpdatedBy             int64
}

type CarrierStatus string

const (
	CarrierStatusActive    CarrierStatus = "ACTIVE"
	CarrierStatusInactive  CarrierStatus = "INACTIVE"
	CarrierStatusSuspended CarrierStatus = "SUSPENDED"
)

// CarrierRateCard defines pricing for a route, weight range, and volume
type CarrierRateCard struct {
	ID              int64
	CompanyID       int64
	CarrierID       int64
	RouteFromCity   string
	RouteTCity      string
	WeightFrom      accountingmoney.Money // kg
	WeightTo        accountingmoney.Money // kg
	VolumeFrom      accountingmoney.Money // cubic meters
	VolumeTo        accountingmoney.Money // cubic meters
	RatePerUnit     accountingmoney.Money
	RateUnit        RateUnit // KG, CBM, SHIPMENT
	Currency        string
	EffectiveFrom   time.Time
	EffectiveTo     *time.Time
	MinimumCharge   *accountingmoney.Money
	FuelSurchargePct *accountingmoney.Money // percentage
	CreatedAt       time.Time
}

type RateUnit string

const (
	RateUnitKG       RateUnit = "KG"
	RateUnitCBM      RateUnit = "CBM"
	RateUnitShipment RateUnit = "SHIPMENT"
)

// ═══════════════════════════════════════════════════════════════════════════
// FLEET & VEHICLE DOMAIN TYPES
// ═══════════════════════════════════════════════════════════════════════════

// Fleet groups vehicles for management and reporting
type Fleet struct {
	ID            int64
	CompanyID     int64
	FleetName     string
	FleetCode     string
	FleetType     FleetType
	Status        FleetStatus
	WarehouseID   *int64
	HomeCity      string
	Notes         string
	CreatedAt     time.Time
	UpdatedAt     time.Time
	CreatedBy     int64
}

type FleetType string

const (
	FleetTypeOwn        FleetType = "OWN"
	FleetTypeContracted FleetType = "CONTRACTED"
	FleetTypeMixed      FleetType = "MIXED"
)

type FleetStatus string

const (
	FleetStatusActive   FleetStatus = "ACTIVE"
	FleetStatusInactive FleetStatus = "INACTIVE"
	FleetStatusRetired  FleetStatus = "RETIRED"
)

// Vehicle represents a single transport unit (van, truck, etc.)
type Vehicle struct {
	ID                 int64
	CompanyID          int64
	FleetID            int64
	VehicleRegistration string
	VehicleType        VehicleType
	Status             VehicleStatus
	MaxWeightKg        *float64
	MaxVolumeCbm       *accountingmoney.Money
	LicensePlate       string
	VIN                string
	Make               string
	Model              string
	YearManufactured   *int
	LastMaintenanceAt  *time.Time
	NextMaintenanceDue *time.Time
	InsuranceExpiresAt *time.Time
	GPSDeviceID        string
	Notes              string
	CreatedAt          time.Time
	UpdatedAt          time.Time
	CreatedBy          int64
}

type VehicleType string

const (
	VehicleTypeVan   VehicleType = "VAN"
	VehicleTypeTruck VehicleType = "TRUCK"
	VehicleTypeLorry VehicleType = "LORRY"
	VehicleTypeBike  VehicleType = "BIKE"
	VehicleTypeCar   VehicleType = "CAR"
)

type VehicleStatus string

const (
	VehicleStatusAvailable   VehicleStatus = "AVAILABLE"
	VehicleStatusInUse       VehicleStatus = "IN_USE"
	VehicleStatusMaintenance VehicleStatus = "MAINTENANCE"
	VehicleStatusRetired     VehicleStatus = "RETIRED"
)

// ═══════════════════════════════════════════════════════════════════════════
// DRIVER DOMAIN TYPES
// ═══════════════════════════════════════════════════════════════════════════

// Driver represents a person authorized to operate vehicles
type Driver struct {
	ID                    int64
	CompanyID             int64
	DriverName            string
	DriverCode            string
	Status                DriverStatus
	Email                 string
	Phone                 string
	LicenseNumber         string
	LicenseClass          LicenseClass
	LicenseExpiresAt      *time.Time
	EmergencyContactName  string
	EmergencyContactPhone string
	Notes                 string
	CreatedAt             time.Time
	UpdatedAt             time.Time
	CreatedBy             int64
}

type DriverStatus string

const (
	DriverStatusActive      DriverStatus = "ACTIVE"
	DriverStatusInactive    DriverStatus = "INACTIVE"
	DriverStatusOnLeave     DriverStatus = "ON_LEAVE"
	DriverStatusTerminated  DriverStatus = "TERMINATED"
)

type LicenseClass string

const (
	LicenseClassA LicenseClass = "A" // Motorcycle
	LicenseClassB LicenseClass = "B" // Car
	LicenseClassC LicenseClass = "C" // Truck
	LicenseClassD LicenseClass = "D" // Bus
	LicenseClassE LicenseClass = "E" // Articulated
)

// ═══════════════════════════════════════════════════════════════════════════
// SHIPMENT DOMAIN TYPES
// ═══════════════════════════════════════════════════════════════════════════

// Shipment represents goods in transit or scheduled for transit
type Shipment struct {
	ID                      int64
	CompanyID               int64
	ShipmentNumber          string
	Status                  ShipmentStatus
	ShipmentType            ShipmentType
	OriginWarehouseID       *int64
	DestinationWarehouseID  *int64
	DestinationAddress      string
	DestinationCity         string
	DestinationCountry      string
	DestinationContactName  string
	DestinationContactPhone string
	// Transport assignment: either vehicle+driver OR carrier+service
	VehicleID              *int64
	DriverID               *int64
	CarrierID              *int64
	CarrierServiceType     *CarrierServiceType
	// Scheduling
	PlannedDispatchAt       *time.Time
	PlannedDeliveryAt       *time.Time
	ActualDispatchAt        *time.Time
	ActualDeliveryAt        *time.Time
	// Tracking
	TotalWeightKg           *accountingmoney.Money
	TotalVolumeCbm          *accountingmoney.Money
	FreightCharge           *accountingmoney.Money
	FreightCurrency         string
	Notes                   string
	CreatedAt               time.Time
	UpdatedAt               time.Time
	CreatedBy               int64
}

type ShipmentStatus string

const (
	ShipmentStatusDraft      ShipmentStatus = "DRAFT"
	ShipmentStatusConfirmed  ShipmentStatus = "CONFIRMED"
	ShipmentStatusDispatched ShipmentStatus = "DISPATCHED"
	ShipmentStatusInTransit  ShipmentStatus = "IN_TRANSIT"
	ShipmentStatusDelivered  ShipmentStatus = "DELIVERED"
	ShipmentStatusCancelled  ShipmentStatus = "CANCELLED"
)

type ShipmentType string

const (
	ShipmentTypeDelivery ShipmentType = "DELIVERY"
	ShipmentTypeReturn   ShipmentType = "RETURN"
	ShipmentTypeTransfer ShipmentType = "TRANSFER"
)

type CarrierServiceType string

const (
	CarrierServiceStandard  CarrierServiceType = "STANDARD"
	CarrierServiceExpress   CarrierServiceType = "EXPRESS"
	CarrierServiceOvernight CarrierServiceType = "OVERNIGHT"
	CarrierServiceEconomy   CarrierServiceType = "ECONOMY"
)

// ShipmentLine represents items in a shipment
type ShipmentLine struct {
	ID            int64
	CompanyID     int64
	ShipmentID    int64
	ProductID     int64
	Quantity      accountingmoney.Money
	WeightKg      *accountingmoney.Money
	VolumeCbm     *accountingmoney.Money
	LotNumber     string
	SerialNumbers []string
	CreatedAt     time.Time
}

// ═══════════════════════════════════════════════════════════════════════════
// TRIP DOMAIN TYPES
// ═══════════════════════════════════════════════════════════════════════════

// Trip represents a complete journey by a vehicle+driver combination
type Trip struct {
	ID                int64
	CompanyID         int64
	TripNumber        string
	Status            TripStatus
	VehicleID         int64
	DriverID          int64
	FleetID           *int64
	OriginWarehouseID *int64
	PlannedStartAt    *time.Time
	PlannedEndAt      *time.Time
	ActualStartAt     *time.Time
	ActualEndAt       *time.Time
	TotalDistanceKm   *float64
	FuelUsedLiters    *accountingmoney.Money
	Notes             string
	CreatedAt         time.Time
	UpdatedAt         time.Time
	CreatedBy         int64
}

type TripStatus string

const (
	TripStatusPlanned    TripStatus = "PLANNED"
	TripStatusDispatched TripStatus = "DISPATCHED"
	TripStatusInProgress TripStatus = "IN_PROGRESS"
	TripStatusCompleted  TripStatus = "COMPLETED"
	TripStatusCancelled  TripStatus = "CANCELLED"
)

// TripStop represents a single stop in a trip (pickup, delivery, or transfer)
type TripStop struct {
	ID              int64
	CompanyID       int64
	TripID          int64
	ShipmentID      *int64
	StopSequence    int
	StopType        StopType
	WarehouseID     *int64
	LocationAddress string
	LocationCity    string
	LocationLat     *float64
	LocationLon     *float64
	ContactName     string
	ContactPhone    string
	PlannedArrivalAt *time.Time
	ActualArrivalAt  *time.Time
	ActualDepartureAt *time.Time
	Notes           string
	CreatedAt       time.Time
}

type StopType string

const (
	StopTypePickup    StopType = "PICKUP"
	StopTypeDelivery  StopType = "DELIVERY"
	StopTypeTransfer  StopType = "TRANSFER"
)
