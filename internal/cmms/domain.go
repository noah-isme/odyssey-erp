package cmms

import (
	"errors"
	"strings"
	"time"
)

// Status captures the state of a work order.
type Status string

const (
	WorkOrderStatusDraft      Status = "DRAFT"
	WorkOrderStatusPlanned    Status = "PLANNED"
	WorkOrderStatusScheduled  Status = "SCHEDULED"
	WorkOrderStatusInProgress Status = "IN_PROGRESS"
	WorkOrderStatusOnHold     Status = "ON_HOLD"
	WorkOrderStatusCompleted  Status = "COMPLETED"
	WorkOrderStatusCancelled  Status = "CANCELLED"
	WorkOrderStatusClosed     Status = "CLOSED"
)

// Priority represents work order priority.
type Priority string

const (
	PriorityLow      Priority = "LOW"
	PriorityMedium   Priority = "MEDIUM"
	PriorityHigh     Priority = "HIGH"
	PriorityCritical Priority = "CRITICAL"
)

// WorkOrder represents a maintenance work order.
type WorkOrder struct {
	ID             int64
	CompanyID      int64
	Number         string
	Title          string
	Description    string
	AssetID        *int64
	AssetName      string
	LocationID     *int64
	LocationName   string
	Priority       Priority
	Status         Status
	Category       string // PREVENTIVE, CORRECTIVE, PREDICTIVE, INSPECTION
	RequesterID    int64
	AssigneeID     *int64
	PlannedStart   *time.Time
	PlannedEnd     *time.Time
	ActualStart    *time.Time
	ActualEnd      *time.Time
	EstimatedHours float64
	ActualHours    float64
	CreatedBy      int64
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// WorkOrderTask represents a task within a work order.
type WorkOrderTask struct {
	ID             int64
	WorkOrderID    int64
	Sequence       int
	Title          string
	Description    string
	Status         Status
	AssigneeID     *int64
	EstimatedHours float64
	ActualHours    float64
	CompletedAt    *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// Asset represents a maintainable asset.
type Asset struct {
	ID             int64
	CompanyID      int64
	Code           string
	Name           string
	Description    string
	AssetType      string // EQUIPMENT, FACILITY, VEHICLE, TOOL, INFRASTRUCTURE
	ParentID       *int64
	LocationID     *int64
	Manufacturer   string
	Model          string
	SerialNumber   string
	InstallDate    *time.Time
	WarrantyExpiry *time.Time
	Status         string // ACTIVE, INACTIVE, DECOMMISSIONED, SCRAPPED
	Criticality    string // A, B, C, D
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// AssetSpecification represents technical specifications for an asset.
type AssetSpecification struct {
	ID        int64
	AssetID   int64
	Name      string
	Value     string
	Unit      string
	DataType  string // STRING, NUMBER, BOOLEAN, DATE
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Location represents a physical location.
type Location struct {
	ID          int64
	CompanyID   int64
	Code        string
	Name        string
	Description string
	ParentID    *int64
	Address     string
	GPSLat      *float64
	GPSLng      *float64
	Active      bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// PreventiveMaintenanceSchedule represents a PM schedule.
type PreventiveMaintenanceSchedule struct {
	ID               int64
	CompanyID        int64
	AssetID          int64
	Name             string
	Description      string
	FrequencyType    string // DAILY, WEEKLY, MONTHLY, QUARTERLY, SEMI_ANNUAL, ANNUAL, METER_BASED
	FrequencyValue   int
	MeterReadingType string // HOURS, CYCLES, DISTANCE
	TaskTemplateID   *int64
	NextDueDate      *time.Time
	NextDueMeter     *float64
	Active           bool
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// TaskTemplate represents a reusable task template.
type TaskTemplate struct {
	ID             int64
	CompanyID      int64
	Name           string
	Description    string
	Category       string
	EstimatedHours float64
	Instructions   string
	SafetyNotes    string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// TaskTemplateStep represents a step in a task template.
type TaskTemplateStep struct {
	ID             int64
	TaskTemplateID int64
	Sequence       int
	Title          string
	Description    string
	EstimatedHours float64
	Instructions   string
	CreatedAt      time.Time
}

// SparePart represents a spare part inventory item.
type SparePart struct {
	ID            int64
	CompanyID     int64
	Code          string
	Name          string
	Description   string
	Category      string
	UnitOfMeasure string
	MinQuantity   float64
	MaxQuantity   float64
	ReorderPoint  float64
	LeadTimeDays  int
	UnitCost      float64
	CriticalSpare bool
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// WorkOrderSparePart represents a spare part used in a work order.
type WorkOrderSparePart struct {
	ID          int64
	WorkOrderID int64
	SparePartID int64
	Quantity    float64
	UnitCost    float64
	TotalCost   float64
	IssuedAt    *time.Time
	IssuedBy    *int64
	CreatedAt   time.Time
}

// MeterReading represents an asset meter reading.
type MeterReading struct {
	ID          int64
	AssetID     int64
	ReadingType string // HOURS, CYCLES, DISTANCE, TEMPERATURE, PRESSURE, VOLTAGE
	Value       float64
	ReadingDate time.Time
	EnteredBy   int64
	Notes       string
	CreatedAt   time.Time
}

// CreateWorkOrderRequest defines the payload for creating a work order.
type CreateWorkOrderRequest struct {
	CompanyID      int64
	Title          string
	Description    string
	AssetID        *int64
	LocationID     *int64
	Priority       Priority
	Category       string
	RequesterID    int64
	AssigneeID     *int64
	PlannedStart   *time.Time
	PlannedEnd     *time.Time
	EstimatedHours float64
	ActorID        int64
}

// Validate ensures the creation request can be processed.
func (r CreateWorkOrderRequest) Validate() error {
	if r.CompanyID <= 0 {
		return errors.New("cmms: company id required")
	}
	if strings.TrimSpace(r.Title) == "" {
		return errors.New("cmms: title required")
	}
	if r.RequesterID <= 0 {
		return errors.New("cmms: requester id required")
	}
	if r.ActorID <= 0 {
		return errors.New("cmms: actor id required")
	}
	return nil
}

// CreateAssetRequest defines the payload for creating an asset.
type CreateAssetRequest struct {
	CompanyID      int64
	Code           string
	Name           string
	Description    string
	AssetType      string
	ParentID       *int64
	LocationID     *int64
	Manufacturer   string
	Model          string
	SerialNumber   string
	InstallDate    *time.Time
	WarrantyExpiry *time.Time
	Criticality    string
	ActorID        int64
}

// Validate ensures the creation request can be processed.
func (r CreateAssetRequest) Validate() error {
	if r.CompanyID <= 0 {
		return errors.New("cmms: company id required")
	}
	if strings.TrimSpace(r.Code) == "" {
		return errors.New("cmms: asset code required")
	}
	if strings.TrimSpace(r.Name) == "" {
		return errors.New("cmms: asset name required")
	}
	if r.ActorID <= 0 {
		return errors.New("cmms: actor id required")
	}
	return nil
}

// ListWorkOrdersFilter configures ListWorkOrders queries.
type ListWorkOrdersFilter struct {
	CompanyID  int64
	AssetID    *int64
	LocationID *int64
	AssigneeID *int64
	Status     *Status
	Priority   *Priority
	Category   string
	DateFrom   *time.Time
	DateTo     *time.Time
	Limit      int
	Offset     int
}

// ListAssetsFilter configures ListAssets queries.
type ListAssetsFilter struct {
	CompanyID  int64
	LocationID *int64
	AssetType  string
	Status     string
	Search     string
	Limit      int
	Offset     int
}

var (
	ErrWorkOrderNotFound = errors.New("cmms: work order not found")
	ErrAssetNotFound     = errors.New("cmms: asset not found")
	ErrLocationNotFound  = errors.New("cmms: location not found")
	ErrInvalidStatus     = errors.New("cmms: invalid status transition")
)

// NormaliseStatus uppercases and trims the provided status string.
func NormaliseStatus(v string) Status {
	v = strings.TrimSpace(strings.ToUpper(v))
	switch Status(v) {
	case WorkOrderStatusDraft, WorkOrderStatusPlanned, WorkOrderStatusScheduled,
		WorkOrderStatusInProgress, WorkOrderStatusOnHold, WorkOrderStatusCompleted,
		WorkOrderStatusCancelled, WorkOrderStatusClosed:
		return Status(v)
	default:
		return WorkOrderStatusDraft
	}
}

// NormalisePriority uppercases and trims the provided priority string.
func NormalisePriority(v string) Priority {
	v = strings.TrimSpace(strings.ToUpper(v))
	switch Priority(v) {
	case PriorityLow, PriorityMedium, PriorityHigh, PriorityCritical:
		return Priority(v)
	default:
		return PriorityMedium
	}
}

// =============================================================================
// Advanced CMMS Features (Predictive Maintenance, IoT)
// =============================================================================

// IoTSensor represents a physical IoT device attached to an asset.
type IoTSensor struct {
	ID               int64
	CompanyID        int64
	AssetID          int64
	SensorCode       string
	SensorType       string // VIBRATION, TEMPERATURE, ACOUSTIC, PRESSURE, CURRENT
	Status           string // ACTIVE, OFFLINE, MAINTENANCE
	LastReadingAt    *time.Time
	LastReadingValue *float64
	CreatedAt        time.Time
}

// IoTReading represents a time-series reading from an IoT sensor.
type IoTReading struct {
	ID        int64
	CompanyID int64
	SensorID  int64
	Value     float64
	Timestamp time.Time
}

// PredictiveModel represents an AI/ML model for predicting asset failures.
type PredictiveModel struct {
	ID         int64
	CompanyID  int64
	AssetType  string
	ModelName  string
	Version    string
	Accuracy   float64
	IsActive   bool
	DeployedAt time.Time
}

// PredictiveAlert represents an alert generated by a predictive model or rules engine based on IoT data.
type PredictiveAlert struct {
	ID          int64
	CompanyID   int64
	AssetID     int64
	SensorID    *int64
	ModelID     *int64
	Severity    string // WARNING, CRITICAL
	Description string
	GeneratedAt time.Time
	ResolvedAt  *time.Time
}

// PredictiveAnomaly is a repository projection used to turn the latest
// sensor reading into an idempotent predictive alert.
type PredictiveAnomaly struct {
	CompanyID int64
	AssetID   int64
	SensorID  int64
	ModelID   int64
	Value     float64
	Timestamp time.Time
}
