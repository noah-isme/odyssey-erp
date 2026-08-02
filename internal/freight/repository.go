package freight

import (
	"context"
accountingmoney "github.com/odyssey-erp/odyssey-erp/internal/accounting/money"
"time"
)

// ═══════════════════════════════════════════════════════════════════════════
// REPOSITORY INTERFACE
// ═══════════════════════════════════════════════════════════════════════════

type Repository interface {
	// Rate Card operations
	CreateRateCard(ctx context.Context, input CreateRateCardInput) (*RateCard, error)
	GetRateCard(ctx context.Context, companyID, rateCardID int64) (*RateCard, error)
	ListRateCards(ctx context.Context, companyID int64, filter RateCardFilter) ([]*RateCard, error)
	UpdateRateCard(ctx context.Context, companyID, rateCardID int64, updates RateCardUpdate) (*RateCard, error)
	DeactivateRateCard(ctx context.Context, companyID, rateCardID int64) error

	// Rate Card lookup for calculation
	GetApplicableRateCard(ctx context.Context, companyID int64, lookup RateLookup) (*RateCard, error)
	ListApplicableRateCards(ctx context.Context, companyID int64, lookup RateLookup) ([]*RateCard, error)

	// Rate Surcharge operations
	CreateRateSurcharge(ctx context.Context, companyID, rateCardID int64, input CreateRateSurchargeInput) (*RateSurcharge, error)
	ListRateSurcharges(ctx context.Context, rateCardID int64) ([]*RateSurcharge, error)
	DeleteRateSurcharge(ctx context.Context, surchargeID int64) error

	// Freight Charge operations
	CreateFreightCharge(ctx context.Context, charge *FreightCharge) (*FreightCharge, error)
	GetFreightCharge(ctx context.Context, companyID, chargeID int64) (*FreightCharge, error)
	ListFreightCharges(ctx context.Context, companyID int64, filter FreightChargeFilter) ([]*FreightCharge, error)
	UpdateFreightCharge(ctx context.Context, companyID, chargeID int64, updates FreightChargeUpdate) (*FreightCharge, error)
	UpdateFreightChargeStatus(ctx context.Context, companyID, chargeID int64, status FreightChargeStatus) error

	// Landed Cost operations
	CreateLandedCost(ctx context.Context, cost *LandedCost) (*LandedCost, error)
	GetLandedCost(ctx context.Context, companyID, costID int64) (*LandedCost, error)
	ListLandedCosts(ctx context.Context, companyID int64, filter LandedCostFilter) ([]*LandedCost, error)
	GetLandedCostByShipment(ctx context.Context, companyID, shipmentID int64) (*LandedCost, error)

	// Cost Center operations
	CreateCostCenter(ctx context.Context, companyID int64, input CreateCostCenterInput) (*CostCenter, error)
	GetCostCenter(ctx context.Context, companyID, costCenterID int64) (*CostCenter, error)
	GetCostCenterByCode(ctx context.Context, companyID int64, code string) (*CostCenter, error)
	ListCostCenters(ctx context.Context, companyID int64) ([]*CostCenter, error)
	UpdateCostCenter(ctx context.Context, companyID, costCenterID int64, updates CostCenterUpdate) (*CostCenter, error)

	// Audit Log operations
	CreateAuditLog(ctx context.Context, log *FreightAuditLog) error
	ListAuditLogs(ctx context.Context, companyID, freightChargeID int64) ([]*FreightAuditLog, error)
}

// ═══════════════════════════════════════════════════════════════════════════
// FILTER & UPDATE TYPES
// ═══════════════════════════════════════════════════════════════════════════

type RateCardFilter struct {
	CarrierID          *int64
	OriginCity         *string
	DestinationCity    *string
	ServiceLevel       *ServiceLevel
	IncludeInactive    bool
	EffectiveDateFrom  *time.Time
	EffectiveDateTo    *time.Time
	Limit              int
	Offset             int
}

type RateCardUpdate struct {
	BaseRate       *accountingmoney.Money
	PerKgRate      *accountingmoney.Money
	PerCbmRate     *accountingmoney.Money
	ExpirationDate *time.Time
	IsActive       *bool
}

