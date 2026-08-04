package cmmshttp

import (
	"errors"
	"log/slog"
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
	logger    *slog.Logger
	service   *cmms.Service
	templates *view.Engine
	csrf      *shared.CSRFManager
	rbac      rbac.Middleware
	pool      *pgxpool.Pool
}

// NewHandler constructs a Handler value.
func NewHandler(logger *slog.Logger, service *cmms.Service, templates *view.Engine, csrf *shared.CSRFManager, rbac rbac.Middleware, pool *pgxpool.Pool) *Handler {
	return &Handler{
		logger:    logger,
		service:   service,
		templates: templates,
		csrf:      csrf,
		rbac:      rbac,
		pool:      pool,
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
	})

	// Assets
	r.Route("/assets", func(r chi.Router) {
		r.Use(h.rbac.RequireAny(shared.PermCMMSAssetView))
		r.Get("/", h.listAssets)
		r.Get("/new", h.newAssetForm)
		r.With(h.rbac.RequireAny(shared.PermCMMSAssetManage)).Post("/", h.createAsset)
		r.Get("/{id}", h.getAsset)
	})

	// Preventive Maintenance Schedules
	r.Route("/pm-schedules", func(r chi.Router) {
		r.Use(h.rbac.RequireAny(shared.PermCMMSPlanView))
		r.Get("/", h.listPMSchedules)
		r.Get("/new", h.newPMScheduleForm)
		r.With(h.rbac.RequireAny(shared.PermCMMSPlanManage)).Post("/", h.createPMSchedule)
		r.Get("/{id}", h.getPMSchedule)
	})

	// Spare Parts
	r.Route("/spare-parts", func(r chi.Router) {
		r.Use(h.rbac.RequireAny(shared.PermCMMSAssetView))
		r.Get("/", h.listSpareParts)
		r.Get("/new", h.newSparePartForm)
		r.With(h.rbac.RequireAny(shared.PermCMMSAssetManage)).Post("/", h.createSparePart)
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
		"Assets":    assets,
		"Locations": locations,
		"Categories": []string{"CORRECTIVE", "PREVENTIVE", "INSPECTION", "EMERGENCY", "CALIBRATION"},
		"Priorities": []cmms.Priority{cmms.PriorityLow, cmms.PriorityMedium, cmms.PriorityHigh, cmms.PriorityCritical},
	})
}

func (h *Handler) createWorkOrder(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	actorID := currentUser(r)
	companyID := currentCompany(r)

	req := cmms.CreateWorkOrderRequest{
		CompanyID:      companyID,
		Title:          strings.TrimSpace(r.PostFormValue("title")),
		Description:    strings.TrimSpace(r.PostFormValue("description")),
		Priority:       cmms.NormalisePriority(r.PostFormValue("priority")),
		Category:       strings.ToUpper(strings.TrimSpace(r.PostFormValue("category"))),
		RequesterID:    actorID,
		EstimatedHours: parseFloat64(r.PostFormValue("estimated_hours")),
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
	h.render(w, r, "pages/cmms/work_order_detail.html", "Work Order "+wo.Number, map[string]any{
		"WorkOrder": wo,
		"Statuses": []cmms.Status{
			cmms.WorkOrderStatusPlanned, cmms.WorkOrderStatusInProgress,
			cmms.WorkOrderStatusOnHold, cmms.WorkOrderStatusCompleted, cmms.WorkOrderStatusClosed,
			cmms.WorkOrderStatusCancelled,
		},
	})
}

func (h *Handler) updateWorkOrderStatus(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	id := parseInt64(chi.URLParam(r, "id"))
	status := cmms.NormaliseStatus(r.PostFormValue("status"))
	actorID := currentUser(r)

	if _, err := h.service.UpdateWorkOrderStatus(r.Context(), id, status, actorID); err != nil {
		h.logger.Warn("update work order status", slog.Any("error", err))
		h.redirectWithFlash(w, r, "/cmms/work-orders/"+strconv.FormatInt(id, 10), "danger", shared.UserSafeMessage(err))
		return
	}
	h.redirectWithFlash(w, r, "/cmms/work-orders/"+strconv.FormatInt(id, 10), "success", "Status updated")
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
		"Assets": assets,
		"Filter": filter,
		"AssetTypes": []string{"EQUIPMENT", "FACILITY", "VEHICLE", "TOOL", "INFRASTRUCTURE"},
		"Statuses":   []string{"ACTIVE", "INACTIVE", "DECOMMISSIONED", "SCRAPPED"},
	})
}

