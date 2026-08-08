package freight

import (
	"context"
	"fmt"
	"time"
)

type MockRepository struct {
	RateCards       map[int64]*RateCard
	Surcharges      map[int64]*RateSurcharge
	Charges         map[int64]*FreightCharge
	LandedCosts     map[int64]*LandedCost
	CostCenters     map[int64]*CostCenter
	AuditLogs       map[int64][]*FreightAuditLog
	
	nextRateCardID  int64
	nextSurchargeID int64
	nextChargeID    int64
	nextLandedCostID int64
	nextCostCenterID int64
}

func NewMockRepository() *MockRepository {
	return &MockRepository{
		RateCards:       make(map[int64]*RateCard),
		Surcharges:      make(map[int64]*RateSurcharge),
		Charges:         make(map[int64]*FreightCharge),
		LandedCosts:     make(map[int64]*LandedCost),
		CostCenters:     make(map[int64]*CostCenter),
		AuditLogs:       make(map[int64][]*FreightAuditLog),
		nextRateCardID:  1,
		nextSurchargeID: 1,
		nextChargeID:    1,
		nextLandedCostID: 1,
		nextCostCenterID: 1,
	}
}

func (m *MockRepository) CreateRateCard(ctx context.Context, input CreateRateCardInput) (*RateCard, error) {
	rc := &RateCard{
		ID:                 m.nextRateCardID,
		CompanyID:          input.CompanyID,
		CarrierID:          input.CarrierID,
		OriginCity:         input.OriginCity,
		OriginCountry:      input.OriginCountry,
		DestinationCity:    input.DestinationCity,
		DestinationCountry: input.DestinationCountry,
		ServiceLevel:       input.ServiceLevel,
		BaseRate:           input.BaseRate,
		Currency:           input.Currency,
		EffectiveDate:      input.EffectiveDate,
		ExpirationDate:     input.ExpirationDate,
		IsActive:           true,
		CreatedBy:          input.CreatedBy,
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
	}
	if input.PerKgRate != nil {
		rc.PerKgRate = input.PerKgRate
	}
	if input.PerCbmRate != nil {
		rc.PerCbmRate = input.PerCbmRate
	}
	
	m.RateCards[rc.ID] = rc
	m.nextRateCardID++
	return rc, nil
}

func (m *MockRepository) GetRateCard(ctx context.Context, companyID, rateCardID int64) (*RateCard, error) {
	if rc, ok := m.RateCards[rateCardID]; ok && rc.CompanyID == companyID {
		return rc, nil
	}
	return nil, fmt.Errorf("rate card not found")
}

func (m *MockRepository) ListRateCards(ctx context.Context, companyID int64, filter RateCardFilter) ([]*RateCard, error) {
	return nil, nil
}

func (m *MockRepository) UpdateRateCard(ctx context.Context, companyID, rateCardID int64, updates RateCardUpdate) (*RateCard, error) {
	return nil, nil
}

func (m *MockRepository) DeactivateRateCard(ctx context.Context, companyID, rateCardID int64) error {
	return nil
}

func (m *MockRepository) GetApplicableRateCard(ctx context.Context, companyID int64, lookup RateLookup) (*RateCard, error) {
	for _, rc := range m.RateCards {
		if rc.CompanyID == companyID && rc.OriginCity == lookup.OriginCity && rc.DestinationCity == lookup.DestinationCity && rc.ServiceLevel == lookup.ServiceLevel {
			return rc, nil
		}
	}
	return nil, fmt.Errorf("no applicable rate card found")
}

func (m *MockRepository) ListApplicableRateCards(ctx context.Context, companyID int64, lookup RateLookup) ([]*RateCard, error) {
	return nil, nil
}

func (m *MockRepository) CreateRateSurcharge(ctx context.Context, companyID, rateCardID int64, input CreateRateSurchargeInput) (*RateSurcharge, error) {
	s := &RateSurcharge{
		ID:            m.nextSurchargeID,
		CompanyID:     companyID,
		RateCardID:    rateCardID,
		SurchargeType: input.SurchargeType,
		SurchargeName: input.SurchargeName,
		SurchargeAmount: input.SurchargeAmount,
		SurchargePercent: input.SurchargePercent,
		EffectiveDate: input.EffectiveDate,
		ExpirationDate: input.ExpirationDate,
		CreatedAt:     time.Now(),
	}
	m.Surcharges[s.ID] = s
	m.nextSurchargeID++
	return s, nil
}

func (m *MockRepository) ListRateSurcharges(ctx context.Context, rateCardID int64) ([]*RateSurcharge, error) {
	var list []*RateSurcharge
	for _, s := range m.Surcharges {
		if s.RateCardID == rateCardID {
			list = append(list, s)
		}
	}
	return list, nil
}

