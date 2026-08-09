package freight

import (
	"context"
	"fmt"
	"math/big"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	accountingmoney "github.com/odyssey-erp/odyssey-erp/internal/accounting/money"
	"github.com/odyssey-erp/odyssey-erp/internal/sqlc"
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
	CarrierID         *int64
	OriginCity        *string
	DestinationCity   *string
	ServiceLevel      *ServiceLevel
	IncludeInactive   bool
	EffectiveDateFrom *time.Time
	EffectiveDateTo   *time.Time
	Limit             int
	Offset            int
}

type RateCardUpdate struct {
	BaseRate       *accountingmoney.Money
	PerKgRate      *accountingmoney.Money
	PerCbmRate     *accountingmoney.Money
	ExpirationDate *time.Time
	IsActive       *bool
}

type RateLookup struct {
	CarrierID       *int64
	OriginCity      string
	DestinationCity string
	ServiceLevel    ServiceLevel
	WeightKg        *accountingmoney.Money
	VolumeCbm       *accountingmoney.Money
	AsOfDate        time.Time
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
	ShipmentID      *int64
	LoadID          *int64
	CarrierID       *int64
	Status          *FreightChargeStatus
	OriginCity      *string
	DestinationCity *string
	CreatedAfter    *time.Time
	CreatedBefore   *time.Time
	Limit           int
	Offset          int
}

type FreightChargeUpdate struct {
	Status        *FreightChargeStatus
	InvoiceNumber *string
	InvoiceDate   *time.Time
	GLPostingID   *int64
	Notes         *string
}

type LandedCostFilter struct {
	ShipmentID    *int64
	LoadID        *int64
	POID          *int64
	CreatedAfter  *time.Time
	CreatedBefore *time.Time
	Limit         int
	Offset        int
}

type CreateCostCenterInput struct {
	CostCenterCode string
	CostCenterName string
	CostCenterType CostCenterType
	WarehouseID    *int64
	GLAccount      *string
	ManagerID      *int64
}

type CostCenterUpdate struct {
	CostCenterName *string
	GLAccount      *string
	ManagerID      *int64
	IsActive       *bool
}

type postgresRepository struct {
	q *sqlc.Queries
}

func NewPostgresRepository(q *sqlc.Queries) Repository {
	return &postgresRepository{q: q}
}

// Helpers
func moneyToNumeric(m *accountingmoney.Money) pgtype.Numeric {
	if m == nil {
		return pgtype.Numeric{}
	}
	var num pgtype.Numeric
	_ = num.Scan(m.Amount)
	return num
}

func moneyToNumericVal(m accountingmoney.Money) pgtype.Numeric {
	var num pgtype.Numeric
	_ = num.Scan(m.Amount)
	return num
}

func numericToMoney(n pgtype.Numeric, scale int) *accountingmoney.Money {
	if !n.Valid || n.Int == nil || n.NaN || n.InfinityModifier != pgtype.Finite {
		return nil
	}
	m, err := accountingmoney.Parse(numericExactString(n, scale), scale)
	if err != nil {
		return nil
	}
	return &m
}

func numericToMoneyVal(n pgtype.Numeric, scale int) accountingmoney.Money {
	if !n.Valid || n.Int == nil || n.NaN || n.InfinityModifier != pgtype.Finite {
		m, _ := accountingmoney.Parse("0", scale)
		return m
	}
	m, err := accountingmoney.Parse(numericExactString(n, scale), scale)
	if err != nil {
		m, _ = accountingmoney.Parse("0", scale)
	}
	return m
}

func numericExactString(n pgtype.Numeric, scale int) string {
	rat := new(big.Rat).SetInt(n.Int)
	if n.Exp >= 0 {
		factor := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(n.Exp)), nil)
		rat.Mul(rat, new(big.Rat).SetInt(factor))
	} else {
		factor := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(-n.Exp)), nil)
		rat.Quo(rat, new(big.Rat).SetInt(factor))
	}
	return rat.FloatString(scale)
}

func timeToTimestamp(t *time.Time) pgtype.Timestamp {
	if t == nil {
		return pgtype.Timestamp{}
	}
	return pgtype.Timestamp{Time: *t, Valid: true}
}

