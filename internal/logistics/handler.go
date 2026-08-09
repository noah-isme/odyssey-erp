package logistics

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
)

// Handler handles HTTP requests for logistics operations
type Handler struct {
	service *Service
}

// NewHandler creates a new logistics handler
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// ═══════════════════════════════════════════════════════════════════════════
// CARRIER ENDPOINTS
// ═══════════════════════════════════════════════════════════════════════════

// CreateCarrierRequest defines the request body for creating a carrier
type CreateCarrierRequest struct {
	CarrierName           string  `json:"carrier_name"`
	CarrierCode           string  `json:"carrier_code"`
	ContactName           string  `json:"contact_name"`
	ContactEmail          string  `json:"contact_email"`
	ContactPhone          string  `json:"contact_phone"`
	InsuranceProvider     string  `json:"insurance_provider"`
	InsurancePolicyNumber string  `json:"insurance_policy_number"`
	InsuranceExpiresAt    *string `json:"insurance_expires_at"` // ISO8601
}

// CreateCarrierHandler creates a new carrier
func (h *Handler) CreateCarrierHandler(w http.ResponseWriter, r *http.Request) {
	companyID := int64(1) // Default CompanyID
	userID := int64(1)

	var req CreateCarrierRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		JSONError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	input := CreateCarrierInput{
		CompanyID:             companyID,
		CarrierName:           req.CarrierName,
		CarrierCode:           req.CarrierCode,
		ContactName:           req.ContactName,
		ContactEmail:          req.ContactEmail,
		ContactPhone:          req.ContactPhone,
		InsuranceProvider:     req.InsuranceProvider,
		InsurancePolicyNumber: req.InsurancePolicyNumber,
		CreatedBy:             userID,
	}

	if req.InsuranceExpiresAt != nil {
		t, err := time.Parse(time.RFC3339, *req.InsuranceExpiresAt)
		if err != nil {
			JSONError(w, "invalid insurance_expires_at format (use ISO8601)", http.StatusBadRequest)
			return
		}
		input.InsuranceExpiresAt = &t
	}

	carrier, err := h.service.RegisterCarrier(r.Context(), input)
	if err != nil {
		JSONErrorFrom(w, err, http.StatusInternalServerError)
		return
	}

	JSONSuccess(w, carrier, "carrier created successfully")
}

// ListCarriersHandler lists all carriers for a company
func (h *Handler) ListCarriersHandler(w http.ResponseWriter, r *http.Request) {
	companyID := int64(1)

	var statusFilter *CarrierStatus
	if s := r.URL.Query().Get("status"); s != "" {
		st := CarrierStatus(s)
		statusFilter = &st
	}

	carriers, err := h.service.repo.ListCarriers(r.Context(), companyID, statusFilter)
	if err != nil {
		JSONErrorFrom(w, err, http.StatusInternalServerError)
		return
	}

	JSONSuccess(w, map[string]interface{}{"carriers": carriers}, "carriers retrieved successfully")
}

