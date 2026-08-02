package freight

import (
	"time"

	accountingmoney "github.com/odyssey-erp/odyssey-erp/internal/accounting/money"
)

// ═══════════════════════════════════════════════════════════════════════════
// RATE CARD TYPES
// ═══════════════════════════════════════════════════════════════════════════

// RateCard represents a freight rate for a specific route and carrier
type RateCard struct {
	ID              int64
	CompanyID       int64
	CarrierID       *int64
	OriginCity      string
	OriginCountry   string
	DestinationCity string
	DestinationCountry string
	ServiceLevel    ServiceLevel // STANDARD, EXPRESS, OVERNIGHT, ECONOMY
	MinWeightKg     *accountingmoney.Money
	MaxWeightKg     *accountingmoney.Money
	BaseRate        accountingmoney.Money
	PerKgRate       *accountingmoney.Money
	PerCbmRate      *accountingmoney.Money
	Currency        string
	EffectiveDate   time.Time
	ExpirationDate  *time.Time
	IsActive        bool
	CreatedBy       int64
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type ServiceLevel string

const (
	ServiceLevelStandard   ServiceLevel = "STANDARD"
	ServiceLevelExpress    ServiceLevel = "EXPRESS"
	ServiceLevelOvernight  ServiceLevel = "OVERNIGHT"
	ServiceLevelEconomy    ServiceLevel = "ECONOMY"
)

// RateSurcharge represents an additional charge to a rate card
type RateSurcharge struct {
	ID              int64
	CompanyID       int64
	RateCardID      int64
	SurchargeType   SurchargeType // FUEL, HOLIDAY, ZONE, HANDLING, INSURANCE
	SurchargeName   string
	SurchargeAmount *accountingmoney.Money
	SurchargePercent *float64 // Percentage of base rate
	EffectiveDate   time.Time
	ExpirationDate  *time.Time
	CreatedAt       time.Time
}

type SurchargeType string

const (
	SurchargeTypeFuel     SurchargeType = "FUEL"
	SurchargeTypeHoliday  SurchargeType = "HOLIDAY"
	SurchargeTypeZone     SurchargeType = "ZONE"
	SurchargeTypeHandling SurchargeType = "HANDLING"
	SurchargeTypeInsurance SurchargeType = "INSURANCE"
)

// ═══════════════════════════════════════════════════════════════════════════
// FREIGHT CHARGE TYPES
// ═══════════════════════════════════════════════════════════════════════════

// FreightCharge represents the calculated freight cost for a shipment or load
type FreightCharge struct {
	ID              int64
	CompanyID       int64
	ShipmentID      *int64
	LoadID          *int64
	CarrierID       *int64
	RateCardID      *int64
	OriginCity      string
	DestinationCity string
	ServiceLevel    *ServiceLevel
	WeightKg        *accountingmoney.Money
	VolumeCbm       *accountingmoney.Money
	BaseCharge      accountingmoney.Money
	WeightCharge    *accountingmoney.Money // weight_kg * per_kg_rate
	VolumeCharge    *accountingmoney.Money // volume_cbm * per_cbm_rate
	SurchargeTotal  *accountingmoney.Money
	FreightTotal    accountingmoney.Money
	Currency        string
	Status          FreightChargeStatus
	InvoiceNumber   *string
	InvoiceDate     *time.Time
	GLPostingID     *int64
	CostCenterID    *int64
	Notes           *string
	CreatedBy       int64
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type FreightChargeStatus string

const (
	FreightChargeStatusCalculated FreightChargeStatus = "CALCULATED"
	FreightChargeStatusInvoiced    FreightChargeStatus = "INVOICED"
	FreightChargeStatusPaid        FreightChargeStatus = "PAID"
)

// ═══════════════════════════════════════════════════════════════════════════
// LANDED COST TYPES
// ═══════════════════════════════════════════════════════════════════════════

// LandedCost represents total cost of goods including freight and duties
type LandedCost struct {
	ID               int64
	CompanyID        int64
	ShipmentID       int64
	LoadID           *int64
	FreightChargeID  int64
	POID             *int64
	ProductCost      accountingmoney.Money
	FreightCost      accountingmoney.Money
	DutyCost         *accountingmoney.Money
	TaxCost          *accountingmoney.Money
	InsuranceCost    *accountingmoney.Money
	OtherCost        *accountingmoney.Money
	TotalLandedCost  accountingmoney.Money
	CostPerUnit      *accountingmoney.Money
	Currency         string
	AllocationMethod AllocationMethod // WEIGHT, VOLUME, ITEM_COUNT, MANUAL
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type AllocationMethod string

const (
	AllocationMethodWeight    AllocationMethod = "WEIGHT"
	AllocationMethodVolume    AllocationMethod = "VOLUME"
	AllocationMethodItemCount AllocationMethod = "ITEM_COUNT"
	AllocationMethodManual    AllocationMethod = "MANUAL"
)

// ═══════════════════════════════════════════════════════════════════════════
// COST CENTER TYPES
// ═══════════════════════════════════════════════════════════════════════════

// CostCenter represents a business unit for cost allocation
type CostCenter struct {
	ID              int64
	CompanyID       int64
	CostCenterCode  string
	CostCenterName  string
	CostCenterType  CostCenterType
	WarehouseID     *int64
	GLAccount       *string
	ManagerID       *int64
	IsActive        bool
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type CostCenterType string

const (
	CostCenterTypeWarehouse CostCenterType = "WAREHOUSE"
	CostCenterTypeDepartment CostCenterType = "DEPARTMENT"
	CostCenterTypeProject    CostCenterType = "PROJECT"
	CostCenterTypeLocation   CostCenterType = "LOCATION"
)

// ═══════════════════════════════════════════════════════════════════════════
// AUDIT LOG TYPES
// ═══════════════════════════════════════════════════════════════════════════

// FreightAuditLog tracks changes to freight charges for compliance
type FreightAuditLog struct {
	ID              int64
	CompanyID       int64
	FreightChargeID int64
	AuditType       AuditType
	OldValue        *accountingmoney.Money
	NewValue        *accountingmoney.Money
	Reason          *string
	UserID          int64
	CreatedAt       time.Time
}

type AuditType string

const (
	AuditTypeCreated     AuditType = "CREATED"
	AuditTypeCalculated  AuditType = "CALCULATED"
	AuditTypeInvoiced    AuditType = "INVOICED"
	AuditTypePosted      AuditType = "POSTED"
	AuditTypeReconciled  AuditType = "RECONCILED"
)

// ═══════════════════════════════════════════════════════════════════════════
// INPUT TYPES FOR SERVICE METHODS
// ═══════════════════════════════════════════════════════════════════════════

type CreateRateCardInput struct {
	CompanyID        int64
	CarrierID        *int64
	OriginCity       string
	OriginCountry    string
	DestinationCity  string
	DestinationCountry string
	ServiceLevel     ServiceLevel
	MinWeightKg      *accountingmoney.Money
	MaxWeightKg      *accountingmoney.Money
	BaseRate         accountingmoney.Money
	PerKgRate        *accountingmoney.Money
	PerCbmRate       *accountingmoney.Money
	Currency         string
	EffectiveDate    time.Time
	ExpirationDate   *time.Time
	CreatedBy        int64
}

type CalculateFreightInput struct {
	CompanyID       int64
	CarrierID       *int64
	OriginCity      string
	DestinationCity string
	ServiceLevel    ServiceLevel
	WeightKg        *accountingmoney.Money
	VolumeCbm       *accountingmoney.Money
	ShipmentID      *int64
	LoadID          *int64
	CostCenterID    *int64
}

type CalculateFreightOutput struct {
	BaseCharge      accountingmoney.Money
	WeightCharge    *accountingmoney.Money
	VolumeCharge    *accountingmoney.Money
	SurchargeTotal  *accountingmoney.Money
	FreightTotal    accountingmoney.Money
	Currency        string
	Breakdown       string // Human-readable breakdown of charges
}

type CalculateLandedCostInput struct {
	ShipmentID      int64
	LoadID          *int64
	POID            *int64
	ProductCost     accountingmoney.Money
	FreightChargeID int64
	DutyCost        *accountingmoney.Money
	TaxCost         *accountingmoney.Money
	InsuranceCost   *accountingmoney.Money
	OtherCost       *accountingmoney.Money
	AllocationMethod AllocationMethod
	CompanyID       int64
}

type PostFreightToGLInput struct {
	CompanyID       int64
	FreightChargeID int64
	CostCenterID    int64
	GLAccount       string
	FreightAmount   accountingmoney.Money
	Description     string
	PostedBy        int64
}
