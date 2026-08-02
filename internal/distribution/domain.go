package distribution

import (
	"time"

	accountingmoney "github.com/odyssey-erp/odyssey-erp/internal/accounting/money"
)

// ═══════════════════════════════════════════════════════════════════════════
// PLANNING CONFIGURATION TYPES
// ═══════════════════════════════════════════════════════════════════════════

// PlanningHorizon defines the time window for distribution planning at a warehouse
type PlanningHorizon struct {
	ID               int64
	CompanyID        int64
	WarehouseID      int64
	PlanningStartDate time.Time
	PlanningEndDate   time.Time
	FrozenUntilDate  *time.Time // Planned orders frozen until this date
	Status           HorizonStatus
	Notes            string
	CreatedAt        time.Time
	UpdatedAt        time.Time
	CreatedBy        int64
}

type HorizonStatus string

const (
	HorizonStatusActive   HorizonStatus = "ACTIVE"
	HorizonStatusArchived HorizonStatus = "ARCHIVED"
	HorizonStatusCancelled HorizonStatus = "CANCELLED"
)

// PlanningRule defines constraints for load planning (capacity, weight, time, etc.)
type PlanningRule struct {
	ID                    int64
	CompanyID             int64
	WarehouseID           int64
	RuleName              string
	RuleType              RuleType
	MaxLoadWeightKg       *accountingmoney.Money
	MaxLoadVolumeCbm      *accountingmoney.Money
	MaxItemsPerLoad       *int
	TimeWindowStart       *time.Time
	TimeWindowEnd         *time.Time
	VehicleTypeRequired   string
	CustomRuleExpression  string
	Priority              int
	IsActive              bool
	CreatedAt             time.Time
	CreatedBy             int64
}

type RuleType string

const (
	RuleTypeCapacity    RuleType = "CAPACITY"
	RuleTypeWeight      RuleType = "WEIGHT"
	RuleTypeTimeWindow  RuleType = "TIME_WINDOW"
	RuleTypeVehicleType RuleType = "VEHICLE_TYPE"
	RuleTypeCustom      RuleType = "CUSTOM"
)

// ═══════════════════════════════════════════════════════════════════════════
// LOAD PLANNING & CONSOLIDATION TYPES
// ═══════════════════════════════════════════════════════════════════════════

// Load represents consolidated shipments ready for transport
type Load struct {
	ID                    int64
	CompanyID             int64
	LoadNumber            string
	Status                LoadStatus
	OriginWarehouseID     int64
	DestinationWarehouseID *int64
	DestinationAddress    string
	DestinationCity       string
	DestinationCountry    string
	// Transport assignment: either vehicle+driver OR carrier+service
	VehicleID             *int64
	DriverID              *int64
	CarrierID             *int64
	CarrierServiceType    *string
	// Load metrics
	TotalWeightKg         *accountingmoney.Money
	TotalVolumeCbm        *accountingmoney.Money
	TotalItems            *int
	// Planning dates
	PlannedPickupDate     *time.Time
	PlannedDeliveryDate   *time.Time
	ActualDispatchAt      *time.Time
	ActualDeliveryAt      *time.Time
	// Costs
	FreightCharge         *accountingmoney.Money
	FreightCurrency       string
	Notes                 string
	CreatedAt             time.Time
	UpdatedAt             time.Time
	CreatedBy             int64
}

type LoadStatus string

const (
	LoadStatusDraft      LoadStatus = "DRAFT"
	LoadStatusPlanned    LoadStatus = "PLANNED"
	LoadStatusReady      LoadStatus = "READY"
	LoadStatusDispatched LoadStatus = "DISPATCHED"
	LoadStatusInTransit  LoadStatus = "IN_TRANSIT"
	LoadStatusDelivered  LoadStatus = "DELIVERED"
	LoadStatusCancelled  LoadStatus = "CANCELLED"
)

// LoadItem represents a product in a load (may be from one or more shipments)
type LoadItem struct {
	ID         int64
	CompanyID  int64
	LoadID     int64
	ShipmentID *int64
	ProductID  int64
	Quantity   accountingmoney.Money
	WeightKg   *accountingmoney.Money
	VolumeCbm  *accountingmoney.Money
	CreatedAt  time.Time
}

