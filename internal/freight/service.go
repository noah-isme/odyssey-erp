package freight

import (
	"context"
	"fmt"
	"time"

	accountingmoney "github.com/odyssey-erp/odyssey-erp/internal/accounting/money"
)

// ═══════════════════════════════════════════════════════════════════════════
// FREIGHT SERVICE
// ═══════════════════════════════════════════════════════════════════════════

type Service interface {
	// Rate Card operations
	CreateRateCard(ctx context.Context, input CreateRateCardInput) (*RateCard, error)
	GetRateCard(ctx context.Context, companyID, rateCardID int64) (*RateCard, error)
	ListRateCards(ctx context.Context, companyID int64, filter RateCardFilter) ([]*RateCard, error)
	UpdateRateCard(ctx context.Context, companyID, rateCardID int64, updates RateCardUpdate) (*RateCard, error)
	DeactivateRateCard(ctx context.Context, companyID, rateCardID int64) error

	// Rate Surcharge operations
	AddSurcharge(ctx context.Context, companyID, rateCardID int64, input CreateRateSurchargeInput) (*RateSurcharge, error)
	ListSurcharges(ctx context.Context, rateCardID int64) ([]*RateSurcharge, error)
	RemoveSurcharge(ctx context.Context, surchargeID int64) error

	// Freight Charge operations
	CalculateAndCreateFreightCharge(ctx context.Context, input CalculateFreightInput) (*FreightCharge, error)
	GetFreightCharge(ctx context.Context, companyID, chargeID int64) (*FreightCharge, error)
	ListFreightCharges(ctx context.Context, companyID int64, filter FreightChargeFilter) ([]*FreightCharge, error)
	UpdateFreightChargeInvoice(ctx context.Context, companyID, chargeID int64, invoiceNumber string, invoiceDate time.Time) (*FreightCharge, error)
	MarkFreightChargeInvoiced(ctx context.Context, companyID, chargeID int64) (*FreightCharge, error)
	MarkFreightChargePaid(ctx context.Context, companyID, chargeID int64) (*FreightCharge, error)

	// Landed Cost operations
	CalculateAndCreateLandedCost(ctx context.Context, input CalculateLandedCostInput) (*LandedCost, error)
	GetLandedCost(ctx context.Context, companyID, costID int64) (*LandedCost, error)
	GetLandedCostByShipment(ctx context.Context, companyID, shipmentID int64) (*LandedCost, error)
	ListLandedCosts(ctx context.Context, companyID int64, filter LandedCostFilter) ([]*LandedCost, error)

	// Cost Center operations
	CreateCostCenter(ctx context.Context, companyID int64, input CreateCostCenterInput) (*CostCenter, error)
	GetCostCenter(ctx context.Context, companyID, costCenterID int64) (*CostCenter, error)
	GetCostCenterByCode(ctx context.Context, companyID int64, code string) (*CostCenter, error)
	ListCostCenters(ctx context.Context, companyID int64) ([]*CostCenter, error)
	UpdateCostCenter(ctx context.Context, companyID, costCenterID int64, updates CostCenterUpdate) (*CostCenter, error)

	// Audit operations
	GetFreightAuditLog(ctx context.Context, companyID, freightChargeID int64) ([]*FreightAuditLog, error)
}

type freightService struct {
	repo      Repository
	calculator RateCalculator
}

