package freight

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	accountingmoney "github.com/odyssey-erp/odyssey-erp/internal/accounting/money"
)

// ═══════════════════════════════════════════════════════════════════════════
// HTTP HANDLER
// ═══════════════════════════════════════════════════════════════════════════

type Handler struct {
	svc Service
}

func NewHandler(svc Service) *Handler {
	return &Handler{svc: svc}
}

// ═══════════════════════════════════════════════════════════════════════════
// REQUEST/RESPONSE TYPES
// ═══════════════════════════════════════════════════════════════════════════

type CreateRateCardRequest struct {
	CarrierID          *int64  `json:"carrier_id"`
	OriginCity         string  `json:"origin_city"`
	OriginCountry      string  `json:"origin_country"`
	DestinationCity    string  `json:"destination_city"`
	DestinationCountry string  `json:"destination_country"`
	ServiceLevel       string  `json:"service_level"`
	MinWeightKg        *string `json:"min_weight_kg"`
	MaxWeightKg        *string `json:"max_weight_kg"`
	BaseRate           string  `json:"base_rate"`
	PerKgRate          *string `json:"per_kg_rate"`
	PerCbmRate         *string `json:"per_cbm_rate"`
	Currency           string  `json:"currency"`
	EffectiveDate      string  `json:"effective_date"`
	ExpirationDate     *string `json:"expiration_date"`
}

type RateCardResponse struct {
	ID                 int64   `json:"id"`
	CarrierID          *int64  `json:"carrier_id"`
	OriginCity         string  `json:"origin_city"`
	OriginCountry      string  `json:"origin_country"`
	DestinationCity    string  `json:"destination_city"`
	DestinationCountry string  `json:"destination_country"`
	ServiceLevel       string  `json:"service_level"`
	MinWeightKg        *string `json:"min_weight_kg"`
	MaxWeightKg        *string `json:"max_weight_kg"`
	BaseRate           string  `json:"base_rate"`
	PerKgRate          *string `json:"per_kg_rate"`
	PerCbmRate         *string `json:"per_cbm_rate"`
	Currency           string  `json:"currency"`
	IsActive           bool    `json:"is_active"`
	CreatedAt          string  `json:"created_at"`
	UpdatedAt          string  `json:"updated_at"`
}

type CalculateFreightRequest struct {
	CarrierID       *int64  `json:"carrier_id"`
	OriginCity      string  `json:"origin_city"`
	DestinationCity string  `json:"destination_city"`
	ServiceLevel    string  `json:"service_level"`
	WeightKg        *string `json:"weight_kg"`
	VolumeCbm       *string `json:"volume_cbm"`
	ShipmentID      *int64  `json:"shipment_id"`
	LoadID          *int64  `json:"load_id"`
	CostCenterID    *int64  `json:"cost_center_id"`
}

type FreightChargeResponse struct {
	ID              int64   `json:"id"`
	ShipmentID      *int64  `json:"shipment_id"`
	LoadID          *int64  `json:"load_id"`
	OriginCity      string  `json:"origin_city"`
	DestinationCity string  `json:"destination_city"`
	ServiceLevel    *string `json:"service_level"`
	WeightKg        *string `json:"weight_kg"`
	VolumeCbm       *string `json:"volume_cbm"`
	BaseCharge      string  `json:"base_charge"`
	WeightCharge    *string `json:"weight_charge"`
	VolumeCharge    *string `json:"volume_charge"`
	SurchargeTotal  *string `json:"surcharge_total"`
	FreightTotal    string  `json:"freight_total"`
	Currency        string  `json:"currency"`
	Status          string  `json:"status"`
	InvoiceNumber   *string `json:"invoice_number"`
	InvoiceDate     *string `json:"invoice_date"`
	CreatedAt       string  `json:"created_at"`
}

type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

// ═══════════════════════════════════════════════════════════════════════════
// RATE CARD ENDPOINTS
// ═══════════════════════════════════════════════════════════════════════════