func (m *MockRepository) DeleteRateSurcharge(ctx context.Context, surchargeID int64) error {
	return nil
}

func (m *MockRepository) CreateFreightCharge(ctx context.Context, charge *FreightCharge) (*FreightCharge, error) {
	charge.ID = m.nextChargeID
	charge.CreatedAt = time.Now()
	charge.UpdatedAt = time.Now()
	m.Charges[charge.ID] = charge
	m.nextChargeID++
	return charge, nil
}

func (m *MockRepository) GetFreightCharge(ctx context.Context, companyID, chargeID int64) (*FreightCharge, error) {
	if c, ok := m.Charges[chargeID]; ok && c.CompanyID == companyID {
		return c, nil
	}
	return nil, fmt.Errorf("freight charge not found")
}

func (m *MockRepository) ListFreightCharges(ctx context.Context, companyID int64, filter FreightChargeFilter) ([]*FreightCharge, error) {
	var list []*FreightCharge
	for _, c := range m.Charges {
		if c.CompanyID == companyID {
			list = append(list, c)
		}
	}
	return list, nil
}

func (m *MockRepository) UpdateFreightCharge(ctx context.Context, companyID, chargeID int64, updates FreightChargeUpdate) (*FreightCharge, error) {
	c, err := m.GetFreightCharge(ctx, companyID, chargeID)
	if err != nil {
		return nil, err
	}
	if updates.Status != nil {
		c.Status = *updates.Status
	}
	if updates.InvoiceNumber != nil {
		c.InvoiceNumber = updates.InvoiceNumber
	}
	if updates.InvoiceDate != nil {
		c.InvoiceDate = updates.InvoiceDate
	}
	if updates.GLPostingID != nil {
		c.GLPostingID = updates.GLPostingID
	}
	if updates.Notes != nil {
		c.Notes = updates.Notes
	}
	return c, nil
}

func (m *MockRepository) UpdateFreightChargeStatus(ctx context.Context, companyID, chargeID int64, status FreightChargeStatus) error {
	c, err := m.GetFreightCharge(ctx, companyID, chargeID)
	if err != nil {
		return err
	}
	c.Status = status
	return nil
}

func (m *MockRepository) CreateLandedCost(ctx context.Context, cost *LandedCost) (*LandedCost, error) {
	cost.ID = m.nextLandedCostID
	m.LandedCosts[cost.ID] = cost
	m.nextLandedCostID++
	return cost, nil
}

func (m *MockRepository) GetLandedCost(ctx context.Context, companyID, costID int64) (*LandedCost, error) {
	return nil, nil
}

func (m *MockRepository) ListLandedCosts(ctx context.Context, companyID int64, filter LandedCostFilter) ([]*LandedCost, error) {
	return nil, nil
}

func (m *MockRepository) GetLandedCostByShipment(ctx context.Context, companyID, shipmentID int64) (*LandedCost, error) {
	return nil, nil
}

func (m *MockRepository) CreateCostCenter(ctx context.Context, companyID int64, input CreateCostCenterInput) (*CostCenter, error) {
	cc := &CostCenter{
		ID:             m.nextCostCenterID,
		CompanyID:      companyID,
		CostCenterCode: input.CostCenterCode,
		CostCenterName: input.CostCenterName,
		CostCenterType: input.CostCenterType,
		IsActive:       true,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	m.CostCenters[cc.ID] = cc
	m.nextCostCenterID++
	return cc, nil
}

func (m *MockRepository) GetCostCenter(ctx context.Context, companyID, costCenterID int64) (*CostCenter, error) {
	if cc, ok := m.CostCenters[costCenterID]; ok && cc.CompanyID == companyID {
		return cc, nil
	}
	return nil, fmt.Errorf("cost center not found")
}

func (m *MockRepository) GetCostCenterByCode(ctx context.Context, companyID int64, code string) (*CostCenter, error) {
	for _, cc := range m.CostCenters {
		if cc.CompanyID == companyID && cc.CostCenterCode == code {
			return cc, nil
		}
	}
	return nil, fmt.Errorf("cost center not found")
}

func (m *MockRepository) ListCostCenters(ctx context.Context, companyID int64) ([]*CostCenter, error) {
	return nil, nil
}

func (m *MockRepository) UpdateCostCenter(ctx context.Context, companyID, costCenterID int64, updates CostCenterUpdate) (*CostCenter, error) {
	return nil, nil
}

func (m *MockRepository) CreateAuditLog(ctx context.Context, log *FreightAuditLog) error {
	return nil
}

func (m *MockRepository) ListAuditLogs(ctx context.Context, companyID, freightChargeID int64) ([]*FreightAuditLog, error) {
	return nil, nil
}
