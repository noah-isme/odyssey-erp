package distribution

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	accountingmoney "github.com/odyssey-erp/odyssey-erp/internal/accounting/money"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// MountRoutes exposes the distribution lifecycle under /distribution.
func (h *Handler) MountRoutes(r chi.Router) {
	r.Route("/planning", func(r chi.Router) {
		r.Get("/horizons", h.ListPlanningHorizonsHandler)
		r.Post("/horizons", h.CreatePlanningHorizonHandler)
		r.Post("/rules", h.AddPlanningRuleHandler)
	})
	r.Route("/loads", func(r chi.Router) {
		r.Get("/", h.ListLoadsHandler)
		r.Post("/", h.CreateLoadHandler)
		r.Route("/{id}", func(r chi.Router) {
			// Static child paths are registered before the resource root so the
			// lifecycle actions are unambiguous to chi.
			r.Post("/shipments", h.CreateShipmentHandler)
			r.Post("/ready", h.MarkLoadReadyHandler)
			r.Post("/dispatch", h.DispatchLoadHandler)
			r.Post("/deliver", h.DeliverLoadHandler)
			r.Get("/utilization", h.GetLoadUtilizationHandler)
			r.Get("/", h.GetLoadHandler)
		})
	})
	r.Route("/routes", func(r chi.Router) {
		r.Get("/", h.ListRoutesHandler)
		r.Post("/", h.PlanRouteHandler)
		r.Route("/{id}", func(r chi.Router) {
			r.Post("/stops", h.AddRouteStopHandler)
			r.Post("/optimize", h.OptimizeRouteHandler)
			r.Post("/approve", h.ApproveRouteHandler)
			r.Get("/metrics", h.GetRouteMetricsHandler)
			r.Get("/", h.GetRouteHandler)
		})
	})
	r.Route("/transfers", func(r chi.Router) {
		r.Get("/", h.ListTransfersHandler)
		r.Post("/", h.CreateTransferHandler)
		r.Route("/{id}", func(r chi.Router) {
			r.Post("/lines", h.AddTransferItemHandler)
			r.Post("/approve", h.ApproveTransferHandler)
			r.Post("/dispatch", h.DispatchTransferHandler)
			r.Post("/receive", h.ReceiveTransferHandler)
			r.Get("/", h.GetTransferHandler)
		})
	})
}

type CreatePlanningHorizonRequest struct {
	WarehouseID       int64   `json:"warehouse_id"`
	PlanningStartDate string  `json:"planning_start_date"`
	PlanningEndDate   string  `json:"planning_end_date"`
	FrozenUntilDate   *string `json:"frozen_until_date"`
	Notes             string  `json:"notes"`
}

func (h *Handler) CreatePlanningHorizonHandler(w http.ResponseWriter, r *http.Request) {
	companyID, userID, err := requestScope(r)
	if err != nil {
		respondError(w, err)
		return
	}
	var request CreatePlanningHorizonRequest
	if err := shared.DecodeJSON(r, &request); err != nil {
		respondClientError(w, err)
		return
	}
	start, err := parseDate(request.PlanningStartDate)
	if err != nil {
		respondClientError(w, err)
		return
	}
	end, err := parseDate(request.PlanningEndDate)
	if err != nil {
		respondClientError(w, err)
		return
	}
	frozen, err := parseOptionalDate(request.FrozenUntilDate)
	if err != nil {
		respondClientError(w, err)
		return
	}
	horizon, err := h.service.SetupPlanningHorizon(r.Context(), CreatePlanningHorizonInput{
		CompanyID:         companyID,
		WarehouseID:       request.WarehouseID,
		PlanningStartDate: start,
		PlanningEndDate:   end,
		FrozenUntilDate:   frozen,
		Notes:             request.Notes,
		CreatedBy:         userID,
	})
	if err != nil {
		respondError(w, err)
		return
	}
	shared.JSONResponse(w, http.StatusCreated, map[string]any{"horizon": horizon})
}

func (h *Handler) ListPlanningHorizonsHandler(w http.ResponseWriter, r *http.Request) {
	companyID, _, err := requestScope(r)
	if err != nil {
		respondError(w, err)
		return
	}
	horizons, err := h.service.ListPlanningHorizons(r.Context(), companyID)
	if err != nil {
		respondError(w, err)
		return
	}
	shared.JSONResponse(w, http.StatusOK, map[string]any{"horizons": horizons})
}