func NewFreightService(repo Repository) Service {
	return &freightService{
		repo:      repo,
		calculator: NewRateCalculator(repo),
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// RATE CARD OPERATIONS
// ═══════════════════════════════════════════════════════════════════════════

func (fs *freightService) CreateRateCard(ctx context.Context, input CreateRateCardInput) (*RateCard, error) {
	if input.CompanyID == 0 {
		return nil, fmt.Errorf("company_id is required")
	}
	if input.OriginCity == "" || input.DestinationCity == "" {
		return nil, fmt.Errorf("origin_city and destination_city are required")
	}

	// Validate rate amounts
	if input.BaseRate.Amount == "" {
		return nil, fmt.Errorf("base_rate is required")
	}

	rateCard, err := fs.repo.CreateRateCard(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to create rate card: %w", err)
	}

	// Log audit event
	fs.logAudit(ctx, 0, AuditTypeCreated, nil, &input.BaseRate, "Rate card created", input.CreatedBy)

	return rateCard, nil
}

func (fs *freightService) GetRateCard(ctx context.Context, companyID, rateCardID int64) (*RateCard, error) {
	if companyID == 0 || rateCardID == 0 {
		return nil, fmt.Errorf("company_id and rate_card_id are required")
	}

	return fs.repo.GetRateCard(ctx, companyID, rateCardID)
}

func (fs *freightService) ListRateCards(ctx context.Context, companyID int64, filter RateCardFilter) ([]*RateCard, error) {
	if companyID == 0 {
		return nil, fmt.Errorf("company_id is required")
	}

	return fs.repo.ListRateCards(ctx, companyID, filter)
}

func (fs *freightService) UpdateRateCard(ctx context.Context, companyID, rateCardID int64, updates RateCardUpdate) (*RateCard, error) {
	if companyID == 0 || rateCardID == 0 {
		return nil, fmt.Errorf("company_id and rate_card_id are required")
	}

	rateCard, err := fs.repo.UpdateRateCard(ctx, companyID, rateCardID, updates)
	if err != nil {
		return nil, fmt.Errorf("failed to update rate card: %w", err)
	}

	return rateCard, nil
}

func (fs *freightService) DeactivateRateCard(ctx context.Context, companyID, rateCardID int64) error {
	if companyID == 0 || rateCardID == 0 {
		return fmt.Errorf("company_id and rate_card_id are required")
	}

	return fs.repo.DeactivateRateCard(ctx, companyID, rateCardID)
}

// ═══════════════════════════════════════════════════════════════════════════
// SURCHARGE OPERATIONS
// ═══════════════════════════════════════════════════════════════════════════

func (fs *freightService) AddSurcharge(ctx context.Context, companyID, rateCardID int64, input CreateRateSurchargeInput) (*RateSurcharge, error) {
	if companyID == 0 || rateCardID == 0 {
		return nil, fmt.Errorf("company_id and rate_card_id are required")
	}

	// Verify rate card exists
	rc, err := fs.repo.GetRateCard(ctx, companyID, rateCardID)
	if err != nil {
		return nil, fmt.Errorf("rate card not found: %w", err)
	}
	if rc == nil {
		return nil, fmt.Errorf("rate card not found")
	}

	return fs.repo.CreateRateSurcharge(ctx, companyID, rateCardID, input)
}

func (fs *freightService) ListSurcharges(ctx context.Context, rateCardID int64) ([]*RateSurcharge, error) {
	if rateCardID == 0 {
		return nil, fmt.Errorf("rate_card_id is required")
	}

	return fs.repo.ListRateSurcharges(ctx, rateCardID)
}

func (fs *freightService) RemoveSurcharge(ctx context.Context, surchargeID int64) error {
	if surchargeID == 0 {
		return fmt.Errorf("surcharge_id is required")
	}

	return fs.repo.DeleteRateSurcharge(ctx, surchargeID)
}

// ═══════════════════════════════════════════════════════════════════════════
// FREIGHT CHARGE OPERATIONS
// ═══════════════════════════════════════════════════════════════════════════

func (fs *freightService) CalculateAndCreateFreightCharge(ctx context.Context, input CalculateFreightInput) (*FreightCharge, error) {
	if input.CompanyID == 0 {
		return nil, fmt.Errorf("company_id is required")
	}

	// Calculate freight using rate calculator
	calcOutput, err := fs.calculator.CalculateFreight(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("freight calculation failed: %w", err)
	}

	// Create freight charge record
	charge := &FreightCharge{
		CompanyID:       input.CompanyID,
		ShipmentID:      input.ShipmentID,
		LoadID:          input.LoadID,
		CarrierID:       input.CarrierID,
		OriginCity:      input.OriginCity,
		DestinationCity: input.DestinationCity,
		ServiceLevel:    &input.ServiceLevel,
		WeightKg:        input.WeightKg,
		VolumeCbm:       input.VolumeCbm,
		BaseCharge:      calcOutput.BaseCharge,
		WeightCharge:    calcOutput.WeightCharge,
		VolumeCharge:    calcOutput.VolumeCharge,
		SurchargeTotal:  calcOutput.SurchargeTotal,
		FreightTotal:    calcOutput.FreightTotal,
		Currency:        calcOutput.Currency,
		Status:          FreightChargeStatusCalculated,
		CostCenterID:    input.CostCenterID,
		CreatedBy:       input.CompanyID, // TODO: pass user ID
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	created, err := fs.repo.CreateFreightCharge(ctx, charge)
	if err != nil {
		return nil, fmt.Errorf("failed to create freight charge: %w", err)
	}

	// Log audit event
	fs.logAudit(ctx, created.ID, AuditTypeCalculated, nil, &calcOutput.FreightTotal, "Freight charge calculated", input.CompanyID)

	return created, nil
}

func (fs *freightService) GetFreightCharge(ctx context.Context, companyID, chargeID int64) (*FreightCharge, error) {
	if companyID == 0 || chargeID == 0 {
		return nil, fmt.Errorf("company_id and charge_id are required")
	}

	return fs.repo.GetFreightCharge(ctx, companyID, chargeID)
}

func (fs *freightService) ListFreightCharges(ctx context.Context, companyID int64, filter FreightChargeFilter) ([]*FreightCharge, error) {
	if companyID == 0 {
		return nil, fmt.Errorf("company_id is required")
	}

	return fs.repo.ListFreightCharges(ctx, companyID, filter)
}

func (fs *freightService) UpdateFreightChargeInvoice(ctx context.Context, companyID, chargeID int64, invoiceNumber string, invoiceDate time.Time) (*FreightCharge, error) {
	if companyID == 0 || chargeID == 0 {
		return nil, fmt.Errorf("company_id and charge_id are required")
	}

	updates := FreightChargeUpdate{
		Status:        func() *FreightChargeStatus { s := FreightChargeStatusInvoiced; return &s }(),
		InvoiceNumber: &invoiceNumber,
		InvoiceDate:   &invoiceDate,
	}

	charge, err := fs.repo.UpdateFreightCharge(ctx, companyID, chargeID, updates)
	if err != nil {
		return nil, fmt.Errorf("failed to update freight charge: %w", err)
	}

	fs.logAudit(ctx, chargeID, AuditTypeInvoiced, nil, &charge.FreightTotal, fmt.Sprintf("Invoice %s", invoiceNumber), companyID)

	return charge, nil
}

func (fs *freightService) MarkFreightChargeInvoiced(ctx context.Context, companyID, chargeID int64) (*FreightCharge, error) {
	if companyID == 0 || chargeID == 0 {
		return nil, fmt.Errorf("company_id and charge_id are required")
	}

	err := fs.repo.UpdateFreightChargeStatus(ctx, companyID, chargeID, FreightChargeStatusInvoiced)
	if err != nil {
		return nil, fmt.Errorf("failed to mark charge as invoiced: %w", err)
	}

	return fs.repo.GetFreightCharge(ctx, companyID, chargeID)
}

func (fs *freightService) MarkFreightChargePaid(ctx context.Context, companyID, chargeID int64) (*FreightCharge, error) {
	if companyID == 0 || chargeID == 0 {
		return nil, fmt.Errorf("company_id and charge_id are required")
	}

	err := fs.repo.UpdateFreightChargeStatus(ctx, companyID, chargeID, FreightChargeStatusPaid)
	if err != nil {
		return nil, fmt.Errorf("failed to mark charge as paid: %w", err)
	}

	charge, _ := fs.repo.GetFreightCharge(ctx, companyID, chargeID)
	fs.logAudit(ctx, chargeID, AuditTypePosted, nil, &charge.FreightTotal, "Freight charge paid", companyID)

	return charge, nil
}

// ═══════════════════════════════════════════════════════════════════════════
// LANDED COST OPERATIONS
// ═══════════════════════════════════════════════════════════════════════════

func (fs *freightService) CalculateAndCreateLandedCost(ctx context.Context, input CalculateLandedCostInput) (*LandedCost, error) {
	if input.CompanyID == 0 {
		return nil, fmt.Errorf("company_id is required")
	}

	// Calculate landed cost using calculator
	cost, err := fs.calculator.CalculateLandedCost(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("landed cost calculation failed: %w", err)
	}

	// Create landed cost record
	created, err := fs.repo.CreateLandedCost(ctx, cost)
	if err != nil {
		return nil, fmt.Errorf("failed to create landed cost: %w", err)
	}

	return created, nil
}

func (fs *freightService) GetLandedCost(ctx context.Context, companyID, costID int64) (*LandedCost, error) {
	if companyID == 0 || costID == 0 {
		return nil, fmt.Errorf("company_id and cost_id are required")
	}

	return fs.repo.GetLandedCost(ctx, companyID, costID)
}

func (fs *freightService) GetLandedCostByShipment(ctx context.Context, companyID, shipmentID int64) (*LandedCost, error) {
	if companyID == 0 || shipmentID == 0 {
		return nil, fmt.Errorf("company_id and shipment_id are required")
	}

	return fs.repo.GetLandedCostByShipment(ctx, companyID, shipmentID)
}

func (fs *freightService) ListLandedCosts(ctx context.Context, companyID int64, filter LandedCostFilter) ([]*LandedCost, error) {
	if companyID == 0 {
		return nil, fmt.Errorf("company_id is required")
	}

	return fs.repo.ListLandedCosts(ctx, companyID, filter)
}

// ═══════════════════════════════════════════════════════════════════════════
// COST CENTER OPERATIONS
// ═══════════════════════════════════════════════════════════════════════════

func (fs *freightService) CreateCostCenter(ctx context.Context, companyID int64, input CreateCostCenterInput) (*CostCenter, error) {
	if companyID == 0 {
		return nil, fmt.Errorf("company_id is required")
	}
	if input.CostCenterCode == "" || input.CostCenterName == "" {
		return nil, fmt.Errorf("cost_center_code and cost_center_name are required")
	}

	return fs.repo.CreateCostCenter(ctx, companyID, input)
}

func (fs *freightService) GetCostCenter(ctx context.Context, companyID, costCenterID int64) (*CostCenter, error) {
	if companyID == 0 || costCenterID == 0 {
		return nil, fmt.Errorf("company_id and cost_center_id are required")
	}

	return fs.repo.GetCostCenter(ctx, companyID, costCenterID)
}

func (fs *freightService) GetCostCenterByCode(ctx context.Context, companyID int64, code string) (*CostCenter, error) {
	if companyID == 0 || code == "" {
		return nil, fmt.Errorf("company_id and cost_center_code are required")
	}

	return fs.repo.GetCostCenterByCode(ctx, companyID, code)
}

func (fs *freightService) ListCostCenters(ctx context.Context, companyID int64) ([]*CostCenter, error) {
	if companyID == 0 {
		return nil, fmt.Errorf("company_id is required")
	}

	return fs.repo.ListCostCenters(ctx, companyID)
}

func (fs *freightService) UpdateCostCenter(ctx context.Context, companyID, costCenterID int64, updates CostCenterUpdate) (*CostCenter, error) {
	if companyID == 0 || costCenterID == 0 {
		return nil, fmt.Errorf("company_id and cost_center_id are required")
	}

	return fs.repo.UpdateCostCenter(ctx, companyID, costCenterID, updates)
}

// ═══════════════════════════════════════════════════════════════════════════
// AUDIT OPERATIONS
// ═══════════════════════════════════════════════════════════════════════════

func (fs *freightService) GetFreightAuditLog(ctx context.Context, companyID, freightChargeID int64) ([]*FreightAuditLog, error) {
	if companyID == 0 || freightChargeID == 0 {
		return nil, fmt.Errorf("company_id and freight_charge_id are required")
	}

	return fs.repo.ListAuditLogs(ctx, companyID, freightChargeID)
}

func (fs *freightService) logAudit(ctx context.Context, chargeID int64, auditType AuditType, oldVal, newVal *accountingmoney.Money, reason string, userID int64) {
	if chargeID == 0 || userID == 0 {
		return
	}

	log := &FreightAuditLog{
		CompanyID:       userID,
		FreightChargeID: chargeID,
		AuditType:       auditType,
		OldValue:        oldVal,
		NewValue:        newVal,
		Reason:          &reason,
		UserID:          userID,
		CreatedAt:       time.Now(),
	}

	_ = fs.repo.CreateAuditLog(ctx, log)
}