func (h *Handler) newAssetForm(w http.ResponseWriter, r *http.Request) {
	companyID := currentCompany(r)
	locations, _ := h.service.ListLocations(r.Context(), companyID)
	assets, _ := h.service.ListAssets(r.Context(), cmms.ListAssetsFilter{CompanyID: companyID, Limit: 200})
	h.render(w, r, "pages/cmms/asset_new.html", "New Asset", map[string]any{
		"Locations":   locations,
		"Assets":      assets,
		"AssetTypes":  []string{"EQUIPMENT", "FACILITY", "VEHICLE", "TOOL", "INFRASTRUCTURE"},
		"Criticalities": []string{"A", "B", "C", "D"},
	})
}

func (h *Handler) createAsset(w http.ResponseWriter, r *http.Request) {
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
	// Load recent work orders for this asset
	assetID := asset.ID
	wos, _ := h.service.ListWorkOrders(r.Context(), cmms.ListWorkOrdersFilter{
		CompanyID: asset.CompanyID,
		AssetID:   &assetID,
		Limit:     10,
	})
	// Load PM schedules for this asset
	schedules, _ := h.service.ListPMSchedules(r.Context(), asset.ID)
	h.render(w, r, "pages/cmms/asset_detail.html", "Asset "+asset.Code, map[string]any{
		"Asset":      asset,
		"WorkOrders": wos,
		"Schedules":  schedules,
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
		"Assets": assets,
		"FrequencyTypes": []string{"DAILY", "WEEKLY", "MONTHLY", "QUARTERLY", "SEMI_ANNUAL", "ANNUAL", "METER_BASED"},
		"MeterTypes":     []string{"HOURS", "CYCLES", "DISTANCE"},
	})
}

func (h *Handler) createPMSchedule(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	actorID := currentUser(r)
	companyID := currentCompany(r)

	req := cmms.CreatePMScheduleRequest{
		CompanyID:        companyID,
		AssetID:          parseInt64(r.PostFormValue("asset_id")),
		Name:             strings.TrimSpace(r.PostFormValue("name")),
		Description:      strings.TrimSpace(r.PostFormValue("description")),
		FrequencyType:    strings.ToUpper(strings.TrimSpace(r.PostFormValue("frequency_type"))),
		FrequencyValue:   int(parseInt64(r.PostFormValue("frequency_value"))),
		MeterReadingType: strings.ToUpper(strings.TrimSpace(r.PostFormValue("meter_reading_type"))),
		Active:           r.PostFormValue("active") == "true",
		ActorID:          actorID,
	}

	sched, err := h.service.CreatePMSchedule(r.Context(), req)
	if err != nil {
		h.logger.Warn("create pm schedule", slog.Any("error", err))
		h.redirectWithFlash(w, r, "/cmms/pm-schedules/new", "danger", shared.UserSafeMessage(err))
		return
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
	h.render(w, r, "pages/cmms/pm_schedule_detail.html", "PM Schedule", map[string]any{
		"Schedule": sched,
	})
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
	if err := r.ParseForm(); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	actorID := currentUser(r)
	companyID := currentCompany(r)

	req := cmms.CreateSparePartRequest{
		CompanyID:     companyID,
		Code:          strings.TrimSpace(r.PostFormValue("code")),
		Name:          strings.TrimSpace(r.PostFormValue("name")),
		Description:   strings.TrimSpace(r.PostFormValue("description")),
		Category:      strings.TrimSpace(r.PostFormValue("category")),
		UnitOfMeasure: strings.TrimSpace(r.PostFormValue("unit_of_measure")),
		UnitCost:      parseFloat64(r.PostFormValue("unit_cost")),
		MinQuantity:   parseFloat64(r.PostFormValue("min_quantity")),
		ReorderPoint:  parseFloat64(r.PostFormValue("reorder_point")),
		CriticalSpare: r.PostFormValue("critical_spare") == "true",
		ActorID:       actorID,
	}

	part, err := h.service.CreateSparePart(r.Context(), req)
	if err != nil {
		h.logger.Warn("create spare part", slog.Any("error", err))
		h.redirectWithFlash(w, r, "/cmms/spare-parts/new", "danger", shared.UserSafeMessage(err))
		return
	}
	h.redirectWithFlash(w, r, "/cmms/spare-parts/"+strconv.FormatInt(part.ID, 10), "success", "Spare part created")
}

// ============================================================================
// Helpers
// ============================================================================

func (h *Handler) render(w http.ResponseWriter, r *http.Request, tpl, title string, data any) {
	sess := shared.SessionFromContext(r.Context())
	csrfToken, _ := h.csrf.EnsureToken(r.Context(), sess)
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