type AddPlanningRuleRequest struct {
	WarehouseID          int64   `json:"warehouse_id"`
	RuleName             string  `json:"rule_name"`
	RuleType             string  `json:"rule_type"`
	MaxLoadWeightKg      *string `json:"max_load_weight_kg"`
	MaxLoadVolumeCbm     *string `json:"max_load_volume_cbm"`
	MaxItemsPerLoad      *int    `json:"max_items_per_load"`
	TimeWindowStart      *string `json:"time_window_start"`
	TimeWindowEnd        *string `json:"time_window_end"`
	VehicleTypeRequired  string  `json:"vehicle_type_required"`
	CustomRuleExpression string  `json:"custom_rule_expression"`
	Priority             int     `json:"priority"`
}

func (h *Handler) AddPlanningRuleHandler(w http.ResponseWriter, r *http.Request) {
	companyID, userID, err := requestScope(r)
	if err != nil {
		respondError(w, err)
		return
	}
	var request AddPlanningRuleRequest
	if err := shared.DecodeJSON(r, &request); err != nil {
		respondClientError(w, err)
		return
	}
	weight, err := parseMoney(request.MaxLoadWeightKg, 4)
	if err != nil {
		respondClientError(w, err)
		return
	}
	volume, err := parseMoney(request.MaxLoadVolumeCbm, 4)
	if err != nil {
		respondClientError(w, err)
		return
	}
	windowStart, err := parseOptionalClock(request.TimeWindowStart)
	if err != nil {
		respondClientError(w, err)
		return
	}
	windowEnd, err := parseOptionalClock(request.TimeWindowEnd)
	if err != nil {
		respondClientError(w, err)
		return
	}
	rule, err := h.service.AddPlanningRule(r.Context(), CreatePlanningRuleInput{
		CompanyID:            companyID,
		WarehouseID:          request.WarehouseID,
		RuleName:             request.RuleName,
		RuleType:             RuleType(request.RuleType),
		MaxLoadWeightKg:      weight,
		MaxLoadVolumeCbm:     volume,
		MaxItemsPerLoad:      request.MaxItemsPerLoad,
		TimeWindowStart:      windowStart,
		TimeWindowEnd:        windowEnd,
		VehicleTypeRequired:  request.VehicleTypeRequired,
		CustomRuleExpression: request.CustomRuleExpression,
		Priority:             request.Priority,
		CreatedBy:            userID,
	})
	if err != nil {
		respondError(w, err)
		return
	}
	shared.JSONResponse(w, http.StatusCreated, map[string]any{"rule": rule})
}

type CreateLoadRequest struct {
	OriginWarehouseID      int64   `json:"origin_warehouse_id"`
	DestinationWarehouseID *int64  `json:"destination_warehouse_id"`
	DestinationAddress     string  `json:"destination_address"`
	DestinationCity        string  `json:"destination_city"`
	DestinationCountry     string  `json:"destination_country"`
	PlannedPickupDate      *string `json:"planned_pickup_date"`
	PlannedDeliveryDate    *string `json:"planned_delivery_date"`
	Notes                  string  `json:"notes"`
}

func (h *Handler) CreateLoadHandler(w http.ResponseWriter, r *http.Request) {
	companyID, userID, err := requestScope(r)
	if err != nil {
		respondError(w, err)
		return
	}
	var request CreateLoadRequest
	if err := shared.DecodeJSON(r, &request); err != nil {
		respondClientError(w, err)
		return
	}
	pickup, err := parseOptionalDate(request.PlannedPickupDate)
	if err != nil {
		respondClientError(w, err)
		return
	}
	delivery, err := parseOptionalDate(request.PlannedDeliveryDate)
	if err != nil {
		respondClientError(w, err)
		return
	}
	load, err := h.service.CreateLoad(r.Context(), CreateLoadInput{
		CompanyID:              companyID,
		OriginWarehouseID:      request.OriginWarehouseID,
		DestinationWarehouseID: request.DestinationWarehouseID,
		DestinationAddress:     request.DestinationAddress,
		DestinationCity:        request.DestinationCity,
		DestinationCountry:     request.DestinationCountry,
		PlannedPickupDate:      pickup,
		PlannedDeliveryDate:    delivery,
		Notes:                  request.Notes,
		CreatedBy:              userID,
	})
	if err != nil {
		respondError(w, err)
		return
	}
	shared.JSONResponse(w, http.StatusCreated, map[string]any{"load": load})
}

