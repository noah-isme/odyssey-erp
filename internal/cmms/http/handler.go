package cmmshttp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/odyssey-erp/odyssey-erp/internal/cmms"
	"github.com/odyssey-erp/odyssey-erp/internal/rbac"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/view"
)

// Handler wires HTTP endpoints for CMMS.
type Handler struct {
	logger      *slog.Logger
	service     *cmms.Service
	templates   *view.Engine
	csrf        *shared.CSRFManager
	rbac        rbac.Middleware
	pool        *pgxpool.Pool
	audit       *shared.AuditLogger
	idempotency *shared.IdempotencyStore
}

// NewHandler constructs a Handler value.
func NewHandler(logger *slog.Logger, service *cmms.Service, templates *view.Engine, csrf *shared.CSRFManager, rbac rbac.Middleware, pool *pgxpool.Pool) *Handler {
	var audit *shared.AuditLogger
	var idempotency *shared.IdempotencyStore
	if pool != nil {
		audit = shared.NewAuditLogger(pool)
		idempotency = shared.NewIdempotencyStore(pool)
	}
	return &Handler{
		logger:      logger,
		service:     service,
		templates:   templates,
		csrf:        csrf,
		rbac:        rbac,
		pool:        pool,
		audit:       audit,
		idempotency: idempotency,
	}
}

// MountRoutes registers HTTP routes for CMMS.
func (h *Handler) MountRoutes(r chi.Router) {
	// Work Orders
	r.Route("/work-orders", func(r chi.Router) {
		r.Use(h.rbac.RequireAny(shared.PermCMMSWorkOrderView))
		r.Get("/", h.listWorkOrders)
		r.Get("/new", h.newWorkOrderForm)
		r.With(h.rbac.RequireAny(shared.PermCMMSRequestCreate)).Post("/", h.createWorkOrder)
		r.Get("/{id}", h.getWorkOrder)
		r.With(h.rbac.RequireAny(shared.PermCMMSWorkOrderExecute)).Post("/{id}/status", h.updateWorkOrderStatus)
		r.With(h.rbac.RequireAny(shared.PermCMMSWorkOrderExecute)).Post("/{id}/complete", h.completeWorkOrder)
		r.With(h.rbac.RequireAny(shared.PermCMMSWorkOrderClose)).Post("/{id}/close", h.closeWorkOrder)
		r.With(h.rbac.RequireAny(shared.PermCMMSWorkOrderExecute)).Post("/{id}/spare-parts", h.addWorkOrderSparePart)
	})
	r.Route("/work-order-spare-parts", func(r chi.Router) {
		r.Use(h.rbac.RequireAny(shared.PermCMMSWorkOrderView))
		r.With(h.rbac.RequireAny(shared.PermCMMSWorkOrderExecute)).Post("/{id}/issue", h.issueWorkOrderSparePart)
	})

	// Assets
	r.Route("/assets", func(r chi.Router) {
		r.Use(h.rbac.RequireAny(shared.PermCMMSAssetView))
		r.Get("/", h.listAssets)
		r.Get("/new", h.newAssetForm)
		r.With(h.rbac.RequireAny(shared.PermCMMSAssetManage)).Post("/", h.createAsset)
		r.Get("/{id}/meter-readings", h.listMeterReadings)
		r.With(h.rbac.RequireAny(shared.PermCMMSAssetManage)).Post("/{id}/meter-readings", h.recordMeterReading)
		r.Get("/{id}", h.getAsset)
	})

	// Preventive Maintenance Schedules
	r.Route("/pm-schedules", func(r chi.Router) {
		r.Use(h.rbac.RequireAny(shared.PermCMMSPlanView))
		r.Get("/", h.listPMSchedules)
		r.Get("/new", h.newPMScheduleForm)
		r.With(h.rbac.RequireAny(shared.PermCMMSPlanManage)).Post("/", h.createPMSchedule)
		r.With(h.rbac.RequireAny(shared.PermCMMSPlanManage)).Post("/run-due", h.runDuePMSchedules)
		r.Get("/{id}", h.getPMSchedule)
	})

	// Spare Parts
	r.Route("/spare-parts", func(r chi.Router) {
		r.Use(h.rbac.RequireAny(shared.PermCMMSAssetView))
		r.Get("/", h.listSpareParts)
		r.Get("/new", h.newSparePartForm)
		r.With(h.rbac.RequireAny(shared.PermCMMSAssetManage)).Post("/", h.createSparePart)
	})

	// IoT and Predictive Maintenance
	r.Route("/iot", func(r chi.Router) {
		r.Use(h.rbac.RequireAny(shared.PermCMMSAssetView))
		r.Post("/sensors", h.registerIoTSensor)
		r.Post("/readings", h.recordIoTReading)
	})
	r.Route("/predictive", func(r chi.Router) {
		r.Use(h.rbac.RequireAny(shared.PermCMMSPlanView))
		r.Post("/models", h.createPredictiveModel)
		r.Post("/evaluate", h.evaluatePredictiveAlerts)
	})
}

// ============================================================================
// Work Orders
// ============================================================================

type workOrderListData struct {
	WorkOrders []cmms.WorkOrder
	Filter     cmms.ListWorkOrdersFilter
	Statuses   []cmms.Status
	Priorities []cmms.Priority
}