func timeToDate(t *time.Time) pgtype.Date {
	if t == nil {
		return pgtype.Date{}
	}
	return pgtype.Date{Time: *t, Valid: true}
}

func timeToDateVal(t time.Time) pgtype.Date {
	return pgtype.Date{Time: t, Valid: true}
}

func int64ToInt8(i *int64) pgtype.Int8 {
	if i == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *i, Valid: true}
}

func stringToText(s *string) pgtype.Text {
	if s == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *s, Valid: true}
}

// Map sqlc RateCard to domain RateCard
func mapRateCard(s sqlc.RateCard) *RateCard {
	var carrierID *int64
	if s.CarrierID.Valid {
		carrierID = &s.CarrierID.Int64
	}
	var expDate *time.Time
	if s.ExpirationDate.Valid {
		expDate = &s.ExpirationDate.Time
	}

	return &RateCard{
		ID:                 s.ID,
		CompanyID:          s.CompanyID,
		CarrierID:          carrierID,
		OriginCity:         s.OriginCity,
		OriginCountry:      s.OriginCountry,
		DestinationCity:    s.DestinationCity,
		DestinationCountry: s.DestinationCountry,
		ServiceLevel:       ServiceLevel(s.ServiceLevel),
		MinWeightKg:        numericToMoney(s.MinWeight, 4),
		MaxWeightKg:        numericToMoney(s.MaxWeight, 4),
		BaseRate:           numericToMoneyVal(s.BaseRate, 2),
		PerKgRate:          numericToMoney(s.PerKgRate, 2),
		PerCbmRate:         numericToMoney(s.PerCbmRate, 2),
		Currency:           s.Currency,
		EffectiveDate:      s.EffectiveDate.Time,
		ExpirationDate:     expDate,
		IsActive:           s.IsActive,
		CreatedBy:          s.CreatedBy,
		CreatedAt:          s.CreatedAt.Time,
		UpdatedAt:          s.UpdatedAt.Time,
	}
}

func (r *postgresRepository) CreateRateCard(ctx context.Context, input CreateRateCardInput) (*RateCard, error) {
	arg := sqlc.CreateRateCardParams{
		CompanyID:          input.CompanyID,
		CarrierID:          int64ToInt8(input.CarrierID),
		OriginCity:         input.OriginCity,
		OriginCountry:      input.OriginCountry,
		DestinationCity:    input.DestinationCity,
		DestinationCountry: input.DestinationCountry,
		ServiceLevel:       string(input.ServiceLevel),
		MinWeight:          moneyToNumeric(input.MinWeightKg),
		MaxWeight:          moneyToNumeric(input.MaxWeightKg),
		BaseRate:           moneyToNumericVal(input.BaseRate),
		PerKgRate:          moneyToNumeric(input.PerKgRate),
		PerCbmRate:         moneyToNumeric(input.PerCbmRate),
		Currency:           input.Currency,
		EffectiveDate:      timeToDateVal(input.EffectiveDate),
		ExpirationDate:     timeToDate(input.ExpirationDate),
		IsActive:           true,
		CreatedBy:          input.CreatedBy,
	}
	res, err := r.q.CreateRateCard(ctx, arg)
	if err != nil {
		return nil, err
	}
	return mapRateCard(res), nil
}

func (r *postgresRepository) GetRateCard(ctx context.Context, companyID, rateCardID int64) (*RateCard, error) {
	res, err := r.q.GetRateCard(ctx, sqlc.GetRateCardParams{
		ID:        rateCardID,
		CompanyID: companyID,
	})
	if err != nil {
		return nil, err
	}
	return mapRateCard(res), nil
}

