package mrp

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/odyssey-erp/odyssey-erp/internal/inventory"
	"github.com/odyssey-erp/odyssey-erp/internal/rbac"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/view"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type Handler struct {
	service    *Service
	rbac       rbac.Middleware
	pool       *pgxpool.Pool
	stock      *inventory.Service
	executor   *ProductionExecutor
	planner    *PlanningRunService
	scheduler  *SchedulingService
	exceptions *ExceptionService
	analytics  *ManufacturingAnalytics
	compliance *ComplianceService
	templates  *view.Engine
	csrf       *shared.CSRFManager
}

func NewHandler(s *Service, m rbac.Middleware, pools ...*pgxpool.Pool) *Handler {
	var p *pgxpool.Pool
	if len(pools) > 0 {
		p = pools[0]
	}
	return &Handler{service: s, rbac: m, pool: p, planner: NewPlanningRunService(p), scheduler: NewSchedulingService(p), exceptions: NewExceptionService(p), analytics: NewManufacturingAnalytics(p), compliance: NewComplianceService(p)}
}
func (h *Handler) SetInventoryService(stock *inventory.Service) {
	h.stock = stock
	h.executor = NewProductionExecutor(h.pool, stock)
}
func (h *Handler) SetManufacturingAccounting(accounting ManufacturingAccounting) {
	if h.executor != nil {
		h.executor.SetAccounting(accounting)
	}
}
func (h *Handler) SetUI(templates *view.Engine, csrf *shared.CSRFManager) {
	h.templates, h.csrf = templates, csrf
}
func (h *Handler) MountRoutes(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAny("mrp.manage"))
		r.Get("/boms", h.bomRevisionPage)
		r.Post("/boms", h.createBOM)
		r.Get("/wip-locations", h.listWIPLocations)
		r.Get("/wip-locations/manage", h.wipLocationsPage)
		r.Post("/wip-locations", h.createWIPLocation)
		r.Post("/wip-locations/{id}/deactivate", h.deactivateWIPLocation)
		r.Get("/boms/revisions", h.listBOMRevisions)
		r.Post("/boms/{id}/revisions", h.createBOMRevision)
		r.Post("/boms/{id}/approve", h.approveBOM)
		r.Post("/work-centers", h.createWorkCenter)
		r.Post("/work-centers/{id}/shifts", h.createWorkCenterShift)
		r.Post("/work-centers/{id}/calendar-exceptions", h.createCalendarException)
		r.Post("/routings", h.createRouting)
		r.Post("/planning-policies", h.createPlanningPolicy)
		r.Post("/planning-runs", h.runPlanning)
		r.Post("/scheduling/run", h.runScheduling)
		r.Post("/operations/{id}/reschedule", h.rescheduleOperation)
		r.Post("/operations/{id}/split", h.splitOperation)
		r.Get("/scheduling", h.schedulingPage)
		r.Get("/exceptions", h.exceptionWorkbench)
		r.Post("/exceptions/{id}/actions", h.exceptionAction)
		r.Post("/inspections", h.createInspection)
		r.Post("/inspection-plans", h.createInspectionPlan)
		r.Post("/quality-holds", h.createQualityHold)
		r.Post("/quality-holds/{id}/release", h.releaseQualityHold)
		r.Get("/genealogy", h.genealogy)
		r.Post("/nonconformances", h.createNCR)
		r.Post("/capas", h.createCAPA)
		r.Post("/subcontract-operations", h.createSubcontractOperation)
		r.Post("/subcontract-operations/{id}/receive", h.receiveSubcontractOperation)
		r.Get("/quality", h.qualityPage)
		r.Get("/analytics", h.analyticsDashboard)
		r.Get("/analytics/export.csv", h.analyticsExport)
		r.Post("/compliance/signatures", h.signControlledRecord)
		r.Get("/compliance/audit-export.csv", h.complianceAuditExport)
		r.Post("/planning-recommendations/{id}/firm", h.firmPlanningRecommendation)
		r.Post("/work-orders", h.createWorkOrder)
		r.Post("/work-orders/{id}/release", h.release)
		r.Post("/work-orders/{id}/start", h.start)
		r.Post("/work-orders/{id}/complete", h.complete)
		r.Get("/work-orders/{id}/operations", h.listWorkOrderOperations)
		r.Get("/work-orders/{id}/wip", h.workOrderWIP)
		r.Get("/work-orders/{id}/dispatch", h.dispatchPage)
		r.Get("/dispatch", h.dispatch)
		r.Post("/work-orders/{id}/operations/{operationID}/report", h.reportOperation)
		r.Post("/work-orders/{id}/materials/issue", h.issueMaterial)
		r.Post("/work-orders/{id}/materials/return", h.returnMaterial)
		// Governance UI routes
		r.Get("/decisions/form", h.decisionSubmissionForm)
		r.Get("/decisions/audit", h.auditLogViewer)
		r.Get("/gates/{gateType}/status", h.gateStatusDisplay)
	})
}
func (h *Handler) listWorkOrderOperations(w http.ResponseWriter, r *http.Request) {
	_, c, ok := ids(r)
	if !ok {
		http.Error(w, "unauthorized", 401)
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "invalid work order id", 400)
		return
	}
	rows, err := h.pool.Query(r.Context(), `SELECT id,company_id,work_order_id,COALESCE(routing_operation_id,0),work_center_id,sequence,code,name,status,planned_setup_minutes::float8,planned_run_minutes::float8,actual_setup_minutes::float8,actual_run_minutes::float8,good_quantity::float8,scrap_quantity::float8,operator_id FROM mrp_work_order_operations WHERE company_id=$1 AND work_order_id=$2 ORDER BY sequence`, c, id)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	defer rows.Close()
	items := []WorkOrderOperation{}
	for rows.Next() {
		var item WorkOrderOperation
		if err := rows.Scan(&item.ID, &item.CompanyID, &item.WorkOrderID, &item.RoutingOperationID, &item.WorkCenterID, &item.Sequence, &item.Code, &item.Name, &item.Status, &item.PlannedSetupMinutes, &item.PlannedRunMinutes, &item.ActualSetupMinutes, &item.ActualRunMinutes, &item.GoodQuantity, &item.ScrapQuantity, &item.OperatorID); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		items = append(items, item)
	}
	out(w, 200, items)
}
func (h *Handler) workOrderWIP(w http.ResponseWriter, r *http.Request) {
	_, c, ok := ids(r)
	if !ok {
		http.Error(w, "unauthorized", 401)
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "invalid work order id", 400)
		return
	}
	rows, err := h.pool.Query(r.Context(), `SELECT product_id,COALESCE(SUM(CASE WHEN movement_type='ISSUE' THEN quantity ELSE -quantity END),0)::float8 FROM mrp_material_movements WHERE company_id=$1 AND work_order_id=$2 GROUP BY product_id ORDER BY product_id`, c, id)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	defer rows.Close()
	type balance struct {
		ProductID int64   `json:"product_id"`
		Quantity  float64 `json:"quantity"`
	}
	items := []balance{}
	for rows.Next() {
		var item balance
		if err := rows.Scan(&item.ProductID, &item.Quantity); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		items = append(items, item)
	}
	out(w, 200, items)
}
func (h *Handler) dispatch(w http.ResponseWriter, r *http.Request) {
	_, c, ok := ids(r)
	if !ok {
		http.Error(w, "unauthorized", 401)
		return
	}
	workCenterID, _ := strconv.ParseInt(r.URL.Query().Get("work_center_id"), 10, 64)
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	rows, err := h.pool.Query(r.Context(), `SELECT id,company_id,work_order_id,COALESCE(routing_operation_id,0),work_center_id,sequence,code,name,status,planned_setup_minutes::float8,planned_run_minutes::float8,actual_setup_minutes::float8,actual_run_minutes::float8,good_quantity::float8,scrap_quantity::float8,operator_id FROM mrp_work_order_operations WHERE company_id=$1 AND ($2=0 OR work_center_id=$2) AND ($3='' OR status=$3) ORDER BY work_center_id,sequence,id`, c, workCenterID, status)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	defer rows.Close()
	items := []WorkOrderOperation{}
	for rows.Next() {
		var item WorkOrderOperation
		if err := rows.Scan(&item.ID, &item.CompanyID, &item.WorkOrderID, &item.RoutingOperationID, &item.WorkCenterID, &item.Sequence, &item.Code, &item.Name, &item.Status, &item.PlannedSetupMinutes, &item.PlannedRunMinutes, &item.ActualSetupMinutes, &item.ActualRunMinutes, &item.GoodQuantity, &item.ScrapQuantity, &item.OperatorID); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		items = append(items, item)
	}
	out(w, 200, items)
}
func (h *Handler) bomRevisionPage(w http.ResponseWriter, r *http.Request) {
	if h.templates == nil {
		http.Error(w, "BOM revision UI is unavailable", http.StatusServiceUnavailable)
		return
	}
	_, companyID, ok := ids(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	productID, err := strconv.ParseInt(r.URL.Query().Get("product_id"), 10, 64)
	if err != nil || productID <= 0 {
		http.Error(w, "product_id is required", http.StatusBadRequest)
		return
	}
	revisions, err := h.service.ListBOMRevisions(r.Context(), companyID, productID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var token string
	if h.csrf != nil {
		token, _ = h.csrf.EnsureToken(r.Context(), shared.SessionFromContext(r.Context()))
	}
	if err := h.templates.Render(w, "pages/mrp/bom_revisions.html", view.TemplateData{Title: "BOM Revisions", CurrentPath: r.URL.Path, CSRFToken: token, Data: map[string]any{"ProductID": productID, "Revisions": revisions}}); err != nil {
		http.Error(w, "render BOM revisions", http.StatusInternalServerError)
	}
}
func (h *Handler) wipLocationsPage(w http.ResponseWriter, r *http.Request) {
	if h.templates == nil {
		http.Error(w, "WIP UI unavailable", 503)
		return
	}
	_, c, ok := ids(r)
	if !ok {
		http.Error(w, "unauthorized", 401)
		return
	}
	items, err := h.service.ListWIPLocations(r.Context(), c)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if err = h.templates.Render(w, "pages/mrp/wip_locations.html", view.TemplateData{Title: "WIP locations", CurrentPath: r.URL.Path, Data: map[string]any{"Locations": items}}); err != nil {
		http.Error(w, "render WIP locations", 500)
	}
}
func (h *Handler) dispatchPage(w http.ResponseWriter, r *http.Request) {
	if h.templates == nil {
		http.Error(w, "dispatch UI unavailable", 503)
		return
	}
	_, _, ok := ids(r)
	if !ok {
		http.Error(w, "unauthorized", 401)
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "invalid work order id", 400)
		return
	}
	if err = h.templates.Render(w, "pages/mrp/work_order_dispatch.html", view.TemplateData{Title: "Work order dispatch", CurrentPath: r.URL.Path, Data: map[string]any{"WorkOrderID": id}}); err != nil {
		http.Error(w, "render dispatch", 500)
	}
}
func (h *Handler) createPlanningPolicy(w http.ResponseWriter, r *http.Request) {
	u, c, ok := ids(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if h.planner == nil {
		http.Error(w, "mrp planning service is unavailable", http.StatusServiceUnavailable)
		return
	}
	var in PlanningPolicyInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	outv, err := h.planner.CreatePolicy(r.Context(), c, u, in)
	if errors.Is(err, ErrNotFound) {
		http.Error(w, "product or warehouse not found for company", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	out(w, http.StatusCreated, outv)
}
func (h *Handler) runPlanning(w http.ResponseWriter, r *http.Request) {
	u, c, ok := ids(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if h.planner == nil {
		http.Error(w, "mrp planning service is unavailable", http.StatusServiceUnavailable)
		return
	}
	var in struct {
		AsOfDate time.Time `json:"as_of_date"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if in.AsOfDate.IsZero() {
		in.AsOfDate = time.Now()
	}
	outv, err := h.planner.Run(r.Context(), c, u, in.AsOfDate)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	out(w, http.StatusCreated, outv)
}
func (h *Handler) runScheduling(w http.ResponseWriter, r *http.Request) {
	u, c, ok := ids(r)
	if !ok {
		http.Error(w, "unauthorized", 401)
		return
	}
	if h.scheduler == nil {
		http.Error(w, "scheduling unavailable", 503)
		return
	}
	var in struct {
		AsOf time.Time `json:"as_of"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, "invalid JSON", 400)
		return
	}
	issues, err := h.scheduler.Run(r.Context(), c, u, in.AsOf)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	out(w, 200, issues)
}
func (h *Handler) rescheduleOperation(w http.ResponseWriter, r *http.Request) {
	u, c, ok := ids(r)
	if !ok {
		http.Error(w, "unauthorized", 401)
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "invalid operation id", 400)
		return
	}
	var in struct {
		Start time.Time `json:"start"`
		End   time.Time `json:"end"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, "invalid JSON", 400)
		return
	}
	if err = h.scheduler.Reschedule(r.Context(), c, u, id, in.Start, in.End); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (h *Handler) splitOperation(w http.ResponseWriter, r *http.Request) {
	_, c, ok := ids(r)
	if !ok {
		http.Error(w, "unauthorized", 401)
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "invalid operation id", 400)
		return
	}
	var in struct {
		RunMinutes float64 `json:"run_minutes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.RunMinutes <= 0 {
		http.Error(w, "invalid split duration", 400)
		return
	}
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()
	var workOrderID int64
	var sequence int
	var run float64
	err = tx.QueryRow(r.Context(), `SELECT work_order_id,sequence,planned_run_minutes::float8 FROM mrp_work_order_operations WHERE id=$1 AND company_id=$2 FOR UPDATE`, id, c).Scan(&workOrderID, &sequence, &run)
	if err != nil || in.RunMinutes >= run {
		http.Error(w, "operation cannot be split", 400)
		return
	}
	if _, err = tx.Exec(r.Context(), `UPDATE mrp_work_order_operations SET sequence=sequence+1 WHERE work_order_id=$1 AND sequence>$2`, workOrderID, sequence); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	var newID int64
	err = tx.QueryRow(r.Context(), `INSERT INTO mrp_work_order_operations(company_id,work_order_id,routing_operation_id,work_center_id,sequence,code,name,status,planned_setup_minutes,planned_run_minutes) SELECT company_id,work_order_id,routing_operation_id,work_center_id,sequence+1,code||'-SPLIT',name||' (split)','PENDING',0,$3 FROM mrp_work_order_operations WHERE id=$1 RETURNING id`, id, c, in.RunMinutes).Scan(&newID)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if _, err = tx.Exec(r.Context(), `UPDATE mrp_work_order_operations SET planned_run_minutes=planned_run_minutes-$2 WHERE id=$1`, id, in.RunMinutes); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	out(w, 201, map[string]int64{"operation_id": newID})
}
func (h *Handler) firmPlanningRecommendation(w http.ResponseWriter, r *http.Request) {
	u, c, ok := ids(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if h.planner == nil {
		http.Error(w, "mrp planning service is unavailable", http.StatusServiceUnavailable)
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "invalid planning recommendation id", http.StatusBadRequest)
		return
	}
	outv, err := h.planner.FirmRecommendation(r.Context(), c, u, id)
	if errors.Is(err, ErrNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	out(w, http.StatusCreated, outv)
}
func (h *Handler) createWorkCenter(w http.ResponseWriter, r *http.Request) {
	u, c, ok := ids(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var in WorkCenter
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	in.CompanyID, in.CreatedBy = c, u
	outv, err := h.service.CreateWorkCenter(r.Context(), in)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	out(w, http.StatusCreated, outv)
}
func (h *Handler) createWorkCenterShift(w http.ResponseWriter, r *http.Request) {
	_, c, ok := ids(r)
	if !ok {
		http.Error(w, "unauthorized", 401)
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "invalid work center id", 400)
		return
	}
	var in struct {
		Weekday       int `json:"weekday"`
		Start, End    string
		CapacityHours float64 `json:"capacity_hours"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.Weekday < 0 || in.Weekday > 6 || in.CapacityHours <= 0 {
		http.Error(w, "invalid shift", 400)
		return
	}
	_, err = h.pool.Exec(r.Context(), `INSERT INTO mrp_work_center_shifts(company_id,work_center_id,weekday,start_time,end_time,capacity_hours) SELECT $1,id,$3,$4::time,$5::time,$6 FROM mrp_work_centers WHERE id=$2 AND company_id=$1`, c, id, in.Weekday, in.Start, in.End, in.CapacityHours)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	w.WriteHeader(http.StatusCreated)
}
func (h *Handler) createCalendarException(w http.ResponseWriter, r *http.Request) {
	_, c, ok := ids(r)
	if !ok {
		http.Error(w, "unauthorized", 401)
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "invalid work center id", 400)
		return
	}
	var in struct {
		Date          time.Time `json:"date"`
		Type          string    `json:"type"`
		CapacityHours float64   `json:"capacity_hours"`
		Note          string    `json:"note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.Date.IsZero() {
		http.Error(w, "invalid calendar exception", 400)
		return
	}
	_, err = h.pool.Exec(r.Context(), `INSERT INTO mrp_work_center_calendar_exceptions(company_id,work_center_id,exception_date,exception_type,capacity_hours,note) SELECT $1,id,$3,$4,$5,$6 FROM mrp_work_centers WHERE id=$2 AND company_id=$1 ON CONFLICT(work_center_id,exception_date) DO UPDATE SET exception_type=EXCLUDED.exception_type,capacity_hours=EXCLUDED.capacity_hours,note=EXCLUDED.note`, c, id, in.Date, in.Type, in.CapacityHours, in.Note)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	w.WriteHeader(http.StatusCreated)
}
func (h *Handler) schedulingPage(w http.ResponseWriter, r *http.Request) {
	if h.templates == nil {
		http.Error(w, "scheduling UI unavailable", 503)
		return
	}
	if _, _, ok := ids(r); !ok {
		http.Error(w, "unauthorized", 401)
		return
	}
	if err := h.templates.Render(w, "pages/mrp/scheduling_board.html", view.TemplateData{Title: "Scheduling board", CurrentPath: r.URL.Path}); err != nil {
		http.Error(w, "render scheduling board", 500)
	}
}
func (h *Handler) exceptionWorkbench(w http.ResponseWriter, r *http.Request) {
	_, c, ok := ids(r)
	if !ok {
		http.Error(w, "unauthorized", 401)
		return
	}
	items, err := h.exceptions.List(r.Context(), c, strings.TrimSpace(r.URL.Query().Get("status")), strings.TrimSpace(r.URL.Query().Get("severity")))
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if strings.Contains(r.Header.Get("Accept"), "application/json") {
		out(w, 200, items)
		return
	}
	if h.templates == nil {
		http.Error(w, "exception UI unavailable", 503)
		return
	}
	if err = h.templates.Render(w, "pages/mrp/exceptions.html", view.TemplateData{Title: "MRP exceptions", CurrentPath: r.URL.Path, Data: map[string]any{"Exceptions": items}}); err != nil {
		http.Error(w, "render exceptions", 500)
	}
}
func (h *Handler) exceptionAction(w http.ResponseWriter, r *http.Request) {
	u, c, ok := ids(r)
	if !ok {
		http.Error(w, "unauthorized", 401)
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "invalid exception id", 400)
		return
	}
	var in struct {
		Action, Comment string
		OwnerID         *int64 `json:"owner_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, "invalid JSON", 400)
		return
	}
	if err = h.exceptions.Act(r.Context(), c, u, id, strings.TrimSpace(in.Action), strings.TrimSpace(in.Comment), in.OwnerID); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (h *Handler) createInspection(w http.ResponseWriter, r *http.Request) {
	u, c, ok := ids(r)
	if !ok {
		http.Error(w, "unauthorized", 401)
		return
	}
	var in struct {
		PlanID      int64           `json:"plan_id"`
		WorkOrderID int64           `json:"work_order_id"`
		OperationID int64           `json:"operation_id"`
		Result      json.RawMessage `json:"result"`
		Status      string          `json:"status"`
		DefectCode  string          `json:"defect_code"`
		Disposition string          `json:"disposition"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.WorkOrderID <= 0 {
		http.Error(w, "invalid inspection", 400)
		return
	}
	if len(in.Result) == 0 {
		in.Result = []byte("{}")
	}
	var id int64
	err := h.pool.QueryRow(r.Context(), `INSERT INTO mrp_inspections(company_id,plan_id,work_order_id,operation_id,status,result,defect_code,disposition,inspector_id) VALUES($1,NULLIF($2,0),$3,NULLIF($4,0),$5,$6::jsonb,$7,$8,$9) RETURNING id`, c, in.PlanID, in.WorkOrderID, in.OperationID, in.Status, string(in.Result), in.DefectCode, in.Disposition, u).Scan(&id)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	out(w, 201, map[string]int64{"id": id})
}
func (h *Handler) createInspectionPlan(w http.ResponseWriter, r *http.Request) {
	u, c, ok := ids(r)
	if !ok {
		http.Error(w, "unauthorized", 401)
		return
	}
	var in struct {
		ProductID, RoutingOperationID int64
		Name                          string
		Required                      bool
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.ProductID <= 0 || strings.TrimSpace(in.Name) == "" {
		http.Error(w, "invalid inspection plan", 400)
		return
	}
	var id int64
	err := h.pool.QueryRow(r.Context(), `INSERT INTO mrp_inspection_plans(company_id,product_id,routing_operation_id,name,required,created_by) SELECT $1,p.id,NULLIF($3,0),$4,$5,$6 FROM products p WHERE p.id=$2 AND p.company_id=$1 RETURNING id`, c, in.ProductID, in.RoutingOperationID, in.Name, in.Required, u).Scan(&id)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	out(w, 201, map[string]int64{"id": id})
}
func (h *Handler) createNCR(w http.ResponseWriter, r *http.Request) {
	u, c, ok := ids(r)
	if !ok {
		http.Error(w, "unauthorized", 401)
		return
	}
	var in struct {
		InspectionID        int64 `json:"inspection_id"`
		Number, Description string
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || strings.TrimSpace(in.Number) == "" || strings.TrimSpace(in.Description) == "" {
		http.Error(w, "invalid non-conformance", 400)
		return
	}
	var id int64
	err := h.pool.QueryRow(r.Context(), `INSERT INTO mrp_nonconformances(company_id,inspection_id,number,description,owner_id) VALUES($1,NULLIF($2,0),$3,$4,$5) RETURNING id`, c, in.InspectionID, in.Number, in.Description, u).Scan(&id)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	out(w, 201, map[string]int64{"id": id})
}
func (h *Handler) createCAPA(w http.ResponseWriter, r *http.Request) {
	u, c, ok := ids(r)
	if !ok {
		http.Error(w, "unauthorized", 401)
		return
	}
	var in struct {
		NCRID   int64 `json:"ncr_id"`
		Action  string
		DueDate *time.Time `json:"due_date"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.NCRID <= 0 || strings.TrimSpace(in.Action) == "" {
		http.Error(w, "invalid CAPA", 400)
		return
	}
	var id int64
	err := h.pool.QueryRow(r.Context(), `INSERT INTO mrp_capas(company_id,ncr_id,action,owner_id,due_date) VALUES($1,$2,$3,$4,$5) RETURNING id`, c, in.NCRID, in.Action, u, in.DueDate).Scan(&id)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	out(w, 201, map[string]int64{"id": id})
}
func (h *Handler) createSubcontractOperation(w http.ResponseWriter, r *http.Request) {
	_, c, ok := ids(r)
	if !ok {
		http.Error(w, "unauthorized", 401)
		return
	}
	var in struct {
		OperationID, SupplierID int64
		Quantity, Cost          float64
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.OperationID <= 0 || in.SupplierID <= 0 || in.Quantity <= 0 {
		http.Error(w, "invalid subcontract operation", 400)
		return
	}
	var id int64
	err := h.pool.QueryRow(r.Context(), `INSERT INTO mrp_subcontract_operations(company_id,operation_id,supplier_id,sent_quantity,sent_cost) SELECT $1,op.id,$3,$4,$5 FROM mrp_work_order_operations op WHERE op.id=$2 AND op.company_id=$1 RETURNING id`, c, in.OperationID, in.SupplierID, in.Quantity, in.Cost).Scan(&id)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	out(w, 201, map[string]int64{"id": id})
}
func (h *Handler) receiveSubcontractOperation(w http.ResponseWriter, r *http.Request) {
	_, c, ok := ids(r)
	if !ok {
		http.Error(w, "unauthorized", 401)
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "invalid subcontract id", 400)
		return
	}
	var in struct {
		Quantity     float64 `json:"quantity"`
		InspectionID int64   `json:"inspection_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.Quantity <= 0 {
		http.Error(w, "invalid received quantity", 400)
		return
	}
	res, err := h.pool.Exec(r.Context(), `UPDATE mrp_subcontract_operations SET received_quantity=received_quantity+$1,received_at=NOW(),inspection_id=NULLIF($2,0),status='INSPECTING' WHERE id=$3 AND company_id=$4 AND status='SENT'`, in.Quantity, in.InspectionID, id, c)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if res.RowsAffected() == 0 {
		http.Error(w, "not found", 404)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (h *Handler) qualityPage(w http.ResponseWriter, r *http.Request) {
	if h.templates == nil {
		http.Error(w, "quality UI unavailable", 503)
		return
	}
	if _, _, ok := ids(r); !ok {
		http.Error(w, "unauthorized", 401)
		return
	}
	if err := h.templates.Render(w, "pages/mrp/quality.html", view.TemplateData{Title: "Production quality", CurrentPath: r.URL.Path}); err != nil {
		http.Error(w, "render quality", 500)
	}
}
func (h *Handler) analyticsDashboard(w http.ResponseWriter, r *http.Request) {
	_, c, ok := ids(r)
	if !ok {
		http.Error(w, "unauthorized", 401)
		return
	}
	wc, _ := strconv.ParseInt(r.URL.Query().Get("work_center_id"), 10, 64)
	product, _ := strconv.ParseInt(r.URL.Query().Get("product_id"), 10, 64)
	metrics, err := h.analytics.Metrics(r.Context(), c, wc, product, time.Time{}, time.Time{})
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if strings.Contains(r.Header.Get("Accept"), "application/json") {
		out(w, 200, metrics)
		return
	}
	if h.templates == nil {
		http.Error(w, "analytics UI unavailable", 503)
		return
	}
	if err = h.templates.Render(w, "pages/mrp/analytics.html", view.TemplateData{Title: "Manufacturing analytics", CurrentPath: r.URL.Path, Data: metrics}); err != nil {
		http.Error(w, "render analytics", 500)
	}
}
func (h *Handler) analyticsExport(w http.ResponseWriter, r *http.Request) {
	_, c, ok := ids(r)
	if !ok {
		http.Error(w, "unauthorized", 401)
		return
	}
	metrics, err := h.analytics.Metrics(r.Context(), c, 0, 0, time.Time{}, time.Time{})
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", "attachment; filename=manufacturing-analytics.csv")
	_, _ = w.Write([]byte(fmt.Sprintf("good_quantity,scrap_quantity,wip_value,on_time_operations,completed_operations\n%.4f,%.4f,%.4f,%d,%d\n", metrics.GoodQuantity, metrics.ScrapQuantity, metrics.WIPValue, metrics.OnTimeOperations, metrics.CompletedOperations)))
}
func (h *Handler) signControlledRecord(w http.ResponseWriter, r *http.Request) {
	u, c, ok := ids(r)
	if !ok {
		http.Error(w, "unauthorized", 401)
		return
	}
	var in SignatureInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, "invalid JSON", 400)
		return
	}
	if err := h.compliance.Sign(r.Context(), c, u, in); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	w.WriteHeader(http.StatusCreated)
}
func (h *Handler) complianceAuditExport(w http.ResponseWriter, r *http.Request) {
	_, c, ok := ids(r)
	if !ok {
		http.Error(w, "unauthorized", 401)
		return
	}
	rows, err := h.pool.Query(r.Context(), `SELECT record_type,record_id,event_type,actor_id,created_at,detail::text FROM mrp_audit_events WHERE company_id=$1 ORDER BY created_at`, c)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	defer rows.Close()
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", "attachment; filename=mrp-audit.csv")
	_, _ = w.Write([]byte("record_type,record_id,event_type,actor_id,created_at,detail\n"))
	for rows.Next() {
		var recordType, eventType, detail string
		var recordID, actorID int64
		var at time.Time
		if err := rows.Scan(&recordType, &recordID, &eventType, &actorID, &at, &detail); err != nil {
			return
		}
		_, _ = w.Write([]byte(fmt.Sprintf("%q,%d,%q,%d,%q,%q\n", recordType, recordID, eventType, actorID, at.Format(time.RFC3339), detail)))
	}
}
func (h *Handler) createQualityHold(w http.ResponseWriter, r *http.Request) {
	u, c, ok := ids(r)
	if !ok {
		http.Error(w, "unauthorized", 401)
		return
	}
	var in struct {
		WorkOrderID, OperationID, InspectionID int64
		Reason                                 string
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.WorkOrderID <= 0 || strings.TrimSpace(in.Reason) == "" {
		http.Error(w, "invalid quality hold", 400)
		return
	}
	var id int64
	err := h.pool.QueryRow(r.Context(), `INSERT INTO mrp_quality_holds(company_id,work_order_id,operation_id,inspection_id,reason,created_by) VALUES($1,$2,NULLIF($3,0),NULLIF($4,0),$5,$6) RETURNING id`, c, in.WorkOrderID, in.OperationID, in.InspectionID, in.Reason, u).Scan(&id)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	out(w, 201, map[string]int64{"id": id})
}
func (h *Handler) releaseQualityHold(w http.ResponseWriter, r *http.Request) {
	u, c, ok := ids(r)
	if !ok {
		http.Error(w, "unauthorized", 401)
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "invalid hold id", 400)
		return
	}
	res, err := h.pool.Exec(r.Context(), `UPDATE mrp_quality_holds SET status='RELEASED',released_by=$1,released_at=NOW() WHERE id=$2 AND company_id=$3 AND status='OPEN'`, u, id, c)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if res.RowsAffected() == 0 {
		http.Error(w, "not found", 404)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (h *Handler) genealogy(w http.ResponseWriter, r *http.Request) {
	_, c, ok := ids(r)
	if !ok {
		http.Error(w, "unauthorized", 401)
		return
	}
	lotID, _ := strconv.ParseInt(r.URL.Query().Get("lot_id"), 10, 64)
	serialID, _ := strconv.ParseInt(r.URL.Query().Get("serial_id"), 10, 64)
	rows, err := h.pool.Query(r.Context(), `SELECT id,work_order_id,operation_id,component_product_id,consumed_lot_id,consumed_serial_id,produced_lot_id,produced_serial_id,quantity::float8,created_at FROM mrp_genealogy WHERE company_id=$1 AND ($2=0 OR consumed_lot_id=$2 OR produced_lot_id=$2) AND ($3=0 OR consumed_serial_id=$3 OR produced_serial_id=$3) ORDER BY id`, c, lotID, serialID)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	defer rows.Close()
	var items []map[string]any
	for rows.Next() {
		var id, wo int64
		var op, prod, cl, cs, pl, ps *int64
		var qty float64
		var at time.Time
		if err := rows.Scan(&id, &wo, &op, &prod, &cl, &cs, &pl, &ps, &qty, &at); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		items = append(items, map[string]any{"id": id, "work_order_id": wo, "operation_id": op, "component_product_id": prod, "consumed_lot_id": cl, "consumed_serial_id": cs, "produced_lot_id": pl, "produced_serial_id": ps, "quantity": qty, "created_at": at})
	}
	out(w, 200, items)
}
func (h *Handler) createRouting(w http.ResponseWriter, r *http.Request) {
	u, c, ok := ids(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var in Routing
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	in.CompanyID, in.CreatedBy = c, u
	outv, err := h.service.CreateRouting(r.Context(), in)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	out(w, http.StatusCreated, outv)
}

func (h *Handler) createBOM(w http.ResponseWriter, r *http.Request) {
	u, c, ok := ids(r)
	if !ok {
		http.Error(w, "unauthorized", 401)
		return
	}
	var in BOM
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, "invalid JSON", 400)
		return
	}
	in.CompanyID = c
	in.CreatedBy = u
	outv, err := h.service.CreateBOM(r.Context(), in)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	out(w, 201, outv)
}
func (h *Handler) listWIPLocations(w http.ResponseWriter, r *http.Request) {
	_, c, ok := ids(r)
	if !ok {
		http.Error(w, "unauthorized", 401)
		return
	}
	items, err := h.service.ListWIPLocations(r.Context(), c)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	out(w, 200, items)
}
func (h *Handler) createWIPLocation(w http.ResponseWriter, r *http.Request) {
	u, c, ok := ids(r)
	if !ok {
		http.Error(w, "unauthorized", 401)
		return
	}
	var in WIPLocation
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, "invalid JSON", 400)
		return
	}
	in.CompanyID, in.CreatedBy = c, u
	item, err := h.service.CreateWIPLocation(r.Context(), in)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	out(w, 201, item)
}
func (h *Handler) deactivateWIPLocation(w http.ResponseWriter, r *http.Request) {
	_, c, ok := ids(r)
	if !ok {
		http.Error(w, "unauthorized", 401)
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "invalid WIP location id", 400)
		return
	}
	if err = h.service.DeactivateWIPLocation(r.Context(), c, id); err != nil {
		if errors.Is(err, ErrNotFound) {
			http.Error(w, "not found", 404)
		} else {
			http.Error(w, err.Error(), 400)
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (h *Handler) listBOMRevisions(w http.ResponseWriter, r *http.Request) {
	_, c, ok := ids(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	productID, err := strconv.ParseInt(r.URL.Query().Get("product_id"), 10, 64)
	if err != nil || productID <= 0 {
		http.Error(w, "product_id is required", http.StatusBadRequest)
		return
	}
	items, err := h.service.ListBOMRevisions(r.Context(), c, productID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	out(w, http.StatusOK, items)
}
func (h *Handler) createBOMRevision(w http.ResponseWriter, r *http.Request) {
	u, c, ok := ids(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "invalid BOM id", http.StatusBadRequest)
		return
	}
	var in BOMRevisionInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	bom, err := h.service.CreateBOMRevision(r.Context(), c, id, u, in)
	if errors.Is(err, ErrNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	out(w, http.StatusCreated, bom)
}
func (h *Handler) approveBOM(w http.ResponseWriter, r *http.Request) {
	u, c, ok := ids(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "invalid BOM id", http.StatusBadRequest)
		return
	}
	var in struct {
		ChangeReason string `json:"change_reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	bom, err := h.service.ApproveBOM(r.Context(), c, id, u, in.ChangeReason)
	if errors.Is(err, ErrNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	out(w, http.StatusOK, bom)
}
func (h *Handler) createWorkOrder(w http.ResponseWriter, r *http.Request) {
	u, c, ok := ids(r)
	if !ok {
		http.Error(w, "unauthorized", 401)
		return
	}
	var in WorkOrder
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, "invalid JSON", 400)
		return
	}
	in.CompanyID = c
	in.CreatedBy = u
	outv, err := h.service.CreateWorkOrder(r.Context(), in)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	out(w, 201, outv)
}
func ids(r *http.Request) (int64, int64, bool) {
	s := shared.SessionFromContext(r.Context())
	if s == nil {
		return 0, 0, false
	}
	u, e := strconv.ParseInt(s.User(), 10, 64)
	if e != nil || u < 1 {
		return 0, 0, false
	}
	c, e := strconv.ParseInt(s.Get("company_id"), 10, 64)
	return u, c, e == nil && c > 0
}
func out(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func (h *Handler) transition(w http.ResponseWriter, r *http.Request, fn func(int64, int64) (WorkOrder, error)) {
	_, c, ok := ids(r)
	if !ok {
		http.Error(w, "unauthorized", 401)
		return
	}
	id, e := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if e != nil {
		http.Error(w, "invalid work order id", 400)
		return
	}
	o, e := fn(c, id)
	if errors.Is(e, ErrNotFound) {
		http.Error(w, "not found", 404)
		return
	}
	if e != nil {
		http.Error(w, e.Error(), 400)
		return
	}
	out(w, 200, o)
}
func (h *Handler) release(w http.ResponseWriter, r *http.Request) {
	h.transition(w, r, func(c, id int64) (WorkOrder, error) { return h.service.Release(r.Context(), c, id) })
}
func (h *Handler) start(w http.ResponseWriter, r *http.Request) {
	h.transition(w, r, func(c, id int64) (WorkOrder, error) { return h.service.Start(r.Context(), c, id) })
}
func (h *Handler) complete(w http.ResponseWriter, r *http.Request) {
	u, c, ok := ids(r)
	if !ok {
		http.Error(w, "unauthorized", 401)
		return
	}
	id, e := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if e != nil {
		http.Error(w, "invalid work order id", 400)
		return
	}
	var in struct {
		Quantity         float64 `json:"quantity"`
		ProducedLotID    int64   `json:"produced_lot_id"`
		ProducedSerialID int64   `json:"produced_serial_id"`
	}
	if e = json.NewDecoder(r.Body).Decode(&in); e != nil {
		http.Error(w, "invalid JSON", 400)
		return
	}
	if h.executor == nil {
		http.Error(w, "mrp production inventory service is unavailable", http.StatusServiceUnavailable)
		return
	}
	o, e := h.executor.Complete(r.Context(), CompletionInput{CompanyID: c, ActorID: u, WorkOrderID: id, Quantity: in.Quantity, ProducedLotID: in.ProducedLotID, ProducedSerialID: in.ProducedSerialID, IdempotencyKey: strings.TrimSpace(r.Header.Get("Idempotency-Key"))})
	if errors.Is(e, ErrNotFound) {
		http.Error(w, "not found", 404)
		return
	}
	if errors.Is(e, ErrIdempotencyKeyRequired) {
		http.Error(w, e.Error(), http.StatusBadRequest)
		return
	}
	if e != nil {
		http.Error(w, e.Error(), 400)
		return
	}
	out(w, 200, o)
}

func (h *Handler) reportOperation(w http.ResponseWriter, r *http.Request) {
	u, c, ok := ids(r)
	if !ok {
		http.Error(w, "unauthorized", 401)
		return
	}
	workOrderID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || workOrderID <= 0 {
		http.Error(w, "invalid work order id", 400)
		return
	}
	opID, err := strconv.ParseInt(chi.URLParam(r, "operationID"), 10, 64)
	if err != nil || opID <= 0 {
		http.Error(w, "invalid operation id", 400)
		return
	}
	var in struct {
		SetupMinutes  float64 `json:"setup_minutes"`
		RunMinutes    float64 `json:"run_minutes"`
		GoodQuantity  float64 `json:"good_quantity"`
		ScrapQuantity float64 `json:"scrap_quantity"`
		Complete      bool    `json:"complete"`
	}
	if err = json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, "invalid JSON", 400)
		return
	}
	if h.executor == nil {
		http.Error(w, "mrp production service is unavailable", 503)
		return
	}
	outv, err := h.executor.ReportOperation(r.Context(), OperationReportInput{CompanyID: c, ActorID: u, WorkOrderID: workOrderID, OperationID: opID, SetupMinutes: in.SetupMinutes, RunMinutes: in.RunMinutes, GoodQuantity: in.GoodQuantity, ScrapQuantity: in.ScrapQuantity, Complete: in.Complete})
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	out(w, 200, outv)
}

func (h *Handler) issueMaterial(w http.ResponseWriter, r *http.Request)  { h.moveMaterial(w, r, false) }
func (h *Handler) returnMaterial(w http.ResponseWriter, r *http.Request) { h.moveMaterial(w, r, true) }
func (h *Handler) moveMaterial(w http.ResponseWriter, r *http.Request, returning bool) {
	u, c, ok := ids(r)
	if !ok {
		http.Error(w, "unauthorized", 401)
		return
	}
	workOrderID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || workOrderID <= 0 {
		http.Error(w, "invalid work order id", 400)
		return
	}
	var in struct {
		OperationID int64   `json:"operation_id"`
		ProductID   int64   `json:"product_id"`
		Quantity    float64 `json:"quantity"`
		LotID       int64   `json:"lot_id"`
		SerialID    int64   `json:"serial_id"`
	}
	if err = json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, "invalid JSON", 400)
		return
	}
	if h.executor == nil {
		http.Error(w, "mrp production service is unavailable", 503)
		return
	}
	err = h.executor.MoveMaterial(r.Context(), MaterialMovementInput{CompanyID: c, ActorID: u, WorkOrderID: workOrderID, OperationID: in.OperationID, ProductID: in.ProductID, Quantity: in.Quantity, LotID: in.LotID, SerialID: in.SerialID, Return: returning, IdempotencyKey: strings.TrimSpace(r.Header.Get("Idempotency-Key"))})
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// decisionSubmissionForm serves the governance decision form template
func (h *Handler) decisionSubmissionForm(w http.ResponseWriter, r *http.Request) {
	u, c, ok := ids(r)
	if !ok {
		http.Error(w, "unauthorized", 401)
		return
	}

	// Render decision submission template
	data := map[string]interface{}{
		"userID":    u,
		"companyID": c,
		"recordTypes": []string{"BOM", "WorkOrder", "Operation", "QualityHold", "NCR", "CAPA"},
		"actions":    []string{"Approve", "Release", "Complete", "Reject", "Revise"},
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.renderTemplate(w, "governance/decision_submission.html", data); err != nil {
		http.Error(w, err.Error(), 500)
	}
}

// auditLogViewer serves the governance audit log viewer template
func (h *Handler) auditLogViewer(w http.ResponseWriter, r *http.Request) {
	u, c, ok := ids(r)
	if !ok {
		http.Error(w, "unauthorized", 401)
		return
	}

	// Parse pagination params
	page := 1
	if p := r.URL.Query().Get("page"); p != "" {
		if parsed, err := strconv.Atoi(p); err == nil && parsed > 0 {
			page = parsed
		}
	}

	// Fetch audit log from repository (if available)
	var auditEvents []map[string]interface{}
	if h.pool != nil {
		// Query audit events (mock for now, would use repository)
		auditEvents = []map[string]interface{}{
			{
				"id":          1,
				"recordType":  "BOM",
				"recordID":    1,
				"action":      "Approve",
				"actorID":     u,
				"actorRole":   "QUALITY_LEAD",
				"timestamp":   "2026-08-03T11:00:00Z",
				"status":      "APPROVED",
				"reason":      "BOM structure verified",
			},
		}
	}

	// Render audit log template
	data := map[string]interface{}{
		"userID":       u,
		"companyID":    c,
		"auditEvents":  auditEvents,
		"currentPage":  page,
		"pageSize":     50,
		"totalRecords": len(auditEvents),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.renderTemplate(w, "governance/audit_log_viewer.html", data); err != nil {
		http.Error(w, err.Error(), 500)
	}
}

// gateStatusDisplay serves the certification gate status template
func (h *Handler) gateStatusDisplay(w http.ResponseWriter, r *http.Request) {
	u, c, ok := ids(r)
	if !ok {
		http.Error(w, "unauthorized", 401)
		return
	}

	gateType := chi.URLParam(r, "gateType")

	// Map gate types to required actors
	gateRequirements := map[string][]string{
		"BOM":       {"QUALITY_LEAD", "ENGINEERING"},
		"WorkOrder": {"PLANNER", "PRODUCTION_MANAGER"},
		"Hold":      {"QUALITY_MANAGER"},
		"NCR":       {"QUALITY_LEAD", "ENGINEERING"},
		"CAPA":      {"QUALITY_MANAGER", "PROCESS_OWNER"},
	}

	requiredActors, ok := gateRequirements[gateType]
	if !ok {
		http.Error(w, "unknown gate type", 400)
		return
	}

	// Mock signatures (would come from repository)
	signatures := []map[string]interface{}{}
	for i, actor := range requiredActors {
		signatures = append(signatures, map[string]interface{}{
			"actorID":   100 + int64(i),
			"actorRole": actor,
			"decision":  "PENDING",
			"timestamp": nil,
		})
	}

	// Render gate status template
	data := map[string]interface{}{
		"userID":         u,
		"companyID":      c,
		"gateType":       gateType,
		"requiredActors": requiredActors,
		"signatures":     signatures,
		"status":         "PENDING",
		"actorCount":     len(requiredActors),
		"signedCount":    0,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.renderTemplate(w, "governance/certification_gate_display.html", data); err != nil {
		http.Error(w, err.Error(), 500)
	}
}

// renderTemplate renders an HTML template with data
func (h *Handler) renderTemplate(w http.ResponseWriter, templateName string, data interface{}) error {
	// Mock template rendering (would use actual template engine)
	// This is a placeholder that writes JSON response
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if _, err := w.Write([]byte("<html><body>Governance UI - " + templateName + "</body></html>")); err != nil {
		return err
	}
	return nil
}