// GetCarrierHandler retrieves a single carrier
func (h *Handler) GetCarrierHandler(w http.ResponseWriter, r *http.Request) {
	carrierIDStr := chi.URLParam(r, "id")
	carrierID, err := strconv.ParseInt(carrierIDStr, 10, 64)
	if err != nil {
		JSONError(w, "invalid carrier id", http.StatusBadRequest)
		return
	}

	carrier, err := h.service.repo.GetCarrier(r.Context(), carrierID)
	if err != nil {
		JSONError(w, "carrier not found", http.StatusNotFound)
		return
	}

	// Verify company matches
	companyID := int64(1)
	if carrier.CompanyID != companyID {
		JSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	JSONSuccess(w, carrier, "carrier retrieved successfully")
}

// ═══════════════════════════════════════════════════════════════════════════
// FLEET & VEHICLE ENDPOINTS
// ═══════════════════════════════════════════════════════════════════════════

// CreateFleetRequest defines the request body for creating a fleet
type CreateFleetRequest struct {
	FleetName   string `json:"fleet_name"`
	FleetCode   string `json:"fleet_code"`
	FleetType   string `json:"fleet_type"` // OWN, CONTRACTED, MIXED
	WarehouseID *int64 `json:"warehouse_id"`
	HomeCity    string `json:"home_city"`
}

// CreateFleetHandler creates a new fleet
func (h *Handler) CreateFleetHandler(w http.ResponseWriter, r *http.Request) {
	companyID := int64(1)
	userID := int64(1)

	var req CreateFleetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		JSONError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	input := CreateFleetInput{
		CompanyID:   companyID,
		FleetName:   req.FleetName,
		FleetCode:   req.FleetCode,
		FleetType:   FleetType(req.FleetType),
		WarehouseID: req.WarehouseID,
		HomeCity:    req.HomeCity,
		CreatedBy:   userID,
	}

	fleet, err := h.service.CreateFleet(r.Context(), input)
	if err != nil {
		JSONErrorFrom(w, err, http.StatusInternalServerError)
		return
	}

	JSONSuccess(w, fleet, "fleet created successfully")
}

// ListFleetsHandler lists all fleets for a company
func (h *Handler) ListFleetsHandler(w http.ResponseWriter, r *http.Request) {
	companyID := int64(1)

	fleets, err := h.service.repo.ListFleets(r.Context(), companyID)
	if err != nil {
		JSONErrorFrom(w, err, http.StatusInternalServerError)
		return
	}

	JSONSuccess(w, map[string]interface{}{"fleets": fleets}, "fleets retrieved successfully")
}

// RegisterVehicleRequest defines the request body for registering a vehicle
type RegisterVehicleRequest struct {
	FleetID             int64    `json:"fleet_id"`
	VehicleRegistration string   `json:"vehicle_registration"`
	VehicleType         string   `json:"vehicle_type"` // VAN, TRUCK, LORRY, BIKE, CAR
	LicensePlate        string   `json:"license_plate"`
	VIN                 string   `json:"vin"`
	Make                string   `json:"make"`
	Model               string   `json:"model"`
	YearManufactured    *int     `json:"year_manufactured"`
	MaxWeightKg         *float64 `json:"max_weight_kg"`
	MaxVolumeCbm        *float64 `json:"max_volume_cbm"` // NUMERIC
	GPSDeviceID         string   `json:"gps_device_id"`
}

// RegisterVehicleHandler registers a new vehicle
func (h *Handler) RegisterVehicleHandler(w http.ResponseWriter, r *http.Request) {
	companyID := int64(1)
	userID := int64(1)

	var req RegisterVehicleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		JSONError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	input := CreateVehicleInput{
		CompanyID:           companyID,
		FleetID:             req.FleetID,
		VehicleRegistration: req.VehicleRegistration,
		VehicleType:         VehicleType(req.VehicleType),
		LicensePlate:        req.LicensePlate,
		VIN:                 req.VIN,
		Make:                req.Make,
		Model:               req.Model,
		YearManufactured:    req.YearManufactured,
		MaxWeightKg:         req.MaxWeightKg,
		MaxVolumeCbm:        req.MaxVolumeCbm,
		GPSDeviceID:         req.GPSDeviceID,
		CreatedBy:           userID,
	}

	vehicle, err := h.service.RegisterVehicle(r.Context(), input)
	if err != nil {
		JSONErrorFrom(w, err, http.StatusInternalServerError)
		return
	}

	JSONSuccess(w, vehicle, "vehicle registered successfully")
}

// ListVehiclesHandler lists all vehicles for a fleet
func (h *Handler) ListVehiclesHandler(w http.ResponseWriter, r *http.Request) {
	fleetIDStr := chi.URLParam(r, "fleet_id")
	fleetID, err := strconv.ParseInt(fleetIDStr, 10, 64)
	if err != nil {
		JSONError(w, "invalid fleet id", http.StatusBadRequest)
		return
	}

	vehicles, err := h.service.repo.ListVehiclesByFleet(r.Context(), fleetID)
	if err != nil {
		JSONErrorFrom(w, err, http.StatusInternalServerError)
		return
	}

	JSONSuccess(w, map[string]interface{}{"vehicles": vehicles}, "vehicles retrieved successfully")
}

// ═══════════════════════════════════════════════════════════════════════════
// DRIVER ENDPOINTS
// ═══════════════════════════════════════════════════════════════════════════

// RegisterDriverRequest defines the request body for registering a driver
type RegisterDriverRequest struct {
	DriverName            string  `json:"driver_name"`
	DriverCode            string  `json:"driver_code"`
	Email                 string  `json:"email"`
	Phone                 string  `json:"phone"`
	LicenseNumber         string  `json:"license_number"`
	LicenseClass          string  `json:"license_class"`      // A-E
	LicenseExpiresAt      *string `json:"license_expires_at"` // ISO8601
	EmergencyContactName  string  `json:"emergency_contact_name"`
	EmergencyContactPhone string  `json:"emergency_contact_phone"`
}

// RegisterDriverHandler registers a new driver
func (h *Handler) RegisterDriverHandler(w http.ResponseWriter, r *http.Request) {
	companyID := int64(1)
	userID := int64(1)

	var req RegisterDriverRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		JSONError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	input := CreateDriverInput{
		CompanyID:             companyID,
		DriverName:            req.DriverName,
		DriverCode:            req.DriverCode,
		Email:                 req.Email,
		Phone:                 req.Phone,
		LicenseNumber:         req.LicenseNumber,
		LicenseClass:          LicenseClass(req.LicenseClass),
		EmergencyContactName:  req.EmergencyContactName,
		EmergencyContactPhone: req.EmergencyContactPhone,
		CreatedBy:             userID,
	}

	if req.LicenseExpiresAt != nil {
		t, err := time.Parse(time.RFC3339, *req.LicenseExpiresAt)
		if err != nil {
			JSONError(w, "invalid license_expires_at format (use ISO8601)", http.StatusBadRequest)
			return
		}
		input.LicenseExpiresAt = &t
	}

	driver, err := h.service.RegisterDriver(r.Context(), input)
	if err != nil {
		JSONErrorFrom(w, err, http.StatusInternalServerError)
		return
	}

	JSONSuccess(w, driver, "driver registered successfully")
}

// ListDriversHandler lists all drivers for a company
func (h *Handler) ListDriversHandler(w http.ResponseWriter, r *http.Request) {
	companyID := int64(1)

	var statusFilter *DriverStatus
	if s := r.URL.Query().Get("status"); s != "" {
		st := DriverStatus(s)
		statusFilter = &st
	}

	drivers, err := h.service.repo.ListDrivers(r.Context(), companyID, statusFilter)
	if err != nil {
		JSONErrorFrom(w, err, http.StatusInternalServerError)
		return
	}

	JSONSuccess(w, map[string]interface{}{"drivers": drivers}, "drivers retrieved successfully")
}

// ═══════════════════════════════════════════════════════════════════════════
// SHIPMENT ENDPOINTS
// ═══════════════════════════════════════════════════════════════════════════

// CreateShipmentRequest defines the request body for creating a shipment
type CreateShipmentRequest struct {
	ShipmentType            string  `json:"shipment_type"` // DELIVERY, RETURN, TRANSFER
	OriginWarehouseID       *int64  `json:"origin_warehouse_id"`
	DestinationWarehouseID  *int64  `json:"destination_warehouse_id"`
	DestinationAddress      string  `json:"destination_address"`
	DestinationCity         string  `json:"destination_city"`
	DestinationCountry      string  `json:"destination_country"`
	DestinationContactName  string  `json:"destination_contact_name"`
	DestinationContactPhone string  `json:"destination_contact_phone"`
	PlannedDispatchAt       *string `json:"planned_dispatch_at"` // ISO8601
	PlannedDeliveryAt       *string `json:"planned_delivery_at"` // ISO8601
}

// CreateShipmentHandler creates a new shipment
func (h *Handler) CreateShipmentHandler(w http.ResponseWriter, r *http.Request) {
	companyID := int64(1)
	userID := int64(1)

	var req CreateShipmentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		JSONError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	shipmentNumber := fmt.Sprintf("SHP-%d-%d", companyID, time.Now().UnixNano()/1000)

	input := CreateShipmentInput{
		CompanyID:               companyID,
		ShipmentNumber:          shipmentNumber,
		ShipmentType:            ShipmentType(req.ShipmentType),
		OriginWarehouseID:       req.OriginWarehouseID,
		DestinationWarehouseID:  req.DestinationWarehouseID,
		DestinationAddress:      req.DestinationAddress,
		DestinationCity:         req.DestinationCity,
		DestinationCountry:      req.DestinationCountry,
		DestinationContactName:  req.DestinationContactName,
		DestinationContactPhone: req.DestinationContactPhone,
		CreatedBy:               userID,
	}

	if req.PlannedDispatchAt != nil {
		t, err := time.Parse(time.RFC3339, *req.PlannedDispatchAt)
		if err != nil {
			JSONError(w, "invalid planned_dispatch_at format (use ISO8601)", http.StatusBadRequest)
			return
		}
		input.PlannedDispatchAt = &t
	}
	if req.PlannedDeliveryAt != nil {
		t, err := time.Parse(time.RFC3339, *req.PlannedDeliveryAt)
		if err != nil {
			JSONError(w, "invalid planned_delivery_at format (use ISO8601)", http.StatusBadRequest)
			return
		}
		input.PlannedDeliveryAt = &t
	}

	shipment, err := h.service.CreateShipment(r.Context(), input)
	if err != nil {
		JSONErrorFrom(w, err, http.StatusInternalServerError)
		return
	}

	JSONSuccess(w, shipment, "shipment created successfully")
}

// ListShipmentsHandler lists all shipments for a company
func (h *Handler) ListShipmentsHandler(w http.ResponseWriter, r *http.Request) {
	companyID := int64(1)

	var statusFilter *ShipmentStatus
	if s := r.URL.Query().Get("status"); s != "" {
		st := ShipmentStatus(s)
		statusFilter = &st
	}

	shipments, err := h.service.repo.ListShipments(r.Context(), companyID, statusFilter)
	if err != nil {
		JSONErrorFrom(w, err, http.StatusInternalServerError)
		return
	}

	JSONSuccess(w, map[string]interface{}{"shipments": shipments}, "shipments retrieved successfully")
}

// GetShipmentHandler retrieves a single shipment
func (h *Handler) GetShipmentHandler(w http.ResponseWriter, r *http.Request) {
	shipmentIDStr := chi.URLParam(r, "id")
	shipmentID, err := strconv.ParseInt(shipmentIDStr, 10, 64)
	if err != nil {
		JSONError(w, "invalid shipment id", http.StatusBadRequest)
		return
	}

	shipment, err := h.service.repo.GetShipment(r.Context(), shipmentID)
	if err != nil {
		JSONErrorFrom(w, err, http.StatusNotFound)
		return
	}

	companyID := int64(1)
	if shipment.CompanyID != companyID {
		JSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	JSONSuccess(w, shipment, "shipment retrieved successfully")
}

// DispatchShipmentRequest defines the request body for dispatching a shipment
type DispatchShipmentRequest struct {
	VehicleID          *int64  `json:"vehicle_id"`
	DriverID           *int64  `json:"driver_id"`
	CarrierID          *int64  `json:"carrier_id"`
	CarrierServiceType *string `json:"carrier_service_type"` // STANDARD, EXPRESS, OVERNIGHT, ECONOMY
}

// DispatchShipmentHandler dispatches a shipment to vehicle+driver or carrier+service
func (h *Handler) DispatchShipmentHandler(w http.ResponseWriter, r *http.Request) {
	shipmentIDStr := chi.URLParam(r, "id")
	shipmentID, err := strconv.ParseInt(shipmentIDStr, 10, 64)
	if err != nil {
		JSONError(w, "invalid shipment id", http.StatusBadRequest)
		return
	}

	var req DispatchShipmentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		JSONError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	var cst *CarrierServiceType
	if req.CarrierServiceType != nil {
		t := CarrierServiceType(*req.CarrierServiceType)
		cst = &t
	}

	err = h.service.DispatchShipment(r.Context(), shipmentID, req.VehicleID, req.DriverID, req.CarrierID, cst)
	if err != nil {
		JSONErrorFrom(w, err, http.StatusInternalServerError)
		return
	}

	JSONSuccess(w, map[string]interface{}{"shipment_id": shipmentID, "status": ShipmentStatusDispatched}, "shipment dispatched successfully")
}

// TrackShipmentHandler returns real-time shipment tracking information
func (h *Handler) TrackShipmentHandler(w http.ResponseWriter, r *http.Request) {
	shipmentIDStr := chi.URLParam(r, "id")
	shipmentID, err := strconv.ParseInt(shipmentIDStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid shipment id", http.StatusBadRequest)
		return
	}

	// TODO: Call h.service.GetShipmentTracking(ctx, shipmentID)
	// TODO: Return JSON with current location, status, ETA

	_ = shipmentID
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "not implemented"})
}