func (r *postgresRepository) ListRateCards(ctx context.Context, companyID int64, filter RateCardFilter) ([]*RateCard, error) {
	var carrierID int64
	if filter.CarrierID != nil {
		carrierID = *filter.CarrierID
	}
	var origin string
	if filter.OriginCity != nil {
		origin = *filter.OriginCity
	}
	var dest string
	if filter.DestinationCity != nil {
		dest = *filter.DestinationCity
	}
	var srv string
	if filter.ServiceLevel != nil {
		srv = string(*filter.ServiceLevel)
	}

	limit := int32(filter.Limit)
	if limit == 0 {
		limit = 100
	}
	offset := int32(filter.Offset)

	res, err := r.q.ListRateCards(ctx, sqlc.ListRateCardsParams{
		CompanyID: companyID,
		Column2:   carrierID,
		Column3:   origin,
		Column4:   dest,
		Column5:   srv,
		Column6:   !filter.IncludeInactive,
		Column7:   timeToDate(filter.EffectiveDateFrom),
		Column8:   timeToDate(filter.EffectiveDateTo),
		Limit:     limit,
		Offset:    offset,
	})
	if err != nil {
		return nil, err
	}

	var ret []*RateCard
	for _, x := range res {
		ret = append(ret, mapRateCard(x))
	}
	return ret, nil
}

func mapFreightCharge(s sqlc.FreightCharge) *FreightCharge {
	var carrierID *int64
	if s.CarrierID.Valid {
		carrierID = &s.CarrierID.Int64
	}
	var loadID *int64
	if s.LoadID.Valid {
		loadID = &s.LoadID.Int64
	}
	var shipmentID *int64
	if s.ShipmentID.Valid {
		shipmentID = &s.ShipmentID.Int64
	}
	var rateCardID *int64
	if s.RateCardID.Valid {
		rateCardID = &s.RateCardID.Int64
	}
	var srvLevel *ServiceLevel
	if s.ServiceLevel.Valid {
		l := ServiceLevel(s.ServiceLevel.String)
		srvLevel = &l
	}
	var invoiceNum *string
	if s.InvoiceNumber.Valid {
		invoiceNum = &s.InvoiceNumber.String
	}
	var notes *string
	if s.Notes.Valid {
		notes = &s.Notes.String
	}
	var invoiceDate *time.Time
	if s.InvoiceDate.Valid {
		invoiceDate = &s.InvoiceDate.Time
	}
	var glPost *int64
	if s.GlPostingID.Valid {
		glPost = &s.GlPostingID.Int64
	}
	var cc *int64
	if s.CostCenterID.Valid {
		cc = &s.CostCenterID.Int64
	}

	return &FreightCharge{
		ID:              s.ID,
		CompanyID:       s.CompanyID,
		ShipmentID:      shipmentID,
		LoadID:          loadID,
		CarrierID:       carrierID,
		RateCardID:      rateCardID,
		OriginCity:      s.OriginCity,
		DestinationCity: s.DestinationCity,
		ServiceLevel:    srvLevel,
		WeightKg:        numericToMoney(s.WeightKg, 4),
		VolumeCbm:       numericToMoney(s.VolumeCbm, 4),
		BaseCharge:      numericToMoneyVal(s.BaseCharge, 2),
		WeightCharge:    numericToMoney(s.WeightCharge, 2),
		VolumeCharge:    numericToMoney(s.VolumeCharge, 2),
		SurchargeTotal:  numericToMoney(s.SurchargeTotal, 2),
		FreightTotal:    numericToMoneyVal(s.FreightTotal, 2),
		Currency:        s.Currency,
		Status:          FreightChargeStatus(s.Status),
		InvoiceNumber:   invoiceNum,
		InvoiceDate:     invoiceDate,
		GLPostingID:     glPost,
		CostCenterID:    cc,
		Notes:           notes,
		CreatedBy:       s.CreatedBy,
		CreatedAt:       s.CreatedAt.Time,
		UpdatedAt:       s.UpdatedAt.Time,
	}
}