// POST /api/freight/rate-cards
func (h *Handler) CreateRateCard(w http.ResponseWriter, r *http.Request) {
	var req CreateRateCardRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	companyID := r.Context().Value("company_id").(int64)
	userID := r.Context().Value("user_id").(int64)

	// Parse Money values
	baseRate, err := accountingmoney.Parse(req.BaseRate, 2)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid base_rate", err.Error())
		return
	}

	var minWeightKg, maxWeightKg, perKgRate, perCbmRate *accountingmoney.Money
	if req.MinWeightKg != nil {
		m, _ := accountingmoney.Parse(*req.MinWeightKg, 4)
		minWeightKg = &m
	}
	if req.MaxWeightKg != nil {
		m, _ := accountingmoney.Parse(*req.MaxWeightKg, 4)
		maxWeightKg = &m
	}
	if req.PerKgRate != nil {
		m, _ := accountingmoney.Parse(*req.PerKgRate, 2)
		perKgRate = &m
	}
	if req.PerCbmRate != nil {
		m, _ := accountingmoney.Parse(*req.PerCbmRate, 2)
		perCbmRate = &m
	}

	effDate, _ := time.Parse("2006-01-02", req.EffectiveDate)
	var expDate *time.Time
	if req.ExpirationDate != nil {
		t, _ := time.Parse("2006-01-02", *req.ExpirationDate)
		expDate = &t
	}

	input := CreateRateCardInput{
		CompanyID:          companyID,
		CarrierID:          req.CarrierID,
		OriginCity:         req.OriginCity,
		OriginCountry:      req.OriginCountry,
		DestinationCity:    req.DestinationCity,
		DestinationCountry: req.DestinationCountry,
		ServiceLevel:       ServiceLevel(req.ServiceLevel),
		MinWeightKg:        minWeightKg,
		MaxWeightKg:        maxWeightKg,
		BaseRate:           baseRate,
		PerKgRate:          perKgRate,
		PerCbmRate:         perCbmRate,
		Currency:           req.Currency,
		EffectiveDate:      effDate,
		ExpirationDate:     expDate,
		CreatedBy:          userID,
	}

	rateCard, err := h.svc.CreateRateCard(r.Context(), input)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create rate card", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, toRateCardResponse(rateCard))
}

// GET /api/freight/rate-cards/:id
func (h *Handler) GetRateCard(w http.ResponseWriter, r *http.Request) {
	companyID := r.Context().Value("company_id").(int64)
	rateCardID, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)

	rateCard, err := h.svc.GetRateCard(r.Context(), companyID, rateCardID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Rate card not found", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, toRateCardResponse(rateCard))
}

// GET /api/freight/rate-cards
func (h *Handler) ListRateCards(w http.ResponseWriter, r *http.Request) {
	companyID := r.Context().Value("company_id").(int64)

	filter := RateCardFilter{
		Limit:  50,
		Offset: 0,
	}

	if limit := r.URL.Query().Get("limit"); limit != "" {
		if l, _ := strconv.Atoi(limit); l > 0 && l <= 100 {
			filter.Limit = l
		}
	}

	rateCards, err := h.svc.ListRateCards(r.Context(), companyID, filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list rate cards", err.Error())
		return
	}

	responses := make([]RateCardResponse, len(rateCards))
	for i, rc := range rateCards {
		responses[i] = toRateCardResponse(rc)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data":  responses,
		"count": len(responses),
	})
}

// ═══════════════════════════════════════════════════════════════════════════
// FREIGHT CHARGE ENDPOINTS
// ═══════════════════════════════════════════════════════════════════════════

// POST /api/freight/charges/calculate
func (h *Handler) CalculateFreightCharge(w http.ResponseWriter, r *http.Request) {
	var req CalculateFreightRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	companyID := r.Context().Value("company_id").(int64)

	var weightKg, volumeCbm *accountingmoney.Money
	if req.WeightKg != nil {
		m, _ := accountingmoney.Parse(*req.WeightKg, 4)
		weightKg = &m
	}
	if req.VolumeCbm != nil {
		m, _ := accountingmoney.Parse(*req.VolumeCbm, 4)
		volumeCbm = &m
	}

	input := CalculateFreightInput{
		CompanyID:       companyID,
		CarrierID:       req.CarrierID,
		OriginCity:      req.OriginCity,
		DestinationCity: req.DestinationCity,
		ServiceLevel:    ServiceLevel(req.ServiceLevel),
		WeightKg:        weightKg,
		VolumeCbm:       volumeCbm,
		ShipmentID:      req.ShipmentID,
		LoadID:          req.LoadID,
		CostCenterID:    req.CostCenterID,
	}

	charge, err := h.svc.CalculateAndCreateFreightCharge(r.Context(), input)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to calculate freight", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, toFreightChargeResponse(charge))
}

// GET /api/freight/charges/:id
func (h *Handler) GetFreightCharge(w http.ResponseWriter, r *http.Request) {
	companyID := r.Context().Value("company_id").(int64)
	chargeID, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)

	charge, err := h.svc.GetFreightCharge(r.Context(), companyID, chargeID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Freight charge not found", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, toFreightChargeResponse(charge))
}

// GET /api/freight/charges
func (h *Handler) ListFreightCharges(w http.ResponseWriter, r *http.Request) {
	companyID := r.Context().Value("company_id").(int64)

	filter := FreightChargeFilter{
		Limit:  50,
		Offset: 0,
	}

	if status := r.URL.Query().Get("status"); status != "" {
		s := FreightChargeStatus(status)
		filter.Status = &s
	}

	charges, err := h.svc.ListFreightCharges(r.Context(), companyID, filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list freight charges", err.Error())
		return
	}

	responses := make([]FreightChargeResponse, len(charges))
	for i, c := range charges {
		responses[i] = toFreightChargeResponse(c)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data":  responses,
		"count": len(responses),
	})
}