func (h *Handler) ListLoadsHandler(w http.ResponseWriter, r *http.Request) {
	companyID, _, err := requestScope(r)
	if err != nil {
		respondError(w, err)
		return
	}
	status, err := parseLoadStatus(r.URL.Query().Get("status"))
	if err != nil {
		respondClientError(w, err)
		return
	}
	loads, err := h.service.ListLoads(r.Context(), companyID, status)
	if err != nil {
		respondError(w, err)
		return
	}
	shared.JSONResponse(w, http.StatusOK, map[string]any{"loads": loads})
}

func (h *Handler) GetLoadHandler(w http.ResponseWriter, r *http.Request) {
	loadID, err := pathID(r, "id")
	if err != nil {
		respondClientError(w, err)
		return
	}
	load, items, err := h.service.GetLoad(r.Context(), loadID)
	if err != nil {
		respondError(w, err)
		return
	}
	if err := verifyCompany(r, load.CompanyID); err != nil {
		respondError(w, err)
		return
	}
	shared.JSONResponse(w, http.StatusOK, map[string]any{"load": load, "items": items})
}

type CreateShipmentRequest struct {
	ShipmentNumber         string                      `json:"shipment_number"`
	ShipmentType           string                      `json:"shipment_type"`
	DestinationAddress     string                      `json:"destination_address"`
	DestinationCity        string                      `json:"destination_city"`
	DestinationCountry     string                      `json:"destination_country"`
	DestinationWarehouseID *int64                      `json:"destination_warehouse_id"`
	PlannedDispatchAt      *string                     `json:"planned_dispatch_at"`
	PlannedDeliveryAt      *string                     `json:"planned_delivery_at"`
	Lines                  []CreateShipmentLineRequest `json:"lines"`
}

type CreateShipmentLineRequest struct {
	ProductID int64    `json:"product_id"`
	Quantity  float64  `json:"quantity"`
	WeightKg  *float64 `json:"weight_kg"`
	VolumeCbm *float64 `json:"volume_cbm"`
}

func (h *Handler) CreateShipmentHandler(w http.ResponseWriter, r *http.Request) {
	_, userID, err := requestScope(r)
	if err != nil {
		respondError(w, err)
		return
	}
	loadID, err := pathID(r, "id")
	if err != nil {
		respondClientError(w, err)
		return
	}
	var request CreateShipmentRequest
	if err := shared.DecodeJSON(r, &request); err != nil {
		respondClientError(w, err)
		return
	}
	dispatchAt, err := parseOptionalTimestamp(request.PlannedDispatchAt)
	if err != nil {
		respondClientError(w, err)
		return
	}
	deliveryAt, err := parseOptionalTimestamp(request.PlannedDeliveryAt)
	if err != nil {
		respondClientError(w, err)
		return
	}
	lines := make([]ShipmentLineInput, 0, len(request.Lines))
	for _, line := range request.Lines {
		lines = append(lines, ShipmentLineInput{ProductID: line.ProductID, Quantity: line.Quantity, WeightKg: line.WeightKg, VolumeCbm: line.VolumeCbm})
	}
	shipmentID, err := h.service.CreateShipmentForLoad(r.Context(), loadID, ShipmentCreateInput{
		ShipmentNumber:         request.ShipmentNumber,
		ShipmentType:           request.ShipmentType,
		DestinationWarehouseID: request.DestinationWarehouseID,
		DestinationAddress:     request.DestinationAddress,
		DestinationCity:        request.DestinationCity,
		DestinationCountry:     request.DestinationCountry,
		PlannedDispatchAt:      dispatchAt,
		PlannedDeliveryAt:      deliveryAt,
		CreatedBy:              userID,
	}, lines)
	if err != nil {
		respondError(w, err)
		return
	}
	shared.JSONResponse(w, http.StatusCreated, map[string]any{"shipment_id": shipmentID})
}

