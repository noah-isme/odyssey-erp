package freight

import (
	"context"
	"fmt"
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
	if !n.Valid {
		return nil
	}
	f, _ := n.Float64Value()
	m, _ := accountingmoney.Parse(fmt.Sprintf("%f", f.Float64), scale)
	return &m
}

func numericToMoneyVal(n pgtype.Numeric, scale int) accountingmoney.Money {
	if !n.Valid {
		m, _ := accountingmoney.Parse("0", scale)
		return m
	}
	f, _ := n.Float64Value()
	m, _ := accountingmoney.Parse(fmt.Sprintf("%f", f.Float64), scale)
	return m
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
		ID:              s.ID,
		CompanyID:       s.CompanyID,
		CarrierID:       carrierID,
		OriginCity:      s.OriginCity,
		OriginCountry:   s.OriginCountry,
		DestinationCity: s.DestinationCity,
		DestinationCountry: s.DestinationCountry,
		ServiceLevel:    ServiceLevel(s.ServiceLevel),
		MinWeightKg:     numericToMoney(s.MinWeight, 4),
		MaxWeightKg:     numericToMoney(s.MaxWeight, 4),
		BaseRate:        numericToMoneyVal(s.BaseRate, 2),
		PerKgRate:       numericToMoney(s.PerKgRate, 2),
		PerCbmRate:      numericToMoney(s.PerCbmRate, 2),
		Currency:        s.Currency,
		EffectiveDate:   s.EffectiveDate.Time,
		ExpirationDate:  expDate,
		IsActive:        s.IsActive,
		CreatedBy:       s.CreatedBy,
		CreatedAt:       s.CreatedAt.Time,
		UpdatedAt:       s.UpdatedAt.Time,
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

// Unimplemented methods
func (r *postgresRepository) UpdateRateCard(ctx context.Context, companyID, rateCardID int64, updates RateCardUpdate) (*RateCard, error) {
	return nil, fmt.Errorf("not implemented")
}
func (r *postgresRepository) DeactivateRateCard(ctx context.Context, companyID, rateCardID int64) error {
	return fmt.Errorf("not implemented")
}
func (r *postgresRepository) GetApplicableRateCard(ctx context.Context, companyID int64, lookup RateLookup) (*RateCard, error) {
	return nil, fmt.Errorf("not implemented")
}
func (r *postgresRepository) ListApplicableRateCards(ctx context.Context, companyID int64, lookup RateLookup) ([]*RateCard, error) {
	return nil, fmt.Errorf("not implemented")
}
func (r *postgresRepository) CreateRateSurcharge(ctx context.Context, companyID, rateCardID int64, input CreateRateSurchargeInput) (*RateSurcharge, error) {
	return nil, fmt.Errorf("not implemented")
}
func (r *postgresRepository) ListRateSurcharges(ctx context.Context, rateCardID int64) ([]*RateSurcharge, error) {
	return nil, fmt.Errorf("not implemented")
}
func (r *postgresRepository) DeleteRateSurcharge(ctx context.Context, surchargeID int64) error {
	return fmt.Errorf("not implemented")
}
func (r *postgresRepository) ListFreightCharges(ctx context.Context, companyID int64, filter FreightChargeFilter) ([]*FreightCharge, error) {
	return nil, fmt.Errorf("not implemented")
}
func (r *postgresRepository) UpdateFreightCharge(ctx context.Context, companyID, chargeID int64, updates FreightChargeUpdate) (*FreightCharge, error) {
	return nil, fmt.Errorf("not implemented")
}
func (r *postgresRepository) UpdateFreightChargeStatus(ctx context.Context, companyID, chargeID int64, status FreightChargeStatus) error {
	return fmt.Errorf("not implemented")
}
func (r *postgresRepository) CreateLandedCost(ctx context.Context, cost *LandedCost) (*LandedCost, error) {
	return nil, fmt.Errorf("not implemented")
}
func (r *postgresRepository) GetLandedCost(ctx context.Context, companyID, costID int64) (*LandedCost, error) {
	return nil, fmt.Errorf("not implemented")
}
func (r *postgresRepository) ListLandedCosts(ctx context.Context, companyID int64, filter LandedCostFilter) ([]*LandedCost, error) {
	return nil, fmt.Errorf("not implemented")
}
func (r *postgresRepository) GetLandedCostByShipment(ctx context.Context, companyID, shipmentID int64) (*LandedCost, error) {
	return nil, fmt.Errorf("not implemented")
}
func (r *postgresRepository) CreateCostCenter(ctx context.Context, companyID int64, input CreateCostCenterInput) (*CostCenter, error) {
	return nil, fmt.Errorf("not implemented")
}
func (r *postgresRepository) GetCostCenter(ctx context.Context, companyID, costCenterID int64) (*CostCenter, error) {
	return nil, fmt.Errorf("not implemented")
}
func (r *postgresRepository) GetCostCenterByCode(ctx context.Context, companyID int64, code string) (*CostCenter, error) {
	return nil, fmt.Errorf("not implemented")
}
func (r *postgresRepository) ListCostCenters(ctx context.Context, companyID int64) ([]*CostCenter, error) {
	return nil, fmt.Errorf("not implemented")
}
func (r *postgresRepository) UpdateCostCenter(ctx context.Context, companyID, costCenterID int64, updates CostCenterUpdate) (*CostCenter, error) {
	return nil, fmt.Errorf("not implemented")
}
func (r *postgresRepository) CreateAuditLog(ctx context.Context, log *FreightAuditLog) error {
	return fmt.Errorf("not implemented")
}
func (r *postgresRepository) ListAuditLogs(ctx context.Context, companyID, freightChargeID int64) ([]*FreightAuditLog, error) {
	return nil, fmt.Errorf("not implemented")
}