type RateLookup struct {
	CarrierID          *int64
	OriginCity         string
	DestinationCity    string
	ServiceLevel       ServiceLevel
	WeightKg           *accountingmoney.Money
	VolumeCbm          *accountingmoney.Money
	AsOfDate           time.Time
}

type CreateRateSurchargeInput struct {
	SurchargeType    SurchargeType
	SurchargeName    string
	SurchargeAmount  *accountingmoney.Money
	SurchargePercent *float64
	EffectiveDate    time.Time
	ExpirationDate   *time.Time
}

type FreightChargeFilter struct {
	ShipmentID    *int64
	LoadID        *int64
	CarrierID     *int64
	Status        *FreightChargeStatus
	OriginCity    *string
	DestinationCity *string
	CreatedAfter  *time.Time
	CreatedBefore *time.Time
	Limit         int
	Offset        int
}

type FreightChargeUpdate struct {
	Status        *FreightChargeStatus
	InvoiceNumber *string
	InvoiceDate   *time.Time
	GLPostingID   *int64
	Notes         *string
}

type LandedCostFilter struct {
	ShipmentID *int64
	LoadID     *int64
	POID       *int64
	CreatedAfter *time.Time
	CreatedBefore *time.Time
	Limit      int
	Offset     int
}

type CreateCostCenterInput struct {
	CostCenterCode  string
	CostCenterName  string
	CostCenterType  CostCenterType
	WarehouseID     *int64
	GLAccount       *string
	ManagerID       *int64
}

type CostCenterUpdate struct {
	CostCenterName *string
	GLAccount      *string
	ManagerID      *int64
	IsActive       *bool
}

// ═══════════════════════════════════════════════════════════════════════════
// EXPORT FOR TESTS
// ═══════════════════════════════════════════════════════════════════════════

type MockRepository struct {
	RateCards      map[int64]*RateCard
	FreightCharges map[int64]*FreightCharge
	LandedCosts    map[int64]*LandedCost
	CostCenters    map[int64]*CostCenter
	AuditLogs      []*FreightAuditLog
}

func NewMockRepository() *MockRepository {
	return &MockRepository{
		RateCards:      make(map[int64]*RateCard),
		FreightCharges: make(map[int64]*FreightCharge),
		LandedCosts:    make(map[int64]*LandedCost),
		CostCenters:    make(map[int64]*CostCenter),
		AuditLogs:      make([]*FreightAuditLog, 0),
	}
}

// Implement all Repository methods for MockRepository
func (m *MockRepository) CreateRateCard(ctx context.Context, input CreateRateCardInput) (*RateCard, error) {
	rc := &RateCard{ID: int64(len(m.RateCards) + 1)}
	m.RateCards[rc.ID] = rc
	return rc, nil
}

func (m *MockRepository) GetRateCard(ctx context.Context, companyID, rateCardID int64) (*RateCard, error) {
	return m.RateCards[rateCardID], nil
}

func (m *MockRepository) ListRateCards(ctx context.Context, companyID int64, filter RateCardFilter) ([]*RateCard, error) {
	var result []*RateCard
	for _, rc := range m.RateCards {
		result = append(result, rc)
	}
	return result, nil
}

func (m *MockRepository) UpdateRateCard(ctx context.Context, companyID, rateCardID int64, updates RateCardUpdate) (*RateCard, error) {
	return m.RateCards[rateCardID], nil
}

func (m *MockRepository) DeactivateRateCard(ctx context.Context, companyID, rateCardID int64) error {
	return nil
}

func (m *MockRepository) GetApplicableRateCard(ctx context.Context, companyID int64, lookup RateLookup) (*RateCard, error) {
	for _, rc := range m.RateCards {
		return rc, nil
	}
	return nil, nil
}

func (m *MockRepository) ListApplicableRateCards(ctx context.Context, companyID int64, lookup RateLookup) ([]*RateCard, error) {
	var result []*RateCard
	for _, rc := range m.RateCards {
		result = append(result, rc)
	}
	return result, nil
}

func (m *MockRepository) CreateRateSurcharge(ctx context.Context, companyID, rateCardID int64, input CreateRateSurchargeInput) (*RateSurcharge, error) {
	return &RateSurcharge{ID: 1}, nil
}

func (m *MockRepository) ListRateSurcharges(ctx context.Context, rateCardID int64) ([]*RateSurcharge, error) {
	return make([]*RateSurcharge, 0), nil
}