type DispatchLoadRequest struct {
	VehicleID      *int64  `json:"vehicle_id"`
	DriverID       *int64  `json:"driver_id"`
	CarrierID      *int64  `json:"carrier_id"`
	CarrierService *string `json:"carrier_service_type"`
}

func (h *Handler) DispatchLoadHandler(w http.ResponseWriter, r *http.Request) {
	_, _, err := requestScope(r)
	if err != nil {
		respondError(w, err)
		return
	}
	loadID, err := pathID(r, "id")
	if err != nil {
		respondClientError(w, err)
		return
	}
	var request DispatchLoadRequest
	if err := shared.DecodeJSON(r, &request); err != nil {
		respondClientError(w, err)
		return
	}
	if err := h.service.DispatchLoad(r.Context(), loadID, request.VehicleID, request.DriverID, request.CarrierID, request.CarrierService); err != nil {
		respondError(w, err)
		return
	}
	shared.JSONResponse(w, http.StatusOK, map[string]any{"status": LoadStatusInTransit})
}

type DeliverLoadRequest struct {
	DeliveredAt *string `json:"delivered_at"`
}

func (h *Handler) MarkLoadReadyHandler(w http.ResponseWriter, r *http.Request) {
	if _, _, err := requestScope(r); err != nil {
		respondError(w, err)
		return
	}
	loadID, err := pathID(r, "id")
	if err != nil {
		respondClientError(w, err)
		return
	}
	_, err = h.service.MarkLoadReady(r.Context(), loadID)
	if err != nil {
		respondError(w, err)
		return
	}
	shared.JSONResponse(w, http.StatusOK, map[string]any{"status": LoadStatusReady})
}

func (h *Handler) DeliverLoadHandler(w http.ResponseWriter, r *http.Request) {
	_, userID, err := requestScope(r)
	if err != nil {
		respondError(w, err)
		return
	}
	loadID, err := pathID(r, "id")
	if err != nil {
		respondClientError(w, err)
		return
	}
	var request DeliverLoadRequest
	if r.Body != nil && r.ContentLength != 0 {
		if err := shared.DecodeJSON(r, &request); err != nil {
			respondClientError(w, err)
			return
		}
	}
	deliveredAt := time.Now().UTC()
	if request.DeliveredAt != nil {
		deliveredAt, err = parseTimestamp(*request.DeliveredAt)
		if err != nil {
			respondClientError(w, err)
			return
		}
	}
	if err := h.service.DeliverLoad(r.Context(), loadID, userID, deliveredAt); err != nil {
		respondError(w, err)
		return
	}
	shared.JSONResponse(w, http.StatusOK, map[string]any{"status": LoadStatusDelivered})
}

type PlanRouteRequest struct {
	LoadID               int64    `json:"load_id"`
	TotalDistanceKm      *float64 `json:"total_distance_km"`
	EstimatedDurationMin *int     `json:"estimated_duration_minutes"`
	OptimizationScore    *string  `json:"optimization_score"`
}

func (h *Handler) PlanRouteHandler(w http.ResponseWriter, r *http.Request) {
	companyID, userID, err := requestScope(r)
	if err != nil {
		respondError(w, err)
		return
	}
	var request PlanRouteRequest
	if err := shared.DecodeJSON(r, &request); err != nil {
		respondClientError(w, err)
		return
	}
	score, err := parseMoney(request.OptimizationScore, 2)
	if err != nil {
		respondClientError(w, err)
		return
	}
	route, err := h.service.PlanDeliveryRoute(r.Context(), CreateRouteInput{
		CompanyID:                companyID,
		LoadID:                   request.LoadID,
		TotalDistanceKm:          request.TotalDistanceKm,
		EstimatedDurationMinutes: request.EstimatedDurationMin,
		OptimizationScore:        score,
		CreatedBy:                userID,
	})
	if err != nil {
		respondError(w, err)
		return
	}
	shared.JSONResponse(w, http.StatusCreated, map[string]any{"route": route})
}