// ═══════════════════════════════════════════════════════════════════════════
// TRIP ENDPOINTS
// ═══════════════════════════════════════════════════════════════════════════

// PlanTripRequest defines the request body for planning a trip
type PlanTripRequest struct {
	VehicleID         int64   `json:"vehicle_id"`
	DriverID          int64   `json:"driver_id"`
	FleetID           *int64  `json:"fleet_id"`
	OriginWarehouseID *int64  `json:"origin_warehouse_id"`
	PlannedStartAt    *string `json:"planned_start_at"` // ISO8601
	PlannedEndAt      *string `json:"planned_end_at"`   // ISO8601
}

// PlanTripHandler creates a new trip
func (h *Handler) PlanTripHandler(w http.ResponseWriter, r *http.Request) {
	companyID := int64(1)
	userID := int64(1)

	var req PlanTripRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		JSONError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	tripNumber := fmt.Sprintf("TRP-%d-%d", companyID, time.Now().UnixNano()/1000)

	input := CreateTripInput{
		CompanyID:         companyID,
		TripNumber:        tripNumber,
		VehicleID:         req.VehicleID,
		DriverID:          req.DriverID,
		FleetID:           req.FleetID,
		OriginWarehouseID: req.OriginWarehouseID,
		CreatedBy:         userID,
	}

	if req.PlannedStartAt != nil {
		t, err := time.Parse(time.RFC3339, *req.PlannedStartAt)
		if err != nil {
			JSONError(w, "invalid planned_start_at format (use ISO8601)", http.StatusBadRequest)
			return
		}
		input.PlannedStartAt = &t
	}
	if req.PlannedEndAt != nil {
		t, err := time.Parse(time.RFC3339, *req.PlannedEndAt)
		if err != nil {
			JSONError(w, "invalid planned_end_at format (use ISO8601)", http.StatusBadRequest)
			return
		}
		input.PlannedEndAt = &t
	}

	trip, err := h.service.PlanTrip(r.Context(), input)
	if err != nil {
		JSONErrorFrom(w, err, http.StatusInternalServerError)
		return
	}

	JSONSuccess(w, trip, "trip planned successfully")
}