func (m *MockRepository) DeleteRateSurcharge(ctx context.Context, surchargeID int64) error {
	return nil
}

func (m *MockRepository) CreateFreightCharge(ctx context.Context, charge *FreightCharge) (*FreightCharge, error) {
	charge.ID = int64(len(m.FreightCharges) + 1)
	m.FreightCharges[charge.ID] = charge
	return charge, nil
}

func (m *MockRepository) GetFreightCharge(ctx context.Context, companyID, chargeID int64) (*FreightCharge, error) {
	return m.FreightCharges[chargeID], nil
}

func (m *MockRepository) ListFreightCharges(ctx context.Context, companyID int64, filter FreightChargeFilter) ([]*FreightCharge, error) {
	var result []*FreightCharge
	for _, fc := range m.FreightCharges {
		result = append(result, fc)
	}
	return result, nil
}

func (m *MockRepository) UpdateFreightCharge(ctx context.Context, companyID, chargeID int64, updates FreightChargeUpdate) (*FreightCharge, error) {
	return m.FreightCharges[chargeID], nil
}

func (m *MockRepository) UpdateFreightChargeStatus(ctx context.Context, companyID, chargeID int64, status FreightChargeStatus) error {
	if charge, ok := m.FreightCharges[chargeID]; ok {
		charge.Status = status
	}
	return nil
}

func (m *MockRepository) CreateLandedCost(ctx context.Context, cost *LandedCost) (*LandedCost, error) {
	cost.ID = int64(len(m.LandedCosts) + 1)
	m.LandedCosts[cost.ID] = cost
	return cost, nil
}

func (m *MockRepository) GetLandedCost(ctx context.Context, companyID, costID int64) (*LandedCost, error) {
	return m.LandedCosts[costID], nil
}

func (m *MockRepository) ListLandedCosts(ctx context.Context, companyID int64, filter LandedCostFilter) ([]*LandedCost, error) {
	var result []*LandedCost
	for _, lc := range m.LandedCosts {
		result = append(result, lc)
	}
	return result, nil
}

func (m *MockRepository) GetLandedCostByShipment(ctx context.Context, companyID, shipmentID int64) (*LandedCost, error) {
	for _, lc := range m.LandedCosts {
		if lc.ShipmentID == shipmentID {
			return lc, nil
		}
	}
	return nil, nil
}

func (m *MockRepository) CreateCostCenter(ctx context.Context, companyID int64, input CreateCostCenterInput) (*CostCenter, error) {
	cc := &CostCenter{
		ID:              int64(len(m.CostCenters) + 1),
		CompanyID:       companyID,
		CostCenterCode:  input.CostCenterCode,
		CostCenterName:  input.CostCenterName,
		CostCenterType:  input.CostCenterType,
		GLAccount:       input.GLAccount,
		IsActive:        true,
	}
	m.CostCenters[cc.ID] = cc
	return cc, nil
}

func (m *MockRepository) GetCostCenter(ctx context.Context, companyID, costCenterID int64) (*CostCenter, error) {
	return m.CostCenters[costCenterID], nil
}

func (m *MockRepository) GetCostCenterByCode(ctx context.Context, companyID int64, code string) (*CostCenter, error) {
	for _, cc := range m.CostCenters {
		if cc.CostCenterCode == code {
			return cc, nil
		}
	}
	return nil, nil
}

func (m *MockRepository) ListCostCenters(ctx context.Context, companyID int64) ([]*CostCenter, error) {
	var result []*CostCenter
	for _, cc := range m.CostCenters {
		result = append(result, cc)
	}
	return result, nil
}

func (m *MockRepository) UpdateCostCenter(ctx context.Context, companyID, costCenterID int64, updates CostCenterUpdate) (*CostCenter, error) {
	return m.CostCenters[costCenterID], nil
}

func (m *MockRepository) CreateAuditLog(ctx context.Context, log *FreightAuditLog) error {
	m.AuditLogs = append(m.AuditLogs, log)
	return nil
}

func (m *MockRepository) ListAuditLogs(ctx context.Context, companyID, freightChargeID int64) ([]*FreightAuditLog, error) {
	var result []*FreightAuditLog
	for _, log := range m.AuditLogs {
		if log.FreightChargeID == freightChargeID {
			result = append(result, log)
		}
	}
	return result, nil
}