func (h *Handler) listWorkOrders(w http.ResponseWriter, r *http.Request) {
	companyID := currentCompany(r)
	filter := cmms.ListWorkOrdersFilter{
		CompanyID: companyID,
		Limit:     50,
		Offset:    0,
	}
	if s := r.URL.Query().Get("status"); s != "" {
		status := cmms.NormaliseStatus(s)
		filter.Status = &status
	}
	if cat := r.URL.Query().Get("category"); cat != "" {
		filter.Category = strings.ToUpper(cat)
	}

	orders, err := h.service.ListWorkOrders(r.Context(), filter)
	if err != nil {
		h.logger.Error("list work orders", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	data := workOrderListData{
		WorkOrders: orders,
		Filter:     filter,
		Statuses: []cmms.Status{
			cmms.WorkOrderStatusDraft, cmms.WorkOrderStatusPlanned, cmms.WorkOrderStatusScheduled,
			cmms.WorkOrderStatusInProgress, cmms.WorkOrderStatusOnHold,
			cmms.WorkOrderStatusCompleted, cmms.WorkOrderStatusClosed,
		},
		Priorities: []cmms.Priority{cmms.PriorityLow, cmms.PriorityMedium, cmms.PriorityHigh, cmms.PriorityCritical},
	}
	h.render(w, r, "pages/cmms/work_orders.html", "Work Orders", data)
}

func (h *Handler) newWorkOrderForm(w http.ResponseWriter, r *http.Request) {
	companyID := currentCompany(r)
	assets, _ := h.service.ListAssets(r.Context(), cmms.ListAssetsFilter{CompanyID: companyID, Limit: 200})
	locations, _ := h.service.ListLocations(r.Context(), companyID)
	h.render(w, r, "pages/cmms/work_order_new.html", "New Work Order", map[string]any{
		"Assets":     assets,
		"Locations":  locations,
		"Categories": []string{"CORRECTIVE", "PREVENTIVE", "INSPECTION", "EMERGENCY", "CALIBRATION"},
		"Priorities": []cmms.Priority{cmms.PriorityLow, cmms.PriorityMedium, cmms.PriorityHigh, cmms.PriorityCritical},
	})
}

func (h *Handler) createWorkOrder(w http.ResponseWriter, r *http.Request) {
	if err := h.verifyCSRF(r); err != nil {
		shared.WriteError(w, err)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	actorID := currentUser(r)
	companyID := currentCompany(r)

	estimatedHours, err := parseOptionalFloat64(r.PostFormValue("estimated_hours"))
	if err != nil || estimatedHours < 0 {
		h.redirectWithFlash(w, r, "/cmms/work-orders/new", "danger", "Estimated hours must be a non-negative number")
		return
	}
	req := cmms.CreateWorkOrderRequest{
		CompanyID:      companyID,
		Title:          strings.TrimSpace(r.PostFormValue("title")),
		Description:    strings.TrimSpace(r.PostFormValue("description")),
		Priority:       cmms.NormalisePriority(r.PostFormValue("priority")),
		Category:       strings.ToUpper(strings.TrimSpace(r.PostFormValue("category"))),
		RequesterID:    actorID,
		EstimatedHours: estimatedHours,
		ActorID:        actorID,
	}
	if assetID := parseInt64(r.PostFormValue("asset_id")); assetID > 0 {
		req.AssetID = &assetID
	}
	if locationID := parseInt64(r.PostFormValue("location_id")); locationID > 0 {
		req.LocationID = &locationID
	}
	if assigneeID := parseInt64(r.PostFormValue("assignee_id")); assigneeID > 0 {
		req.AssigneeID = &assigneeID
	}
	if ps := strings.TrimSpace(r.PostFormValue("planned_start")); ps != "" {
		if t, err := time.Parse("2006-01-02", ps); err == nil {
			req.PlannedStart = &t
		}
	}
	if pe := strings.TrimSpace(r.PostFormValue("planned_end")); pe != "" {
		if t, err := time.Parse("2006-01-02", pe); err == nil {
			req.PlannedEnd = &t
		}
	}

	wo, err := h.service.CreateWorkOrder(r.Context(), req)
	if err != nil {
		h.logger.Warn("create work order", slog.Any("error", err))
		h.redirectWithFlash(w, r, "/cmms/work-orders/new", "danger", shared.UserSafeMessage(err))
		return
	}
	if err := h.recordAudit(r.Context(), actorID, "CREATE", "cmms_work_order", strconv.FormatInt(wo.ID, 10), map[string]any{
		"company_id": companyID, "number": wo.Number, "status": wo.Status,
	}); err != nil {
		h.logger.Warn("audit create work order", slog.Any("error", err), slog.Int64("work_order_id", wo.ID))
	}
	h.redirectWithFlash(w, r, "/cmms/work-orders/"+strconv.FormatInt(wo.ID, 10), "success", "Work order created successfully")
}

func (h *Handler) getWorkOrder(w http.ResponseWriter, r *http.Request) {
	id := parseInt64(chi.URLParam(r, "id"))
	wo, err := h.service.GetWorkOrder(r.Context(), id)
	if err != nil {
		if errors.Is(err, cmms.ErrWorkOrderNotFound) {
			http.NotFound(w, r)
			return
		}
		h.logger.Error("get work order", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	if !ownedByCompany(wo.CompanyID, currentCompany(r)) {
		http.NotFound(w, r)
		return
	}
	parts, partsErr := h.service.ListWorkOrderSpareParts(r.Context(), wo.ID)
	if partsErr != nil {
		h.logger.Warn("list work order spare parts", slog.Any("error", partsErr), slog.Int64("work_order_id", wo.ID))
	}
	spareParts, sparePartsErr := h.service.ListSpareParts(r.Context(), wo.CompanyID)
	if sparePartsErr != nil {
		h.logger.Warn("list spare parts for work order", slog.Any("error", sparePartsErr), slog.Int64("work_order_id", wo.ID))
	}
	h.render(w, r, "pages/cmms/work_order_detail.html", "Work Order "+wo.Number, map[string]any{
		"WorkOrder":  wo,
		"SpareParts": parts,
		"Parts":      spareParts,
		"Statuses": []cmms.Status{
			cmms.WorkOrderStatusPlanned, cmms.WorkOrderStatusInProgress,
			cmms.WorkOrderStatusOnHold, cmms.WorkOrderStatusCompleted, cmms.WorkOrderStatusClosed,
			cmms.WorkOrderStatusCancelled,
		},
	})
}

func (h *Handler) updateWorkOrderStatus(w http.ResponseWriter, r *http.Request) {
	if err := h.verifyCSRF(r); err != nil {
		shared.WriteError(w, err)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	id := parseInt64(chi.URLParam(r, "id"))
	status := cmms.NormaliseStatus(r.PostFormValue("status"))
	actorID := currentUser(r)
	companyID := currentCompany(r)
	wo, err := h.ownedWorkOrder(r, id, companyID)
	if err != nil {
		h.redirectWithFlash(w, r, workOrderPath(id), "danger", shared.UserSafeMessage(err))
		return
	}
	if status == cmms.WorkOrderStatusClosed {
		h.redirectWithFlash(w, r, workOrderPath(id), "danger", "Closing a work order requires the close action")
		return
	}
	if !validLifecycleTransition(wo.Status, status) {
		h.redirectWithFlash(w, r, workOrderPath(id), "danger", "Work order cannot make that status transition")
		return
	}
	key := mutationKey(r, fmt.Sprintf("cmms:%d:work-order:%d:status:%s", companyID, id, status))
	duplicate, err := h.beginMutation(r.Context(), key, "cmms.work_order.status")
	if err != nil {
		h.logger.Error("start work order status mutation", slog.Any("error", err), slog.Int64("work_order_id", id))
		h.redirectWithFlash(w, r, workOrderPath(id), "danger", shared.UserSafeMessage(err))
		return
	}
	if duplicate {
		h.redirectWithFlash(w, r, workOrderPath(id), "success", "Status update already processed")
		return
	}

	updated, err := h.service.UpdateWorkOrderStatus(r.Context(), id, status, actorID)
	if err != nil {
		h.rollbackMutation(r.Context(), key)
		h.logger.Warn("update work order status", slog.Any("error", err))
		h.redirectWithFlash(w, r, workOrderPath(id), "danger", shared.UserSafeMessage(err))
		return
	}
	if err := h.recordAudit(r.Context(), actorID, "STATUS", "cmms_work_order", strconv.FormatInt(id, 10), map[string]any{
		"company_id": companyID, "from": wo.Status, "to": updated.Status,
	}); err != nil {
		h.logger.Warn("audit work order status", slog.Any("error", err), slog.Int64("work_order_id", id))
	}
	h.redirectWithFlash(w, r, workOrderPath(id), "success", "Status updated")
}

// completeWorkOrder records the terminal execution transition for a work order.
// Actual hours are accepted for the browser contract. The legacy CMMS service
// owns the status transition; when a database is available we persist the
// optional hours with a company predicate as an additive field update.
func (h *Handler) completeWorkOrder(w http.ResponseWriter, r *http.Request) {
	h.updateWorkOrderLifecycle(w, r, cmms.WorkOrderStatusCompleted, "complete")
}

// closeWorkOrder requires the dedicated close permission at the route boundary.
func (h *Handler) closeWorkOrder(w http.ResponseWriter, r *http.Request) {
	h.updateWorkOrderLifecycle(w, r, cmms.WorkOrderStatusClosed, "close")
}

func (h *Handler) updateWorkOrderLifecycle(w http.ResponseWriter, r *http.Request, status cmms.Status, action string) {
	if err := h.verifyCSRF(r); err != nil {
		shared.WriteError(w, err)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	id := parseInt64(chi.URLParam(r, "id"))
	companyID := currentCompany(r)
	actorID := currentUser(r)
	wo, err := h.ownedWorkOrder(r, id, companyID)
	if err != nil {
		h.redirectWithFlash(w, r, workOrderPath(id), "danger", shared.UserSafeMessage(err))
		return
	}
	if !validLifecycleTransition(wo.Status, status) {
		h.redirectWithFlash(w, r, workOrderPath(id), "danger", "Work order cannot make that status transition")
		return
	}
	var actualHours *float64
	if status == cmms.WorkOrderStatusCompleted {
		if raw := strings.TrimSpace(r.PostFormValue("actual_hours")); raw != "" {
			hours, parseErr := parseOptionalFloat64(raw)
			if parseErr != nil || hours < 0 {
				h.redirectWithFlash(w, r, workOrderPath(id), "danger", "Actual hours cannot be negative")
				return
			}
			actualHours = &hours
		}
	}
	key := mutationKey(r, fmt.Sprintf("cmms:%d:work-order:%d:%s", companyID, id, action))
	duplicate, err := h.beginMutation(r.Context(), key, "cmms.work_order."+action)
	if err != nil {
		h.logger.Error("start work order lifecycle mutation", slog.Any("error", err), slog.Int64("work_order_id", id))
		h.redirectWithFlash(w, r, workOrderPath(id), "danger", shared.UserSafeMessage(err))
		return
	}
	if duplicate {
		h.redirectWithFlash(w, r, workOrderPath(id), "success", "Work order transition already processed")
		return
	}
	updated, err := h.service.UpdateWorkOrderStatus(r.Context(), id, status, actorID)
	if err != nil {
		h.rollbackMutation(r.Context(), key)
		h.logger.Warn("update work order lifecycle", slog.Any("error", err), slog.Int64("work_order_id", id))
		h.redirectWithFlash(w, r, workOrderPath(id), "danger", shared.UserSafeMessage(err))
		return
	}
	if status == cmms.WorkOrderStatusCompleted {
		if actualHours != nil && h.pool != nil {
			if _, updateErr := h.pool.Exec(r.Context(), `UPDATE work_orders SET actual_hours=$1, updated_at=NOW() WHERE id=$2 AND company_id=$3`, *actualHours, id, companyID); updateErr != nil {
				h.logger.Warn("persist work order actual hours", slog.Any("error", updateErr), slog.Int64("work_order_id", id))
			}
		}
	}
	if err := h.recordAudit(r.Context(), actorID, strings.ToUpper(action), "cmms_work_order", strconv.FormatInt(id, 10), map[string]any{
		"company_id": companyID, "from": wo.Status, "to": updated.Status,
	}); err != nil {
		h.logger.Warn("audit work order lifecycle", slog.Any("error", err), slog.Int64("work_order_id", id))
	}
	h.redirectWithFlash(w, r, workOrderPath(id), "success", "Work order "+strings.ToLower(string(status))+"")
}

// ============================================================================
// Assets
// ============================================================================

func (h *Handler) listAssets(w http.ResponseWriter, r *http.Request) {
	companyID := currentCompany(r)
	filter := cmms.ListAssetsFilter{
		CompanyID: companyID,
		Search:    r.URL.Query().Get("q"),
		AssetType: r.URL.Query().Get("type"),
		Status:    r.URL.Query().Get("status"),
		Limit:     50,
	}
	assets, err := h.service.ListAssets(r.Context(), filter)
	if err != nil {
		h.logger.Error("list assets", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	h.render(w, r, "pages/cmms/assets.html", "Assets", map[string]any{
		"Assets":     assets,
		"Filter":     filter,
		"AssetTypes": []string{"EQUIPMENT", "FACILITY", "VEHICLE", "TOOL", "INFRASTRUCTURE"},
		"Statuses":   []string{"ACTIVE", "INACTIVE", "DECOMMISSIONED", "SCRAPPED"},
	})
}

func (h *Handler) newAssetForm(w http.ResponseWriter, r *http.Request) {
	companyID := currentCompany(r)
	locations, _ := h.service.ListLocations(r.Context(), companyID)
	assets, _ := h.service.ListAssets(r.Context(), cmms.ListAssetsFilter{CompanyID: companyID, Limit: 200})
	h.render(w, r, "pages/cmms/asset_new.html", "New Asset", map[string]any{
		"Locations":     locations,
		"Assets":        assets,
		"AssetTypes":    []string{"EQUIPMENT", "FACILITY", "VEHICLE", "TOOL", "INFRASTRUCTURE"},
		"Criticalities": []string{"A", "B", "C", "D"},
	})
}

func (h *Handler) createAsset(w http.ResponseWriter, r *http.Request) {
	if err := h.verifyCSRF(r); err != nil {
		shared.WriteError(w, err)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	actorID := currentUser(r)
	companyID := currentCompany(r)

	req := cmms.CreateAssetRequest{
		CompanyID:    companyID,
		Code:         strings.TrimSpace(r.PostFormValue("code")),
		Name:         strings.TrimSpace(r.PostFormValue("name")),
		Description:  strings.TrimSpace(r.PostFormValue("description")),
		AssetType:    strings.ToUpper(strings.TrimSpace(r.PostFormValue("asset_type"))),
		Manufacturer: strings.TrimSpace(r.PostFormValue("manufacturer")),
		Model:        strings.TrimSpace(r.PostFormValue("model")),
		SerialNumber: strings.TrimSpace(r.PostFormValue("serial_number")),
		Criticality:  strings.ToUpper(strings.TrimSpace(r.PostFormValue("criticality"))),
		ActorID:      actorID,
	}
	if locationID := parseInt64(r.PostFormValue("location_id")); locationID > 0 {
		req.LocationID = &locationID
	}
	if parentID := parseInt64(r.PostFormValue("parent_id")); parentID > 0 {
		req.ParentID = &parentID
	}

	asset, err := h.service.CreateAsset(r.Context(), req)
	if err != nil {
		h.logger.Warn("create asset", slog.Any("error", err))
		h.redirectWithFlash(w, r, "/cmms/assets/new", "danger", shared.UserSafeMessage(err))
		return
	}
	if err := h.recordAudit(r.Context(), actorID, "CREATE", "cmms_asset", strconv.FormatInt(asset.ID, 10), map[string]any{
		"company_id": companyID, "code": asset.Code,
	}); err != nil {
		h.logger.Warn("audit create asset", slog.Any("error", err), slog.Int64("asset_id", asset.ID))
	}
	h.redirectWithFlash(w, r, "/cmms/assets/"+strconv.FormatInt(asset.ID, 10), "success", "Asset created successfully")
}

func (h *Handler) getAsset(w http.ResponseWriter, r *http.Request) {
	id := parseInt64(chi.URLParam(r, "id"))
	asset, err := h.service.GetAsset(r.Context(), id)
	if err != nil {
		if errors.Is(err, cmms.ErrAssetNotFound) {
			http.NotFound(w, r)
			return
		}
		h.logger.Error("get asset", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	if !ownedByCompany(asset.CompanyID, currentCompany(r)) {
		http.NotFound(w, r)
		return
	}
	// Load recent work orders for this asset
	assetID := asset.ID
	wos, _ := h.service.ListWorkOrders(r.Context(), cmms.ListWorkOrdersFilter{
		CompanyID: asset.CompanyID,
		AssetID:   &assetID,
		Limit:     10,
	})
	// Load PM schedules for this asset
	schedules, _ := h.service.ListPMSchedules(r.Context(), asset.ID)
	meterReadings, meterErr := h.service.GetMeterReadings(r.Context(), asset.ID, "", 20)
	if meterErr != nil {
		h.logger.Warn("list asset meter readings", slog.Any("error", meterErr), slog.Int64("asset_id", asset.ID))
	}
	h.render(w, r, "pages/cmms/asset_detail.html", "Asset "+asset.Code, map[string]any{
		"Asset":         asset,
		"WorkOrders":    wos,
		"Schedules":     schedules,
		"MeterReadings": meterReadings,
	})
}

// ============================================================================
// PM Schedules
// ============================================================================

func (h *Handler) listPMSchedules(w http.ResponseWriter, r *http.Request) {
	companyID := currentCompany(r)
	assets, err := h.service.ListAssets(r.Context(), cmms.ListAssetsFilter{CompanyID: companyID, Limit: 200})
	if err != nil {
		h.logger.Error("list assets for pm", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	// Build list across all assets
	var schedules []cmms.PreventiveMaintenanceSchedule
	for _, a := range assets {
		sched, err := h.service.ListPMSchedules(r.Context(), a.ID)
		if err != nil {
			continue
		}
		schedules = append(schedules, sched...)
	}

	h.render(w, r, "pages/cmms/pm_schedules.html", "PM Schedules", map[string]any{
		"Schedules": schedules,
		"Assets":    assets,
	})
}

func (h *Handler) newPMScheduleForm(w http.ResponseWriter, r *http.Request) {
	companyID := currentCompany(r)
	assets, _ := h.service.ListAssets(r.Context(), cmms.ListAssetsFilter{CompanyID: companyID, Limit: 200})
	h.render(w, r, "pages/cmms/pm_schedule_new.html", "New PM Schedule", map[string]any{
		"Assets":         assets,
		"FrequencyTypes": []string{"DAILY", "WEEKLY", "MONTHLY", "QUARTERLY", "SEMI_ANNUAL", "ANNUAL", "METER_BASED"},
		"MeterTypes":     []string{"HOURS", "CYCLES", "DISTANCE"},
	})
}

func (h *Handler) createPMSchedule(w http.ResponseWriter, r *http.Request) {
	if err := h.verifyCSRF(r); err != nil {
		shared.WriteError(w, err)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	actorID := currentUser(r)
	companyID := currentCompany(r)
	assetID := parseInt64(r.PostFormValue("asset_id"))
	if assetID <= 0 {
		h.redirectWithFlash(w, r, "/cmms/pm-schedules/new", "danger", "Asset is required")
		return
	}
	asset, assetErr := h.ownedAsset(r, assetID, companyID)
	if assetErr != nil {
		h.redirectWithFlash(w, r, "/cmms/pm-schedules/new", "danger", shared.UserSafeMessage(assetErr))
		return
	}

	active := strings.TrimSpace(r.PostFormValue("active"))
	req := cmms.CreatePMScheduleRequest{
		CompanyID:        companyID,
		AssetID:          asset.ID,
		Name:             strings.TrimSpace(r.PostFormValue("name")),
		Description:      strings.TrimSpace(r.PostFormValue("description")),
		FrequencyType:    strings.ToUpper(strings.TrimSpace(r.PostFormValue("frequency_type"))),
		FrequencyValue:   int(parseInt64(r.PostFormValue("frequency_value"))),
		MeterReadingType: strings.ToUpper(strings.TrimSpace(r.PostFormValue("meter_reading_type"))),
		Active:           active != "false",
		ActorID:          actorID,
	}

	sched, err := h.service.CreatePMSchedule(r.Context(), req)
	if err != nil {
		h.logger.Warn("create pm schedule", slog.Any("error", err))
		h.redirectWithFlash(w, r, "/cmms/pm-schedules/new", "danger", shared.UserSafeMessage(err))
		return
	}
	// The legacy request has no initial due-date field. Make a newly activated
	// calendar schedule eligible for its first execution immediately; meter-
	// based schedules remain governed by their first reading.
	if h.pool != nil && req.Active && req.FrequencyType != "METER_BASED" {
		if _, dueErr := h.pool.Exec(r.Context(), `UPDATE pm_schedules SET next_due_date=COALESCE(next_due_date,CURRENT_DATE), updated_at=NOW() WHERE id=$1 AND company_id=$2`, sched.ID, companyID); dueErr != nil {
			h.logger.Warn("initialize pm schedule due date", slog.Any("error", dueErr), slog.Int64("schedule_id", sched.ID))
		}
	}
	if err := h.recordAudit(r.Context(), actorID, "CREATE", "cmms_pm_schedule", strconv.FormatInt(sched.ID, 10), map[string]any{
		"company_id": companyID, "asset_id": sched.AssetID, "frequency_type": sched.FrequencyType,
	}); err != nil {
		h.logger.Warn("audit create pm schedule", slog.Any("error", err), slog.Int64("schedule_id", sched.ID))
	}
	h.redirectWithFlash(w, r, "/cmms/pm-schedules/"+strconv.FormatInt(sched.ID, 10), "success", "PM schedule created")
}

func (h *Handler) getPMSchedule(w http.ResponseWriter, r *http.Request) {
	id := parseInt64(chi.URLParam(r, "id"))
	sched, err := h.service.GetPMSchedule(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if !ownedByCompany(sched.CompanyID, currentCompany(r)) {
		http.NotFound(w, r)
		return
	}
	h.render(w, r, "pages/cmms/pm_schedule_detail.html", "PM Schedule", map[string]any{
		"Schedule": sched,
	})
}

// runDuePMSchedules executes due preventive-maintenance schedules for the
// active company and redirects back to the schedule list with a summary.
func (h *Handler) runDuePMSchedules(w http.ResponseWriter, r *http.Request) {
	if err := h.verifyCSRF(r); err != nil {
		shared.WriteError(w, err)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	companyID := currentCompany(r)
	actorID := currentUser(r)
	if companyID <= 0 || actorID <= 0 {
		h.redirectWithFlash(w, r, "/cmms/pm-schedules", "danger", "An active company and signed-in user are required")
		return
	}
	key := mutationKey(r, fmt.Sprintf("cmms:%d:pm-schedules:run-due:%s", companyID, time.Now().UTC().Format("2006-01-02")))
	duplicate, err := h.beginMutation(r.Context(), key, "cmms.pm.run_due")
	if err != nil {
		h.logger.Error("start pm schedule mutation", slog.Any("error", err), slog.Int64("company_id", companyID))
		h.redirectWithFlash(w, r, "/cmms/pm-schedules", "danger", shared.UserSafeMessage(err))
		return
	}
	if duplicate {
		h.redirectWithFlash(w, r, "/cmms/pm-schedules", "success", "Due PM schedules already processed")
		return
	}
	workOrders, err := h.service.GeneratePMWorkOrdersForCompany(r.Context(), companyID, actorID)
	if err != nil {
		h.rollbackMutation(r.Context(), key)
		h.logger.Warn("run due pm schedules", slog.Any("error", err), slog.Int64("company_id", companyID))
		h.redirectWithFlash(w, r, "/cmms/pm-schedules", "danger", shared.UserSafeMessage(err))
		return
	}
	companyOrders := len(workOrders)
	for _, wo := range workOrders {
		if auditErr := h.recordAudit(r.Context(), actorID, "GENERATE", "cmms_work_order", strconv.FormatInt(wo.ID, 10), map[string]any{
			"company_id": companyID, "pm_schedule_id": "generated", "category": wo.Category,
		}); auditErr != nil {
			h.logger.Warn("audit generated pm work order", slog.Any("error", auditErr), slog.Int64("work_order_id", wo.ID))
		}
	}
	h.redirectWithFlash(w, r, "/cmms/pm-schedules", "success", fmt.Sprintf("Generated %d due PM work order(s)", companyOrders))
}

// ============================================================================
// Spare Parts
// ============================================================================

func (h *Handler) listSpareParts(w http.ResponseWriter, r *http.Request) {
	companyID := currentCompany(r)
	parts, err := h.service.ListSpareParts(r.Context(), companyID)
	if err != nil {
		h.logger.Error("list spare parts", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	h.render(w, r, "pages/cmms/spare_parts.html", "Spare Parts", map[string]any{
		"Parts": parts,
	})
}

func (h *Handler) newSparePartForm(w http.ResponseWriter, r *http.Request) {
	h.render(w, r, "pages/cmms/spare_part_new.html", "New Spare Part", map[string]any{
		"UOMs": []string{"EA", "PCS", "KG", "M", "L", "BOX", "SET"},
	})
}

func (h *Handler) createSparePart(w http.ResponseWriter, r *http.Request) {
	if err := h.verifyCSRF(r); err != nil {
		shared.WriteError(w, err)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	actorID := currentUser(r)
	companyID := currentCompany(r)

	unitCost, unitCostErr := parseOptionalFloat64(r.PostFormValue("unit_cost"))
	minQuantity, minQuantityErr := parseOptionalFloat64(r.PostFormValue("min_quantity"))
	reorderPoint, reorderPointErr := parseOptionalFloat64(r.PostFormValue("reorder_point"))
	if unitCostErr != nil || minQuantityErr != nil || reorderPointErr != nil || unitCost < 0 || minQuantity < 0 || reorderPoint < 0 {
		h.redirectWithFlash(w, r, "/cmms/spare-parts/new", "danger", "Spare-part quantities and unit cost must be non-negative numbers")
		return
	}
	req := cmms.CreateSparePartRequest{
		CompanyID:     companyID,
		Code:          strings.TrimSpace(r.PostFormValue("code")),
		Name:          strings.TrimSpace(r.PostFormValue("name")),
		Description:   strings.TrimSpace(r.PostFormValue("description")),
		Category:      strings.TrimSpace(r.PostFormValue("category")),
		UnitOfMeasure: strings.TrimSpace(r.PostFormValue("unit_of_measure")),
		UnitCost:      unitCost,
		MinQuantity:   minQuantity,
		ReorderPoint:  reorderPoint,
		CriticalSpare: r.PostFormValue("critical_spare") == "true",
		ActorID:       actorID,
	}

	part, err := h.service.CreateSparePart(r.Context(), req)
	if err != nil {
		h.logger.Warn("create spare part", slog.Any("error", err))
		h.redirectWithFlash(w, r, "/cmms/spare-parts/new", "danger", shared.UserSafeMessage(err))
		return
	}
	if err := h.recordAudit(r.Context(), actorID, "CREATE", "cmms_spare_part", strconv.FormatInt(part.ID, 10), map[string]any{
		"company_id": companyID, "code": part.Code,
	}); err != nil {
		h.logger.Warn("audit create spare part", slog.Any("error", err), slog.Int64("spare_part_id", part.ID))
	}
	h.redirectWithFlash(w, r, "/cmms/spare-parts/"+strconv.FormatInt(part.ID, 10), "success", "Spare part created")
}

// recordMeterReading records a dated meter value against a company-owned asset.
func (h *Handler) recordMeterReading(w http.ResponseWriter, r *http.Request) {
	if err := h.verifyCSRF(r); err != nil {
		shared.WriteError(w, err)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	assetID := parseInt64(chi.URLParam(r, "id"))
	companyID := currentCompany(r)
	actorID := currentUser(r)
	asset, err := h.ownedAsset(r, assetID, companyID)
	if err != nil {
		h.redirectWithFlash(w, r, assetPath(assetID), "danger", shared.UserSafeMessage(err))
		return
	}
	readingDate := time.Now().UTC()
	if raw := strings.TrimSpace(r.PostFormValue("reading_date")); raw != "" {
		parsed, parseErr := time.Parse("2006-01-02", raw)
		if parseErr != nil {
			h.redirectWithFlash(w, r, assetPath(assetID), "danger", "Reading date must use YYYY-MM-DD")
			return
		}
		readingDate = parsed
	}
	readingType := strings.ToUpper(strings.TrimSpace(r.PostFormValue("reading_type")))
	value, valueErr := parseOptionalFloat64(r.PostFormValue("value"))
	if readingType == "" {
		h.redirectWithFlash(w, r, assetPath(assetID), "danger", "Reading type is required")
		return
	}
	if valueErr != nil || strings.TrimSpace(r.PostFormValue("value")) == "" {
		h.redirectWithFlash(w, r, assetPath(assetID), "danger", "Reading value must be a finite number")
		return
	}
	if value < 0 {
		h.redirectWithFlash(w, r, assetPath(assetID), "danger", "Reading value cannot be negative")
		return
	}
	key := mutationKey(r, fmt.Sprintf("cmms:%d:asset:%d:meter:%s:%s:%s", companyID, asset.ID, readingType, strconv.FormatFloat(value, 'g', -1, 64), readingDate.Format("2006-01-02")))
	duplicate, err := h.beginMutation(r.Context(), key, "cmms.asset.meter")
	if err != nil {
		h.logger.Error("start meter reading mutation", slog.Any("error", err), slog.Int64("asset_id", asset.ID))
		h.redirectWithFlash(w, r, assetPath(assetID), "danger", shared.UserSafeMessage(err))
		return
	}
	if duplicate {
		h.redirectWithFlash(w, r, assetPath(assetID), "success", "Meter reading already recorded")
		return
	}
	reading, err := h.service.RecordMeterReading(r.Context(), cmms.CreateMeterReadingRequest{
		AssetID: asset.ID, ReadingType: readingType, Value: value, ReadingDate: readingDate,
		EnteredBy: actorID, Notes: strings.TrimSpace(r.PostFormValue("notes")), ActorID: actorID,
	})
	if err != nil {
		h.rollbackMutation(r.Context(), key)
		h.logger.Warn("record meter reading", slog.Any("error", err), slog.Int64("asset_id", asset.ID))
		h.redirectWithFlash(w, r, assetPath(assetID), "danger", shared.UserSafeMessage(err))
		return
	}
	if err := h.recordAudit(r.Context(), actorID, "RECORD", "cmms_meter_reading", strconv.FormatInt(reading.ID, 10), map[string]any{
		"company_id": companyID, "asset_id": asset.ID, "reading_type": reading.ReadingType, "value": reading.Value,
	}); err != nil {
		h.logger.Warn("audit meter reading", slog.Any("error", err), slog.Int64("reading_id", reading.ID))
	}
	h.redirectWithFlash(w, r, assetPath(assetID), "success", "Meter reading recorded")
}

func (h *Handler) listMeterReadings(w http.ResponseWriter, r *http.Request) {
	assetID := parseInt64(chi.URLParam(r, "id"))
	asset, err := h.ownedAsset(r, assetID, currentCompany(r))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	readingType := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("reading_type")))
	readings, err := h.service.GetMeterReadings(r.Context(), asset.ID, readingType, 100)
	if err != nil {
		h.logger.Error("list meter readings", slog.Any("error", err), slog.Int64("asset_id", asset.ID))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	h.render(w, r, "pages/cmms/meter_readings.html", "Meter Readings", map[string]any{
		"Asset": asset, "Readings": readings, "ReadingType": readingType,
	})
}

func (h *Handler) addWorkOrderSparePart(w http.ResponseWriter, r *http.Request) {
	if err := h.verifyCSRF(r); err != nil {
		shared.WriteError(w, err)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	workOrderID := parseInt64(chi.URLParam(r, "id"))
	companyID := currentCompany(r)
	actorID := currentUser(r)
	wo, err := h.ownedWorkOrder(r, workOrderID, companyID)
	if err != nil {
		h.redirectWithFlash(w, r, workOrderPath(workOrderID), "danger", shared.UserSafeMessage(err))
		return
	}
	partID := parseInt64(r.PostFormValue("spare_part_id"))
	part, err := h.service.GetSparePart(r.Context(), partID)
	if err != nil || !ownedByCompany(part.CompanyID, companyID) {
		h.redirectWithFlash(w, r, workOrderPath(workOrderID), "danger", "Spare part was not found")
		return
	}
	quantity, quantityErr := parseOptionalFloat64(r.PostFormValue("quantity"))
	unitCost, unitCostErr := parseOptionalFloat64(r.PostFormValue("unit_cost"))
	if quantityErr != nil || unitCostErr != nil {
		h.redirectWithFlash(w, r, workOrderPath(workOrderID), "danger", "Quantity and unit cost must be finite numbers")
		return
	}
	if unitCost == 0 {
		unitCost = part.UnitCost
	}
	if quantity <= 0 || unitCost < 0 {
		h.redirectWithFlash(w, r, workOrderPath(workOrderID), "danger", "Quantity must be positive and unit cost cannot be negative")
		return
	}
	key := mutationKey(r, fmt.Sprintf("cmms:%d:work-order:%d:spare:%d:%s:%s", companyID, wo.ID, part.ID, strconv.FormatFloat(quantity, 'g', -1, 64), strconv.FormatFloat(unitCost, 'g', -1, 64)))
	duplicate, err := h.beginMutation(r.Context(), key, "cmms.work_order.spare_part")
	if err != nil {
		h.logger.Error("start add spare part mutation", slog.Any("error", err), slog.Int64("work_order_id", wo.ID))
		h.redirectWithFlash(w, r, workOrderPath(workOrderID), "danger", shared.UserSafeMessage(err))
		return
	}
	if duplicate {
		h.redirectWithFlash(w, r, workOrderPath(workOrderID), "success", "Spare part was already added")
		return
	}
	item, err := h.service.AddSparePartToWorkOrder(r.Context(), cmms.AddSparePartRequest{
		WorkOrderID: wo.ID, SparePartID: part.ID, Quantity: quantity, UnitCost: unitCost, ActorID: actorID,
	})
	if err != nil {
		h.rollbackMutation(r.Context(), key)
		h.logger.Warn("add spare part to work order", slog.Any("error", err), slog.Int64("work_order_id", wo.ID))
		h.redirectWithFlash(w, r, workOrderPath(workOrderID), "danger", shared.UserSafeMessage(err))
		return
	}
	if err := h.recordAudit(r.Context(), actorID, "ADD", "cmms_work_order_spare_part", strconv.FormatInt(item.ID, 10), map[string]any{
		"company_id": companyID, "work_order_id": wo.ID, "spare_part_id": part.ID, "quantity": quantity,
	}); err != nil {
		h.logger.Warn("audit add spare part", slog.Any("error", err), slog.Int64("item_id", item.ID))
	}
	h.redirectWithFlash(w, r, workOrderPath(workOrderID), "success", "Spare part added to work order")
}

func (h *Handler) issueWorkOrderSparePart(w http.ResponseWriter, r *http.Request) {
	if err := h.verifyCSRF(r); err != nil {
		shared.WriteError(w, err)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	itemID := parseInt64(chi.URLParam(r, "id"))
	companyID := currentCompany(r)
	actorID := currentUser(r)
	item, err := h.service.GetWorkOrderSparePart(r.Context(), itemID)
	if err != nil {
		h.redirectWithFlash(w, r, "/cmms/work-orders", "danger", shared.UserSafeMessage(err))
		return
	}
	wo, err := h.ownedWorkOrder(r, item.WorkOrderID, companyID)
	if err != nil {
		h.redirectWithFlash(w, r, "/cmms/work-orders", "danger", shared.UserSafeMessage(err))
		return
	}
	part, err := h.service.GetSparePart(r.Context(), item.SparePartID)
	if err != nil || !ownedByCompany(part.CompanyID, companyID) {
		h.redirectWithFlash(w, r, workOrderPath(wo.ID), "danger", "Spare part was not found")
		return
	}
	if item.IssuedAt != nil {
		// A retried browser submission is already converged. Do not rewrite the
		// issuer/timestamp or emit a second mutation for an issued line.
		h.redirectWithFlash(w, r, workOrderPath(wo.ID), "success", "Spare part issue was already processed")
		return
	}
	key := mutationKey(r, fmt.Sprintf("cmms:%d:work-order-spare-part:%d:issue", companyID, itemID))
	duplicate, err := h.beginMutation(r.Context(), key, "cmms.work_order.spare_part.issue")
	if err != nil {
		h.logger.Error("start issue spare part mutation", slog.Any("error", err), slog.Int64("item_id", item.ID))
		h.redirectWithFlash(w, r, workOrderPath(wo.ID), "danger", shared.UserSafeMessage(err))
		return
	}
	if duplicate {
		h.redirectWithFlash(w, r, workOrderPath(wo.ID), "success", "Spare part issue was already processed")
		return
	}
	issued, err := h.service.IssueSparePart(r.Context(), item.ID, actorID)
	if err != nil {
		h.rollbackMutation(r.Context(), key)
		h.logger.Warn("issue spare part", slog.Any("error", err), slog.Int64("item_id", item.ID))
		h.redirectWithFlash(w, r, workOrderPath(wo.ID), "danger", shared.UserSafeMessage(err))
		return
	}
	if err := h.recordAudit(r.Context(), actorID, "ISSUE", "cmms_work_order_spare_part", strconv.FormatInt(issued.ID, 10), map[string]any{
		"company_id": companyID, "work_order_id": wo.ID, "spare_part_id": part.ID, "quantity": issued.Quantity,
	}); err != nil {
		h.logger.Warn("audit issue spare part", slog.Any("error", err), slog.Int64("item_id", issued.ID))
	}
	h.redirectWithFlash(w, r, workOrderPath(wo.ID), "success", "Spare part issued")
}

// ============================================================================
// Helpers
// ============================================================================

func workOrderPath(id int64) string {
	return "/cmms/work-orders/" + strconv.FormatInt(id, 10)
}

func assetPath(id int64) string {
	return "/cmms/assets/" + strconv.FormatInt(id, 10)
}

func ownedByCompany(recordCompanyID, activeCompanyID int64) bool {
	return recordCompanyID > 0 && activeCompanyID > 0 && recordCompanyID == activeCompanyID
}

func (h *Handler) ownedWorkOrder(r *http.Request, id, companyID int64) (cmms.WorkOrder, error) {
	if id <= 0 || companyID <= 0 || h.service == nil {
		return cmms.WorkOrder{}, shared.ErrNotFound
	}
	workOrder, err := h.service.GetWorkOrder(r.Context(), id)
	if err != nil {
		return cmms.WorkOrder{}, err
	}
	if !ownedByCompany(workOrder.CompanyID, companyID) {
		return cmms.WorkOrder{}, shared.ErrNotFound
	}
	return workOrder, nil
}

func (h *Handler) ownedAsset(r *http.Request, id, companyID int64) (cmms.Asset, error) {
	if id <= 0 || companyID <= 0 || h.service == nil {
		return cmms.Asset{}, shared.ErrNotFound
	}
	asset, err := h.service.GetAsset(r.Context(), id)
	if err != nil {
		return cmms.Asset{}, err
	}
	if !ownedByCompany(asset.CompanyID, companyID) {
		return cmms.Asset{}, shared.ErrNotFound
	}
	return asset, nil
}

func (h *Handler) verifyCSRF(r *http.Request) error {
	if h.csrf == nil {
		return nil
	}
	sess := shared.SessionFromContext(r.Context())
	return h.csrf.VerifyToken(r.Context(), sess, r.PostFormValue(shared.CSRFFormField))
}

func mutationKey(r *http.Request, fallback string) string {
	if key := strings.TrimSpace(r.Header.Get("Idempotency-Key")); key != "" {
		return key
	}
	if key := strings.TrimSpace(r.PostFormValue("idempotency_key")); key != "" {
		return key
	}
	return fallback
}

func (h *Handler) beginMutation(ctx context.Context, key, module string) (bool, error) {
	if h.idempotency == nil {
		return false, nil
	}
	err := h.idempotency.CheckAndInsert(ctx, key, module)
	if errors.Is(err, shared.ErrIdempotencyConflict) {
		return true, nil
	}
	return false, err
}

func (h *Handler) rollbackMutation(ctx context.Context, key string) {
	if h.idempotency == nil || key == "" {
		return
	}
	if err := h.idempotency.Delete(ctx, key); err != nil && h.logger != nil {
		h.logger.Warn("rollback cmms idempotency key", slog.Any("error", err))
	}
}

func (h *Handler) recordAudit(ctx context.Context, actorID int64, action, entity, entityID string, meta map[string]any) error {
	if h.audit == nil {
		return nil
	}
	return h.audit.Record(ctx, shared.AuditLog{
		ActorID:  actorID,
		Action:   "cmms:" + strings.ToLower(action),
		Entity:   entity,
		EntityID: entityID,
		Meta:     meta,
	})
}

func validLifecycleTransition(current, next cmms.Status) bool {
	if current == next {
		return true
	}
	switch current {
	case cmms.WorkOrderStatusDraft:
		return next == cmms.WorkOrderStatusPlanned || next == cmms.WorkOrderStatusScheduled || next == cmms.WorkOrderStatusInProgress || next == cmms.WorkOrderStatusCancelled
	case cmms.Status("REQUESTED"):
		// PM generation in the legacy repository uses REQUESTED as its initial
		// state; permit the same execution transitions as a draft request.
		return next == cmms.WorkOrderStatusPlanned || next == cmms.WorkOrderStatusScheduled || next == cmms.WorkOrderStatusInProgress || next == cmms.WorkOrderStatusCancelled
	case cmms.WorkOrderStatusPlanned:
		return next == cmms.WorkOrderStatusScheduled || next == cmms.WorkOrderStatusInProgress || next == cmms.WorkOrderStatusCancelled
	case cmms.WorkOrderStatusScheduled:
		return next == cmms.WorkOrderStatusInProgress || next == cmms.WorkOrderStatusOnHold || next == cmms.WorkOrderStatusCancelled
	case cmms.WorkOrderStatusInProgress:
		return next == cmms.WorkOrderStatusOnHold || next == cmms.WorkOrderStatusCompleted || next == cmms.WorkOrderStatusCancelled
	case cmms.WorkOrderStatusOnHold:
		return next == cmms.WorkOrderStatusInProgress || next == cmms.WorkOrderStatusCancelled
	case cmms.WorkOrderStatusCompleted:
		return next == cmms.WorkOrderStatusClosed
	default:
		return false
	}
}

func (h *Handler) render(w http.ResponseWriter, r *http.Request, tpl, title string, data any) {
	sess := shared.SessionFromContext(r.Context())
	csrfToken := shared.CSRFTokenFromContext(r.Context())
	if h.csrf != nil {
		csrfToken, _ = h.csrf.EnsureToken(r.Context(), sess)
	}
	var flash *shared.FlashMessage
	if sess != nil {
		flash = sess.PopFlash()
	}
	viewData := view.TemplateData{
		Title:       title,
		CSRFToken:   csrfToken,
		Flash:       flash,
		CurrentPath: r.URL.Path,
		Data:        data,
	}
	if h.templates == nil {
		return
	}
	if err := h.templates.Render(w, tpl, viewData); err != nil && h.logger != nil {
		h.logger.Error("render cmms template", slog.Any("error", err))
	}
}

func (h *Handler) redirectWithFlash(w http.ResponseWriter, r *http.Request, location, kind, message string) {
	if sess := shared.SessionFromContext(r.Context()); sess != nil {
		sess.AddFlash(shared.FlashMessage{Kind: kind, Message: message})
	}
	http.Redirect(w, r, location, http.StatusSeeOther)
}

func currentUser(r *http.Request) int64 {
	sess := shared.SessionFromContext(r.Context())
	if sess == nil {
		return 0
	}
	id, _ := strconv.ParseInt(sess.User(), 10, 64)
	return id
}

func currentCompany(r *http.Request) int64 {
	sess := shared.SessionFromContext(r.Context())
	if sess == nil {
		return 0
	}
	id, _ := strconv.ParseInt(sess.Get("company_id"), 10, 64)
	return id
}

func parseInt64(value string) int64 {
	v, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil {
		return 0
	}
	return v
}

func parseFloat64(value string) float64 {
	v, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil {
		return 0
	}
	return v
}

func parseOptionalFloat64(value string) (float64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}
	v, err := strconv.ParseFloat(value, 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		if err == nil {
			err = errors.New("value must be finite")
		}
		return 0, err
	}
	return v, nil
}

// ============================================================================
// IoT and Predictive Maintenance
// ============================================================================

func (h *Handler) registerIoTSensor(w http.ResponseWriter, r *http.Request) {
	var in cmms.IoTSensor
	if err := shared.DecodeJSON(r, &in); err != nil {
		shared.JSONErrorFrom(w, http.StatusBadRequest, err)
		return
	}
	in.CompanyID = currentCompany(r)
	created, err := h.service.RegisterIoTSensor(r.Context(), in)
	if err != nil {
		shared.JSONErrorFrom(w, http.StatusInternalServerError, err)
		return
	}
	shared.JSONResponse(w, http.StatusCreated, created)
}

func (h *Handler) recordIoTReading(w http.ResponseWriter, r *http.Request) {
	var in cmms.IoTReading
	if err := shared.DecodeJSON(r, &in); err != nil {
		shared.JSONErrorFrom(w, http.StatusBadRequest, err)
		return
	}
	in.CompanyID = currentCompany(r)
	created, err := h.service.RecordIoTReading(r.Context(), in)
	if err != nil {
		shared.JSONErrorFrom(w, http.StatusInternalServerError, err)
		return
	}
	shared.JSONResponse(w, http.StatusCreated, created)
}

func (h *Handler) createPredictiveModel(w http.ResponseWriter, r *http.Request) {
	var in cmms.PredictiveModel
	if err := shared.DecodeJSON(r, &in); err != nil {
		shared.JSONErrorFrom(w, http.StatusBadRequest, err)
		return
	}
	in.CompanyID = currentCompany(r)
	created, err := h.service.CreatePredictiveModel(r.Context(), in)
	if err != nil {
		shared.JSONErrorFrom(w, http.StatusInternalServerError, err)
		return
	}
	shared.JSONResponse(w, http.StatusCreated, created)
}

func (h *Handler) evaluatePredictiveAlerts(w http.ResponseWriter, r *http.Request) {
	companyID := currentCompany(r)
	alerts, err := h.service.EvaluatePredictiveAlertsBatch(r.Context(), companyID)
	if err != nil {
		shared.JSONErrorFrom(w, http.StatusInternalServerError, err)
		return
	}
	shared.JSONResponse(w, http.StatusOK, alerts)
}