// ListTripsHandler lists all active trips for a company
func (h *Handler) ListTripsHandler(w http.ResponseWriter, r *http.Request) {
	companyID := int64(1)

	var statusFilter *TripStatus
	if s := r.URL.Query().Get("status"); s != "" {
		st := TripStatus(s)
		statusFilter = &st
	}

	trips, err := h.service.repo.ListTrips(r.Context(), companyID, statusFilter)
	if err != nil {
		JSONErrorFrom(w, err, http.StatusInternalServerError)
		return
	}

	JSONSuccess(w, map[string]interface{}{"trips": trips}, "trips retrieved successfully")
}

// GetTripHandler retrieves a single trip with all stops
func (h *Handler) GetTripHandler(w http.ResponseWriter, r *http.Request) {
	tripIDStr := chi.URLParam(r, "id")
	tripID, err := strconv.ParseInt(tripIDStr, 10, 64)
	if err != nil {
		JSONError(w, "invalid trip id", http.StatusBadRequest)
		return
	}

	trip, err := h.service.repo.GetTrip(r.Context(), tripID)
	if err != nil {
		JSONErrorFrom(w, err, http.StatusNotFound)
		return
	}

	companyID := int64(1)
	if trip.CompanyID != companyID {
		JSONError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	stops, err := h.service.repo.GetTripStops(r.Context(), tripID)
	if err != nil {
		JSONErrorFrom(w, err, http.StatusInternalServerError)
		return
	}

	res := map[string]interface{}{
		"trip":  trip,
		"stops": stops,
	}

	JSONSuccess(w, res, "trip retrieved successfully")
}

// AddTripStopRequest defines request body for adding a stop to a trip
type AddTripStopRequest struct {
	ShipmentID       *int64   `json:"shipment_id"`
	StopSequence     int      `json:"stop_sequence"`
	StopType         string   `json:"stop_type"` // PICKUP, DELIVERY, TRANSFER
	WarehouseID      *int64   `json:"warehouse_id"`
	LocationAddress  string   `json:"location_address"`
	LocationCity     string   `json:"location_city"`
	LocationLat      *float64 `json:"location_lat"`
	LocationLon      *float64 `json:"location_lon"`
	ContactName      string   `json:"contact_name"`
	ContactPhone     string   `json:"contact_phone"`
	PlannedArrivalAt *string  `json:"planned_arrival_at"` // ISO8601
}

// AddTripStopHandler adds a pickup/delivery stop to a trip
func (h *Handler) AddTripStopHandler(w http.ResponseWriter, r *http.Request) {
	tripIDStr := chi.URLParam(r, "id")
	tripID, err := strconv.ParseInt(tripIDStr, 10, 64)
	if err != nil {
		JSONError(w, "invalid trip id", http.StatusBadRequest)
		return
	}
	companyID := int64(1)

	var req AddTripStopRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		JSONError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	input := AddTripStopInput{
		CompanyID:       companyID,
		TripID:          tripID,
		ShipmentID:      req.ShipmentID,
		StopSequence:    req.StopSequence,
		StopType:        StopType(req.StopType),
		WarehouseID:     req.WarehouseID,
		LocationAddress: req.LocationAddress,
		LocationCity:    req.LocationCity,
		LocationLat:     req.LocationLat,
		LocationLon:     req.LocationLon,
		ContactName:     req.ContactName,
		ContactPhone:    req.ContactPhone,
	}

	if req.PlannedArrivalAt != nil {
		t, err := time.Parse(time.RFC3339, *req.PlannedArrivalAt)
		if err != nil {
			JSONError(w, "invalid planned_arrival_at format", http.StatusBadRequest)
			return
		}
		input.PlannedArrivalAt = &t
	}

	stop, err := h.service.AddStopToTrip(r.Context(), input)
	if err != nil {
		JSONErrorFrom(w, err, http.StatusInternalServerError)
		return
	}

	JSONSuccess(w, stop, "stop added to trip successfully")
}

// DispatchTripHandler dispatches a trip to the driver
func (h *Handler) DispatchTripHandler(w http.ResponseWriter, r *http.Request) {
	tripIDStr := chi.URLParam(r, "id")
	tripID, err := strconv.ParseInt(tripIDStr, 10, 64)
	if err != nil {
		JSONError(w, "invalid trip id", http.StatusBadRequest)
		return
	}

	err = h.service.DispatchTrip(r.Context(), tripID)
	if err != nil {
		JSONErrorFrom(w, err, http.StatusInternalServerError)
		return
	}

	JSONSuccess(w, map[string]interface{}{"trip_id": tripID, "status": TripStatusDispatched}, "trip dispatched successfully")
}

// ═══════════════════════════════════════════════════════════════════════════
// HELPER RESPONSE TYPES
// ═══════════════════════════════════════════════════════════════════════════

type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
	Code    int    `json:"code"`
}

type SuccessResponse struct {
	Data    interface{} `json:"data"`
	Message string      `json:"message"`
}

// JSONError returns a JSON error response
func JSONError(w http.ResponseWriter, message string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(ErrorResponse{
		Error:   http.StatusText(code),
		Message: message,
		Code:    code,
	})
}

// JSONErrorFrom preserves the logistics response shape while applying the
// shared safe-message policy to internal errors.
func JSONErrorFrom(w http.ResponseWriter, err error, code int) {
	JSONError(w, shared.UserSafeMessage(err), code)
}

// JSONSuccess returns a JSON success response
func JSONSuccess(w http.ResponseWriter, data interface{}, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(SuccessResponse{
		Data:    data,
		Message: message,
	})
}