func (h *Handler) ListRoutesHandler(w http.ResponseWriter, r *http.Request) {
	companyID, _, err := requestScope(r)
	if err != nil {
		respondError(w, err)
		return
	}
	status, err := parseRouteStatus(r.URL.Query().Get("status"))
	if err != nil {
		respondClientError(w, err)
		return
	}
	routes, err := h.service.repo.ListRoutes(r.Context(), companyID, status)
	if err != nil {
		respondError(w, err)
		return
	}
	shared.JSONResponse(w, http.StatusOK, map[string]any{"routes": routes})
}

func (h *Handler) GetRouteHandler(w http.ResponseWriter, r *http.Request) {
	routeID, err := pathID(r, "id")
	if err != nil {
		respondClientError(w, err)
		return
	}
	route, err := h.service.repo.GetRoute(r.Context(), routeID)
	if err != nil {
		respondError(w, err)
		return
	}
	if err := verifyCompany(r, route.CompanyID); err != nil {
		respondError(w, err)
		return
	}
	stops, err := h.service.repo.GetRouteStops(r.Context(), routeID)
	if err != nil {
		respondError(w, err)
		return
	}
	shared.JSONResponse(w, http.StatusOK, map[string]any{"route": route, "stops": stops})
}

func (h *Handler) OptimizeRouteHandler(w http.ResponseWriter, r *http.Request) {
	if _, _, err := requestScope(r); err != nil {
		respondError(w, err)
		return
	}
	routeID, err := pathID(r, "id")
	if err != nil {
		respondClientError(w, err)
		return
	}
	if err := h.service.OptimizeRoute(r.Context(), routeID); err != nil {
		respondError(w, err)
		return
	}
	shared.JSONResponse(w, http.StatusOK, map[string]any{"status": RouteStatusOptimized})
}

func (h *Handler) ApproveRouteHandler(w http.ResponseWriter, r *http.Request) {
	if _, _, err := requestScope(r); err != nil {
		respondError(w, err)
		return
	}
	routeID, err := pathID(r, "id")
	if err != nil {
		respondClientError(w, err)
		return
	}
	if err := h.service.ApproveRoute(r.Context(), routeID); err != nil {
		respondError(w, err)
		return
	}
	shared.JSONResponse(w, http.StatusOK, map[string]any{"status": RouteStatusApproved})
}

type AddRouteStopRequest struct {
	StopSequence         int64    `json:"stop_sequence"`
	StopType             string   `json:"stop_type"`
	WarehouseID          *int64   `json:"warehouse_id"`
	CustomerID           *int64   `json:"customer_id"`
	CustomerAddress      string   `json:"customer_address"`
	CustomerCity         string   `json:"customer_city"`
	LocationLat          *float64 `json:"location_lat"`
	LocationLon          *float64 `json:"location_lon"`
	ContactName          string   `json:"contact_name"`
	ContactPhone         string   `json:"contact_phone"`
	PlannedArrivalTime   *string  `json:"planned_arrival_time"`
	PlannedDepartureTime *string  `json:"planned_departure_time"`
	Notes                string   `json:"notes"`
}

func (h *Handler) AddRouteStopHandler(w http.ResponseWriter, r *http.Request) {
	companyID, _, err := requestScope(r)
	if err != nil {
		respondError(w, err)
		return
	}
	routeID, err := pathID(r, "id")
	if err != nil {
		respondClientError(w, err)
		return
	}
	var request AddRouteStopRequest
	if err := shared.DecodeJSON(r, &request); err != nil {
		respondClientError(w, err)
		return
	}
	arrival, err := parseOptionalClock(request.PlannedArrivalTime)
	if err != nil {
		respondClientError(w, err)
		return
	}
	departure, err := parseOptionalClock(request.PlannedDepartureTime)
	if err != nil {
		respondClientError(w, err)
		return
	}
	stop, err := h.service.AddDeliveryStop(r.Context(), AddRouteStopInput{
		CompanyID:            companyID,
		RouteID:              routeID,
		StopSequence:         int(request.StopSequence),
		StopType:             StopType(request.StopType),
		WarehouseID:          request.WarehouseID,
		CustomerID:           request.CustomerID,
		CustomerAddress:      request.CustomerAddress,
		CustomerCity:         request.CustomerCity,
		LocationLat:          request.LocationLat,
		LocationLon:          request.LocationLon,
		ContactName:          request.ContactName,
		ContactPhone:         request.ContactPhone,
		PlannedArrivalTime:   arrival,
		PlannedDepartureTime: departure,
		Notes:                request.Notes,
	})
	if err != nil {
		respondError(w, err)
		return
	}
	shared.JSONResponse(w, http.StatusCreated, map[string]any{"stop": stop})
}