func (r *postgresRepository) CreateFreightCharge(ctx context.Context, charge *FreightCharge) (*FreightCharge, error) {
	arg := sqlc.CreateFreightChargeParams{
		CompanyID:       charge.CompanyID,
		ShipmentID:      int64ToInt8(charge.ShipmentID),
		LoadID:          int64ToInt8(charge.LoadID),
		CarrierID:       int64ToInt8(charge.CarrierID),
		RateCardID:      int64ToInt8(charge.RateCardID),
		OriginCity:      charge.OriginCity,
		DestinationCity: charge.DestinationCity,
		ServiceLevel:    pgtype.Text{},
		WeightKg:        moneyToNumeric(charge.WeightKg),
		VolumeCbm:       moneyToNumeric(charge.VolumeCbm),
		BaseCharge:      moneyToNumericVal(charge.BaseCharge),
		WeightCharge:    moneyToNumeric(charge.WeightCharge),
		VolumeCharge:    moneyToNumeric(charge.VolumeCharge),
		SurchargeTotal:  moneyToNumeric(charge.SurchargeTotal),
		FreightTotal:    moneyToNumericVal(charge.FreightTotal),
		Currency:        charge.Currency,
		Status:          string(charge.Status),
		CostCenterID:    int64ToInt8(charge.CostCenterID),
		Notes:           stringToText(charge.Notes),
		CreatedBy:       charge.CreatedBy,
	}
	if charge.ServiceLevel != nil {
		arg.ServiceLevel = pgtype.Text{String: string(*charge.ServiceLevel), Valid: true}
	}

	res, err := r.q.CreateFreightCharge(ctx, arg)
	if err != nil {
		return nil, err
	}
	return mapFreightCharge(res), nil
}

func (r *postgresRepository) GetFreightCharge(ctx context.Context, companyID, chargeID int64) (*FreightCharge, error) {
	res, err := r.q.GetFreightCharge(ctx, sqlc.GetFreightChargeParams{
		ID:        chargeID,
		CompanyID: companyID,
	})
	if err != nil {
		return nil, err
	}
	return mapFreightCharge(res), nil
}

func mapRateSurcharge(s sqlc.RateSurcharge) *RateSurcharge {
	var expirationDate *time.Time
	if s.ExpirationDate.Valid {
		expirationDate = &s.ExpirationDate.Time
	}
	var surchargePercent *float64
	if s.SurchargePercent.Valid {
		value, err := s.SurchargePercent.Float64Value()
		if err == nil && value.Valid {
			surchargePercent = &value.Float64
		}
	}
	return &RateSurcharge{
		ID:               s.ID,
		CompanyID:        s.CompanyID,
		RateCardID:       s.RateCardID,
		SurchargeType:    SurchargeType(s.SurchargeType),
		SurchargeName:    s.SurchargeName,
		SurchargeAmount:  numericToMoney(s.SurchargeAmount, 2),
		SurchargePercent: surchargePercent,
		EffectiveDate:    s.EffectiveDate.Time,
		ExpirationDate:   expirationDate,
		CreatedAt:        s.CreatedAt.Time,
	}
}

func mapLandedCost(s sqlc.LandedCost) *LandedCost {
	var loadID, poID *int64
	if s.LoadID.Valid {
		loadID = &s.LoadID.Int64
	}
	if s.PoID.Valid {
		poID = &s.PoID.Int64
	}
	return &LandedCost{
		ID:               s.ID,
		CompanyID:        s.CompanyID,
		ShipmentID:       s.ShipmentID,
		LoadID:           loadID,
		FreightChargeID:  s.FreightChargeID,
		POID:             poID,
		ProductCost:      numericToMoneyVal(s.ProductCost, 2),
		FreightCost:      numericToMoneyVal(s.FreightCost, 2),
		DutyCost:         numericToMoney(s.DutyCost, 2),
		TaxCost:          numericToMoney(s.TaxCost, 2),
		InsuranceCost:    numericToMoney(s.InsuranceCost, 2),
		OtherCost:        numericToMoney(s.OtherCost, 2),
		TotalLandedCost:  numericToMoneyVal(s.TotalLandedCost, 2),
		CostPerUnit:      numericToMoney(s.CostPerUnit, 2),
		Currency:         s.Currency,
		AllocationMethod: AllocationMethod(s.AllocationMethod),
		CreatedAt:        s.CreatedAt.Time,
		UpdatedAt:        s.UpdatedAt.Time,
	}
}

func mapFreightAuditLog(s sqlc.FreightAuditLog) *FreightAuditLog {
	var reason *string
	if s.Reason.Valid {
		reason = &s.Reason.String
	}
	return &FreightAuditLog{
		ID:              s.ID,
		CompanyID:       s.CompanyID,
		FreightChargeID: s.FreightChargeID,
		AuditType:       AuditType(s.AuditType),
		OldValue:        numericToMoney(s.OldValue, 2),
		NewValue:        numericToMoney(s.NewValue, 2),
		Reason:          reason,
		UserID:          s.UserID,
		CreatedAt:       s.CreatedAt.Time,
	}
}