// ═══════════════════════════════════════════════════════════════════════════
// ROUTE OPTIMIZATION & SEQUENCING TYPES
// ═══════════════════════════════════════════════════════════════════════════

// DeliveryRoute represents an optimized sequence of stops for a load
type DeliveryRoute struct {
	ID                  int64
	CompanyID           int64
	RouteNumber         string
	Status              RouteStatus
	LoadID              int64
	TotalDistanceKm     *float64
	EstimatedDurationMinutes *int
	OptimizationScore   *accountingmoney.Money // 0-100 efficiency metric
	CreatedAt           time.Time
	UpdatedAt           time.Time
	CreatedBy           int64
}

type RouteStatus string

const (
	RouteStatusDraft      RouteStatus = "DRAFT"
	RouteStatusOptimized  RouteStatus = "OPTIMIZED"
	RouteStatusApproved   RouteStatus = "APPROVED"
	RouteStatusActive     RouteStatus = "ACTIVE"
	RouteStatusCompleted  RouteStatus = "COMPLETED"
	RouteStatusCancelled  RouteStatus = "CANCELLED"
)

// RouteStop represents a single stop in a delivery route
type RouteStop struct {
	ID                 int64
	CompanyID          int64
	RouteID            int64
	StopSequence       int
	StopType           StopType
	WarehouseID        *int64
	CustomerID         *int64
	CustomerAddress    string
	CustomerCity       string
	LocationLat        *float64
	LocationLon        *float64
	ContactName        string
	ContactPhone       string
	PlannedArrivalTime *time.Time
	PlannedDepartureTime *time.Time
	ActualArrivalAt    *time.Time
	ActualDepartureAt  *time.Time
	ItemsDelivered     *int
	Notes              string
	CreatedAt          time.Time
}

type StopType string

const (
	StopTypeWarehouse     StopType = "WAREHOUSE"
	StopTypeCustomer      StopType = "CUSTOMER"
	StopTypeDeliveryPoint StopType = "DELIVERY_POINT"
)

// ═══════════════════════════════════════════════════════════════════════════
// TRANSFER ORDER TYPES (Inter-Warehouse Transfers)
// ═══════════════════════════════════════════════════════════════════════════

// TransferOrder represents inter-warehouse inventory transfers
type TransferOrder struct {
	ID                     int64
	CompanyID              int64
	TransferNumber         string
	Status                 TransferStatus
	FromWarehouseID        int64
	ToWarehouseID          int64
	// Transport assignment
	LoadID                 *int64
	VehicleID              *int64
	DriverID               *int64
	CarrierID              *int64
	CarrierServiceType     *string
	// Scheduling
	PlannedDispatchDate    *time.Time
	PlannedArrivalDate     *time.Time
	ActualDispatchAt       *time.Time
	ActualArrivalAt        *time.Time
	// Tracking
	TotalWeightKg          *accountingmoney.Money
	TotalVolumeCbm         *accountingmoney.Money
	TotalItems             *int
	InTransitQuantity      *accountingmoney.Money
	// Costs
	TransferCost           *accountingmoney.Money
	TransferCostCurrency   string
	Notes                  string
	CreatedAt              time.Time
	UpdatedAt              time.Time
	CreatedBy              int64
}

type TransferStatus string

const (
	TransferStatusDraft      TransferStatus = "DRAFT"
	TransferStatusApproved   TransferStatus = "APPROVED"
	TransferStatusDispatched TransferStatus = "DISPATCHED"
	TransferStatusInTransit  TransferStatus = "IN_TRANSIT"
	TransferStatusReceived   TransferStatus = "RECEIVED"
	TransferStatusCancelled  TransferStatus = "CANCELLED"
)

// TransferOrderLine represents items in a transfer order
type TransferOrderLine struct {
	ID                int64
	CompanyID         int64
	TransferOrderID   int64
	ProductID         int64
	QuantityRequested accountingmoney.Money
	QuantityShipped   *accountingmoney.Money
	QuantityReceived  *accountingmoney.Money
	LotNumber         string
	SerialNumbers     []string
	CreatedAt         time.Time
}