type CreateTransferOrderRequest struct {
	FromWarehouseID     int64   `json:"from_warehouse_id"`
	ToWarehouseID       int64   `json:"to_warehouse_id"`
	PlannedDispatchDate *string `json:"planned_dispatch_date"`
	PlannedArrivalDate  *string `json:"planned_arrival_date"`
	Notes               string  `json:"notes"`
}

func (h *Handler) CreateTransferHandler(w http.ResponseWriter, r *http.Request) {
	companyID, userID, err := requestScope(r)
	if err != nil {
		respondError(w, err)
		return
	}
	var request CreateTransferOrderRequest
	if err := shared.DecodeJSON(r, &request); err != nil {
		respondClientError(w, err)
		return
	}
	dispatchDate, err := parseOptionalDate(request.PlannedDispatchDate)
	if err != nil {
		respondClientError(w, err)
		return
	}
	arrivalDate, err := parseOptionalDate(request.PlannedArrivalDate)
	if err != nil {
		respondClientError(w, err)
		return
	}
	transfer, err := h.service.CreateTransferOrder(r.Context(), CreateTransferOrderInput{
		CompanyID:           companyID,
		FromWarehouseID:     request.FromWarehouseID,
		ToWarehouseID:       request.ToWarehouseID,
		PlannedDispatchDate: dispatchDate,
		PlannedArrivalDate:  arrivalDate,
		Notes:               request.Notes,
		CreatedBy:           userID,
	})
	if err != nil {
		respondError(w, err)
		return
	}
	shared.JSONResponse(w, http.StatusCreated, map[string]any{"transfer": transfer})
}

func (h *Handler) ListTransfersHandler(w http.ResponseWriter, r *http.Request) {
	companyID, _, err := requestScope(r)
	if err != nil {
		respondError(w, err)
		return
	}
	status, err := parseTransferStatus(r.URL.Query().Get("status"))
	if err != nil {
		respondClientError(w, err)
		return
	}
	transfers, err := h.service.ListTransfers(r.Context(), companyID, status)
	if err != nil {
		respondError(w, err)
		return
	}
	shared.JSONResponse(w, http.StatusOK, map[string]any{"transfers": transfers})
}

func (h *Handler) GetTransferHandler(w http.ResponseWriter, r *http.Request) {
	transferID, err := pathID(r, "id")
	if err != nil {
		respondClientError(w, err)
		return
	}
	transfer, lines, err := h.service.GetTransfer(r.Context(), transferID)
	if err != nil {
		respondError(w, err)
		return
	}
	if err := verifyCompany(r, transfer.CompanyID); err != nil {
		respondError(w, err)
		return
	}
	shared.JSONResponse(w, http.StatusOK, map[string]any{"transfer": transfer, "lines": lines})
}

type AddTransferItemRequest struct {
	ProductID     int64    `json:"product_id"`
	Quantity      string   `json:"quantity"`
	LotNumber     string   `json:"lot_number"`
	SerialNumbers []string `json:"serial_numbers"`
}

func (h *Handler) AddTransferItemHandler(w http.ResponseWriter, r *http.Request) {
	if _, _, err := requestScope(r); err != nil {
		respondError(w, err)
		return
	}
	transferID, err := pathID(r, "id")
	if err != nil {
		respondClientError(w, err)
		return
	}
	var request AddTransferItemRequest
	if err := shared.DecodeJSON(r, &request); err != nil {
		respondClientError(w, err)
		return
	}
	quantity, err := parseRequiredMoney(request.Quantity, 4)
	if err != nil {
		respondClientError(w, err)
		return
	}
	if err := h.service.AddTransferItem(r.Context(), transferID, request.ProductID, quantity, request.LotNumber, request.SerialNumbers); err != nil {
		respondError(w, err)
		return
	}
	shared.JSONResponse(w, http.StatusCreated, map[string]any{"status": "added"})
}