// POST /api/freight/charges/:id/invoice
func (h *Handler) MarkFreightChargeInvoiced(w http.ResponseWriter, r *http.Request) {
	companyID := r.Context().Value("company_id").(int64)
	chargeID, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)

	var req struct {
		InvoiceNumber string `json:"invoice_number"`
		InvoiceDate   string `json:"invoice_date"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	invoiceDate, err := time.Parse("2006-01-02", req.InvoiceDate)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid invoice_date format", "Use YYYY-MM-DD")
		return
	}

	charge, err := h.svc.UpdateFreightChargeInvoice(r.Context(), companyID, chargeID, req.InvoiceNumber, invoiceDate)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to update freight charge", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, toFreightChargeResponse(charge))
}

// POST /api/freight/charges/:id/paid
func (h *Handler) MarkFreightChargePaid(w http.ResponseWriter, r *http.Request) {
	companyID := r.Context().Value("company_id").(int64)
	chargeID, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)

	charge, err := h.svc.MarkFreightChargePaid(r.Context(), companyID, chargeID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to mark charge as paid", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, toFreightChargeResponse(charge))
}

// ═══════════════════════════════════════════════════════════════════════════
// COST CENTER ENDPOINTS
// ═══════════════════════════════════════════════════════════════════════════

// POST /api/freight/cost-centers
func (h *Handler) CreateCostCenter(w http.ResponseWriter, r *http.Request) {
	companyID := r.Context().Value("company_id").(int64)

	var req CreateCostCenterInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	costCenter, err := h.svc.CreateCostCenter(r.Context(), companyID, req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create cost center", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, costCenter)
}

// GET /api/freight/cost-centers
func (h *Handler) ListCostCenters(w http.ResponseWriter, r *http.Request) {
	companyID := r.Context().Value("company_id").(int64)

	costCenters, err := h.svc.ListCostCenters(r.Context(), companyID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list cost centers", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data":  costCenters,
		"count": len(costCenters),
	})
}

// ═══════════════════════════════════════════════════════════════════════════
// HELPER FUNCTIONS
// ═══════════════════════════════════════════════════════════════════════════

func toRateCardResponse(rc *RateCard) RateCardResponse {
	return RateCardResponse{
		ID:                 rc.ID,
		CarrierID:          rc.CarrierID,
		OriginCity:         rc.OriginCity,
		OriginCountry:      rc.OriginCountry,
		DestinationCity:    rc.DestinationCity,
		DestinationCountry: rc.DestinationCountry,
		ServiceLevel:       string(rc.ServiceLevel),
		MinWeightKg:        moneyPtr(rc.MinWeightKg),
		MaxWeightKg:        moneyPtr(rc.MaxWeightKg),
		BaseRate:           rc.BaseRate.String(),
		PerKgRate:          moneyPtr(rc.PerKgRate),
		PerCbmRate:         moneyPtr(rc.PerCbmRate),
		Currency:           rc.Currency,
		IsActive:           rc.IsActive,
		CreatedAt:          rc.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:          rc.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
}

func toFreightChargeResponse(fc *FreightCharge) FreightChargeResponse {
	return FreightChargeResponse{
		ID:              fc.ID,
		ShipmentID:      fc.ShipmentID,
		LoadID:          fc.LoadID,
		OriginCity:      fc.OriginCity,
		DestinationCity: fc.DestinationCity,
		ServiceLevel:    stringPtr(string(*fc.ServiceLevel)),
		WeightKg:        moneyPtr(fc.WeightKg),
		VolumeCbm:       moneyPtr(fc.VolumeCbm),
		BaseCharge:      fc.BaseCharge.String(),
		WeightCharge:    moneyPtr(fc.WeightCharge),
		VolumeCharge:    moneyPtr(fc.VolumeCharge),
		SurchargeTotal:  moneyPtr(fc.SurchargeTotal),
		FreightTotal:    fc.FreightTotal.String(),
		Currency:        fc.Currency,
		Status:          string(fc.Status),
		InvoiceNumber:   fc.InvoiceNumber,
		InvoiceDate:     timePtr(fc.InvoiceDate),
		CreatedAt:       fc.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}
}

func moneyPtr(m *accountingmoney.Money) *string {
	if m == nil {
		return nil
	}
	s := m.String()
	return &s
}

func timePtr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.Format("2006-01-02")
	return &s
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, errType, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(ErrorResponse{
		Error:   errType,
		Message: message,
	})
}