func mapCostCenterValues(
	id, companyID int64,
	departmentID pgtype.Int8,
	code, name, centerType string,
	warehouseID pgtype.Int8,
	glAccount pgtype.Text,
	managerID pgtype.Int8,
	isActive bool,
	createdAt, updatedAt pgtype.Timestamptz,
) *CostCenter {
	_ = departmentID
	var warehouse, manager *int64
	if warehouseID.Valid {
		warehouse = &warehouseID.Int64
	}
	if managerID.Valid {
		manager = &managerID.Int64
	}
	var account *string
	if glAccount.Valid {
		account = &glAccount.String
	}
	return &CostCenter{
		ID:             id,
		CompanyID:      companyID,
		CostCenterCode: code,
		CostCenterName: name,
		CostCenterType: CostCenterType(centerType),
		WarehouseID:    warehouse,
		GLAccount:      account,
		ManagerID:      manager,
		IsActive:       isActive,
		CreatedAt:      createdAt.Time,
		UpdatedAt:      updatedAt.Time,
	}
}

func (r *postgresRepository) UpdateRateCard(ctx context.Context, companyID, rateCardID int64, updates RateCardUpdate) (*RateCard, error) {
	current, err := r.GetRateCard(ctx, companyID, rateCardID)
	if err != nil {
		return nil, err
	}
	active := current.IsActive
	if updates.IsActive != nil {
		active = *updates.IsActive
	}
	res, err := r.q.UpdateRateCard(ctx, sqlc.UpdateRateCardParams{
		ID:             rateCardID,
		CompanyID:      companyID,
		BaseRate:       moneyToNumeric(updates.BaseRate),
		PerKgRate:      moneyToNumeric(updates.PerKgRate),
		PerCbmRate:     moneyToNumeric(updates.PerCbmRate),
		ExpirationDate: timeToDate(updates.ExpirationDate),
		IsActive:       active,
	})
	if err != nil {
		return nil, err
	}
	return mapRateCard(res), nil
}
func (r *postgresRepository) DeactivateRateCard(ctx context.Context, companyID, rateCardID int64) error {
	return r.q.DeactivateRateCard(ctx, sqlc.DeactivateRateCardParams{ID: rateCardID, CompanyID: companyID})
}
func (r *postgresRepository) GetApplicableRateCard(ctx context.Context, companyID int64, lookup RateLookup) (*RateCard, error) {
	cards, err := r.ListApplicableRateCards(ctx, companyID, lookup)
	if err != nil {
		return nil, err
	}
	if len(cards) == 0 {
		return nil, fmt.Errorf("no applicable rate card found")
	}
	return cards[0], nil
}
func (r *postgresRepository) ListApplicableRateCards(ctx context.Context, companyID int64, lookup RateLookup) ([]*RateCard, error) {
	asOf := lookup.AsOfDate
	if asOf.IsZero() {
		asOf = time.Now()
	}
	asOfDate := time.Date(asOf.Year(), asOf.Month(), asOf.Day(), 0, 0, 0, 0, asOf.Location())
	origin, destination, serviceLevel := lookup.OriginCity, lookup.DestinationCity, lookup.ServiceLevel
	filter := RateCardFilter{
		CarrierID:       lookup.CarrierID,
		OriginCity:      &origin,
		DestinationCity: &destination,
		ServiceLevel:    &serviceLevel,
		EffectiveDateTo: &asOfDate,
		Limit:           1000,
	}
	cards, err := r.ListRateCards(ctx, companyID, filter)
	if err != nil {
		return nil, err
	}
	applicable := make([]*RateCard, 0, len(cards))
	for _, card := range cards {
		if !card.IsActive || card.EffectiveDate.After(asOfDate) {
			continue
		}
		if card.ExpirationDate != nil && card.ExpirationDate.Before(asOfDate) {
			continue
		}
		if lookup.WeightKg != nil {
			if card.MinWeightKg != nil && lookup.WeightKg.Cmp(*card.MinWeightKg) < 0 {
				continue
			}
			if card.MaxWeightKg != nil && lookup.WeightKg.Cmp(*card.MaxWeightKg) > 0 {
				continue
			}
		}
		applicable = append(applicable, card)
	}
	return applicable, nil
}
func (r *postgresRepository) CreateRateSurcharge(ctx context.Context, companyID, rateCardID int64, input CreateRateSurchargeInput) (*RateSurcharge, error) {
	amount := input.SurchargeAmount
	if amount == nil {
		zero := accountingmoney.Must("0", 2)
		amount = &zero
	}
	var percent pgtype.Numeric
	if input.SurchargePercent != nil {
		_ = percent.Scan(fmt.Sprintf("%.4f", *input.SurchargePercent))
	}
	res, err := r.q.CreateRateSurcharge(ctx, sqlc.CreateRateSurchargeParams{
		CompanyID:        companyID,
		RateCardID:       rateCardID,
		SurchargeType:    string(input.SurchargeType),
		SurchargeName:    input.SurchargeName,
		SurchargeAmount:  moneyToNumericVal(*amount),
		SurchargePercent: percent,
		EffectiveDate:    timeToDateVal(input.EffectiveDate),
		ExpirationDate:   timeToDate(input.ExpirationDate),
	})
	if err != nil {
		return nil, err
	}
	return mapRateSurcharge(res), nil
}
func (r *postgresRepository) ListRateSurcharges(ctx context.Context, rateCardID int64) ([]*RateSurcharge, error) {
	rows, err := r.q.ListRateSurcharges(ctx, rateCardID)
	if err != nil {
		return nil, err
	}
	result := make([]*RateSurcharge, 0, len(rows))
	for _, row := range rows {
		result = append(result, mapRateSurcharge(row))
	}
	return result, nil
}
func (r *postgresRepository) DeleteRateSurcharge(ctx context.Context, surchargeID int64) error {
	return r.q.DeleteRateSurcharge(ctx, surchargeID)
}
func (r *postgresRepository) ListFreightCharges(ctx context.Context, companyID int64, filter FreightChargeFilter) ([]*FreightCharge, error) {
	var shipmentID, loadID, carrierID int64
	if filter.ShipmentID != nil {
		shipmentID = *filter.ShipmentID
	}
	if filter.LoadID != nil {
		loadID = *filter.LoadID
	}
	if filter.CarrierID != nil {
		carrierID = *filter.CarrierID
	}
	var status, origin, destination string
	if filter.Status != nil {
		status = string(*filter.Status)
	}
	if filter.OriginCity != nil {
		origin = *filter.OriginCity
	}
	if filter.DestinationCity != nil {
		destination = *filter.DestinationCity
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.q.ListFreightCharges(ctx, sqlc.ListFreightChargesParams{
		CompanyID: companyID,
		Column2:   shipmentID, Column3: loadID, Column4: carrierID,
		Column5: status, Column6: origin, Column7: destination,
		Column8: timeToTimestamp(filter.CreatedAfter), Column9: timeToTimestamp(filter.CreatedBefore),
		Limit: int32(limit), Offset: int32(maxInt(filter.Offset, 0)),
	})
	if err != nil {
		return nil, err
	}
	result := make([]*FreightCharge, 0, len(rows))
	for _, row := range rows {
		result = append(result, mapFreightCharge(row))
	}
	return result, nil
}
func (r *postgresRepository) UpdateFreightCharge(ctx context.Context, companyID, chargeID int64, updates FreightChargeUpdate) (*FreightCharge, error) {
	current, err := r.GetFreightCharge(ctx, companyID, chargeID)
	if err != nil {
		return nil, err
	}
	status := string(current.Status)
	if updates.Status != nil {
		status = string(*updates.Status)
	}
	res, err := r.q.UpdateFreightCharge(ctx, sqlc.UpdateFreightChargeParams{
		ID:            chargeID,
		CompanyID:     companyID,
		Status:        status,
		InvoiceNumber: stringToText(updates.InvoiceNumber),
		InvoiceDate:   timeToDate(updates.InvoiceDate),
		GlPostingID:   int64ToInt8(updates.GLPostingID),
		Notes:         stringToText(updates.Notes),
	})
	if err != nil {
		return nil, err
	}
	return mapFreightCharge(res), nil
}
func (r *postgresRepository) UpdateFreightChargeStatus(ctx context.Context, companyID, chargeID int64, status FreightChargeStatus) error {
	return r.q.UpdateFreightChargeStatus(ctx, sqlc.UpdateFreightChargeStatusParams{
		ID: chargeID, CompanyID: companyID, Status: string(status),
	})
}
func (r *postgresRepository) CreateLandedCost(ctx context.Context, cost *LandedCost) (*LandedCost, error) {
	res, err := r.q.CreateLandedCost(ctx, sqlc.CreateLandedCostParams{
		CompanyID:        cost.CompanyID,
		ShipmentID:       cost.ShipmentID,
		LoadID:           int64ToInt8(cost.LoadID),
		FreightChargeID:  cost.FreightChargeID,
		PoID:             int64ToInt8(cost.POID),
		ProductCost:      moneyToNumericVal(cost.ProductCost),
		FreightCost:      moneyToNumericVal(cost.FreightCost),
		DutyCost:         moneyToNumeric(cost.DutyCost),
		TaxCost:          moneyToNumeric(cost.TaxCost),
		InsuranceCost:    moneyToNumeric(cost.InsuranceCost),
		OtherCost:        moneyToNumeric(cost.OtherCost),
		TotalLandedCost:  moneyToNumericVal(cost.TotalLandedCost),
		CostPerUnit:      moneyToNumeric(cost.CostPerUnit),
		Currency:         cost.Currency,
		AllocationMethod: string(cost.AllocationMethod),
	})
	if err != nil {
		return nil, err
	}
	return mapLandedCost(res), nil
}
func (r *postgresRepository) GetLandedCost(ctx context.Context, companyID, costID int64) (*LandedCost, error) {
	res, err := r.q.GetLandedCost(ctx, sqlc.GetLandedCostParams{ID: costID, CompanyID: companyID})
	if err != nil {
		return nil, err
	}
	return mapLandedCost(res), nil
}
func (r *postgresRepository) ListLandedCosts(ctx context.Context, companyID int64, filter LandedCostFilter) ([]*LandedCost, error) {
	var shipmentID, loadID, poID int64
	if filter.ShipmentID != nil {
		shipmentID = *filter.ShipmentID
	}
	if filter.LoadID != nil {
		loadID = *filter.LoadID
	}
	if filter.POID != nil {
		poID = *filter.POID
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.q.ListLandedCosts(ctx, sqlc.ListLandedCostsParams{
		CompanyID: companyID, Column2: shipmentID, Column3: loadID, Column4: poID,
		Column5: timeToTimestamp(filter.CreatedAfter), Column6: timeToTimestamp(filter.CreatedBefore),
		Limit: int32(limit), Offset: int32(maxInt(filter.Offset, 0)),
	})
	if err != nil {
		return nil, err
	}
	result := make([]*LandedCost, 0, len(rows))
	for _, row := range rows {
		result = append(result, mapLandedCost(row))
	}
	return result, nil
}
func (r *postgresRepository) GetLandedCostByShipment(ctx context.Context, companyID, shipmentID int64) (*LandedCost, error) {
	res, err := r.q.GetLandedCostByShipment(ctx, sqlc.GetLandedCostByShipmentParams{
		ShipmentID: shipmentID, CompanyID: companyID,
	})
	if err != nil {
		return nil, err
	}
	return mapLandedCost(res), nil
}
func (r *postgresRepository) CreateCostCenter(ctx context.Context, companyID int64, input CreateCostCenterInput) (*CostCenter, error) {
	centerType := input.CostCenterType
	if centerType == "" {
		centerType = CostCenterTypeDepartment
	}
	res, err := r.q.CreateFreightCostCenter(ctx, sqlc.CreateFreightCostCenterParams{
		CompanyID: companyID, Code: input.CostCenterCode, Name: input.CostCenterName,
		CostCenterType: string(centerType), WarehouseID: int64ToInt8(input.WarehouseID),
		GlAccount: stringToText(input.GLAccount), ManagerID: int64ToInt8(input.ManagerID),
	})
	if err != nil {
		return nil, err
	}
	return mapCostCenterValues(res.ID, res.CompanyID, res.DepartmentID, res.Code, res.Name, res.CostCenterType, res.WarehouseID, res.GlAccount, res.ManagerID, res.IsActive, res.CreatedAt, res.UpdatedAt), nil
}
func (r *postgresRepository) GetCostCenter(ctx context.Context, companyID, costCenterID int64) (*CostCenter, error) {
	res, err := r.q.GetFreightCostCenter(ctx, sqlc.GetFreightCostCenterParams{ID: costCenterID, CompanyID: companyID})
	if err != nil {
		return nil, err
	}
	return mapCostCenterValues(res.ID, res.CompanyID, res.DepartmentID, res.Code, res.Name, res.CostCenterType, res.WarehouseID, res.GlAccount, res.ManagerID, res.IsActive, res.CreatedAt, res.UpdatedAt), nil
}
func (r *postgresRepository) GetCostCenterByCode(ctx context.Context, companyID int64, code string) (*CostCenter, error) {
	res, err := r.q.GetFreightCostCenterByCode(ctx, sqlc.GetFreightCostCenterByCodeParams{CompanyID: companyID, Code: code})
	if err != nil {
		return nil, err
	}
	return mapCostCenterValues(res.ID, res.CompanyID, res.DepartmentID, res.Code, res.Name, res.CostCenterType, res.WarehouseID, res.GlAccount, res.ManagerID, res.IsActive, res.CreatedAt, res.UpdatedAt), nil
}
func (r *postgresRepository) ListCostCenters(ctx context.Context, companyID int64) ([]*CostCenter, error) {
	rows, err := r.q.ListFreightCostCenters(ctx, companyID)
	if err != nil {
		return nil, err
	}
	result := make([]*CostCenter, 0, len(rows))
	for _, res := range rows {
		result = append(result, mapCostCenterValues(res.ID, res.CompanyID, res.DepartmentID, res.Code, res.Name, res.CostCenterType, res.WarehouseID, res.GlAccount, res.ManagerID, res.IsActive, res.CreatedAt, res.UpdatedAt))
	}
	return result, nil
}
func (r *postgresRepository) UpdateCostCenter(ctx context.Context, companyID, costCenterID int64, updates CostCenterUpdate) (*CostCenter, error) {
	current, err := r.GetCostCenter(ctx, companyID, costCenterID)
	if err != nil {
		return nil, err
	}
	name := current.CostCenterName
	if updates.CostCenterName != nil {
		name = *updates.CostCenterName
	}
	centerType := string(current.CostCenterType)
	active := current.IsActive
	if updates.IsActive != nil {
		active = *updates.IsActive
	}
	res, err := r.q.UpdateFreightCostCenter(ctx, sqlc.UpdateFreightCostCenterParams{
		ID: costCenterID, CompanyID: companyID, Name: name, CostCenterType: centerType,
		WarehouseID: int64ToInt8(current.WarehouseID), GlAccount: stringToText(updates.GLAccount),
		ManagerID: int64ToInt8(updates.ManagerID), IsActive: active,
	})
	if err != nil {
		return nil, err
	}
	return mapCostCenterValues(res.ID, res.CompanyID, res.DepartmentID, res.Code, res.Name, res.CostCenterType, res.WarehouseID, res.GlAccount, res.ManagerID, res.IsActive, res.CreatedAt, res.UpdatedAt), nil
}
func (r *postgresRepository) CreateAuditLog(ctx context.Context, log *FreightAuditLog) error {
	return r.q.CreateFreightAuditLog(ctx, sqlc.CreateFreightAuditLogParams{
		CompanyID: log.CompanyID, FreightChargeID: log.FreightChargeID, AuditType: string(log.AuditType),
		OldValue: moneyToNumeric(log.OldValue), NewValue: moneyToNumeric(log.NewValue),
		Reason: stringToText(log.Reason), UserID: log.UserID,
	})
}
func (r *postgresRepository) ListAuditLogs(ctx context.Context, companyID, freightChargeID int64) ([]*FreightAuditLog, error) {
	rows, err := r.q.ListFreightAuditLogs(ctx, sqlc.ListFreightAuditLogsParams{CompanyID: companyID, FreightChargeID: freightChargeID})
	if err != nil {
		return nil, err
	}
	result := make([]*FreightAuditLog, 0, len(rows))
	for _, row := range rows {
		result = append(result, mapFreightAuditLog(row))
	}
	return result, nil
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