func (h *Handler) ApproveTransferHandler(w http.ResponseWriter, r *http.Request) {
	if _, _, err := requestScope(r); err != nil {
		respondError(w, err)
		return
	}
	transferID, err := pathID(r, "id")
	if err != nil {
		respondClientError(w, err)
		return
	}
	if err := h.service.ApproveTransfer(r.Context(), transferID); err != nil {
		respondError(w, err)
		return
	}
	shared.JSONResponse(w, http.StatusOK, map[string]any{"status": TransferStatusApproved})
}

type DispatchTransferRequest struct {
	VehicleID *int64 `json:"vehicle_id"`
	DriverID  *int64 `json:"driver_id"`
	CarrierID *int64 `json:"carrier_id"`
}

func (h *Handler) DispatchTransferHandler(w http.ResponseWriter, r *http.Request) {
	if _, _, err := requestScope(r); err != nil {
		respondError(w, err)
		return
	}
	transferID, err := pathID(r, "id")
	if err != nil {
		respondClientError(w, err)
		return
	}
	var request DispatchTransferRequest
	if err := shared.DecodeJSON(r, &request); err != nil {
		respondClientError(w, err)
		return
	}
	if err := h.service.DispatchTransfer(r.Context(), transferID, request.VehicleID, request.DriverID, request.CarrierID); err != nil {
		respondError(w, err)
		return
	}
	shared.JSONResponse(w, http.StatusOK, map[string]any{"status": TransferStatusInTransit})
}

func (h *Handler) ReceiveTransferHandler(w http.ResponseWriter, r *http.Request) {
	if _, _, err := requestScope(r); err != nil {
		respondError(w, err)
		return
	}
	transferID, err := pathID(r, "id")
	if err != nil {
		respondClientError(w, err)
		return
	}
	var request struct {
		ReceivedAt *string `json:"received_at"`
	}
	if r.Body != nil && r.ContentLength != 0 {
		if err := shared.DecodeJSON(r, &request); err != nil {
			respondClientError(w, err)
			return
		}
	}
	receivedAt := time.Now().UTC()
	if request.ReceivedAt != nil {
		receivedAt, err = parseTimestamp(*request.ReceivedAt)
		if err != nil {
			respondClientError(w, err)
			return
		}
	}
	if err := h.service.ReceiveTransfer(r.Context(), transferID, receivedAt); err != nil {
		respondError(w, err)
		return
	}
	shared.JSONResponse(w, http.StatusOK, map[string]any{"status": TransferStatusReceived})
}

func (h *Handler) GetLoadUtilizationHandler(w http.ResponseWriter, r *http.Request) {
	loadID, err := pathID(r, "id")
	if err != nil {
		respondClientError(w, err)
		return
	}
	utilization, err := h.service.GetLoadUtilization(r.Context(), loadID)
	if err != nil {
		respondError(w, err)
		return
	}
	shared.JSONResponse(w, http.StatusOK, map[string]any{"utilization": utilization})
}

func (h *Handler) GetRouteMetricsHandler(w http.ResponseWriter, r *http.Request) {
	routeID, err := pathID(r, "id")
	if err != nil {
		respondClientError(w, err)
		return
	}
	metrics, err := h.service.GetRouteMetrics(r.Context(), routeID)
	if err != nil {
		respondError(w, err)
		return
	}
	shared.JSONResponse(w, http.StatusOK, map[string]any{"metrics": metrics})
}

func requestScope(r *http.Request) (int64, int64, error) {
	sess := shared.SessionFromContext(r.Context())
	if sess == nil || sess.User() == "" {
		return 0, 0, shared.ErrUnauthorized
	}
	userID, err := strconv.ParseInt(sess.User(), 10, 64)
	if err != nil || userID == 0 {
		return 0, 0, shared.ErrUnauthorized
	}
	companyID, err := strconv.ParseInt(sess.Get("company_id"), 10, 64)
	if err != nil || companyID == 0 {
		return 0, 0, shared.ErrInvalidInput
	}
	return companyID, userID, nil
}

func verifyCompany(r *http.Request, companyID int64) error {
	scopeCompany, _, err := requestScope(r)
	if err != nil {
		return err
	}
	if companyID != scopeCompany {
		return shared.ErrNotFound
	}
	return nil
}

func pathID(r *http.Request, name string) (int64, error) {
	id, err := strconv.ParseInt(chi.URLParam(r, name), 10, 64)
	if err != nil || id <= 0 {
		return 0, errors.New("invalid resource ID")
	}
	return id, nil
}

func respondError(w http.ResponseWriter, err error) {
	status := shared.HTTPStatus(err)
	if status == http.StatusInternalServerError && looksLikeClientError(err) {
		status = http.StatusBadRequest
	}
	shared.JSONErrorFrom(w, status, err)
}

func respondClientError(w http.ResponseWriter, err error) {
	shared.JSONErrorFrom(w, http.StatusBadRequest, err)
}

func looksLikeClientError(err error) bool {
	message := strings.ToLower(err.Error())
	for _, token := range []string{"required", "invalid", "cannot", "can only", "must ", "positive", "violates", "not configured", "does not match"} {
		if strings.Contains(message, token) {
			return true
		}
	}
	return false
}

func parseDate(value string) (time.Time, error) {
	parsed, err := time.Parse("2006-01-02", strings.TrimSpace(value))
	if err != nil {
		return time.Time{}, errors.New("date must use YYYY-MM-DD")
	}
	return parsed.UTC(), nil
}

func parseOptionalDate(value *string) (*time.Time, error) {
	if value == nil || strings.TrimSpace(*value) == "" {
		return nil, nil
	}
	parsed, err := parseDate(*value)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func parseTimestamp(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(value))
	if err != nil {
		return time.Time{}, errors.New("timestamp must use RFC3339")
	}
	return parsed.UTC(), nil
}

func parseOptionalTimestamp(value *string) (*time.Time, error) {
	if value == nil || strings.TrimSpace(*value) == "" {
		return nil, nil
	}
	parsed, err := parseTimestamp(*value)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func parseOptionalClock(value *string) (*time.Time, error) {
	if value == nil || strings.TrimSpace(*value) == "" {
		return nil, nil
	}
	parsed, err := time.Parse("15:04", strings.TrimSpace(*value))
	if err != nil {
		parsed, err = time.Parse("15:04:05", strings.TrimSpace(*value))
	}
	if err != nil {
		return nil, errors.New("time must use HH:MM or HH:MM:SS")
	}
	return &parsed, nil
}

func parseMoney(value *string, scale int) (*accountingmoney.Money, error) {
	if value == nil || strings.TrimSpace(*value) == "" {
		return nil, nil
	}
	parsed, err := accountingmoney.Parse(*value, scale)
	if err != nil {
		return nil, errors.New("amount must be a valid decimal")
	}
	return &parsed, nil
}

func parseRequiredMoney(value string, scale int) (accountingmoney.Money, error) {
	parsed, err := accountingmoney.Parse(value, scale)
	if err != nil {
		return accountingmoney.Money{}, errors.New("quantity must be a valid decimal")
	}
	return parsed, nil
}

func parseLoadStatus(value string) (*LoadStatus, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	status := LoadStatus(strings.ToUpper(value))
	switch status {
	case LoadStatusDraft, LoadStatusPlanned, LoadStatusReady, LoadStatusDispatched, LoadStatusInTransit, LoadStatusDelivered, LoadStatusCancelled:
		return &status, nil
	default:
		return nil, errors.New("invalid load status")
	}
}

func parseRouteStatus(value string) (*RouteStatus, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	status := RouteStatus(strings.ToUpper(value))
	switch status {
	case RouteStatusDraft, RouteStatusOptimized, RouteStatusApproved, RouteStatusActive, RouteStatusCompleted, RouteStatusCancelled:
		return &status, nil
	default:
		return nil, errors.New("invalid route status")
	}
}

func parseTransferStatus(value string) (*TransferStatus, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	status := TransferStatus(strings.ToUpper(value))
	switch status {
	case TransferStatusDraft, TransferStatusApproved, TransferStatusDispatched, TransferStatusInTransit, TransferStatusReceived, TransferStatusCancelled:
		return &status, nil
	default:
		return nil, errors.New("invalid transfer status")
	}
}
