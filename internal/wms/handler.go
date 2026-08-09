package wms

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/odyssey-erp/odyssey-erp/internal/inventory"
	"github.com/odyssey-erp/odyssey-erp/internal/rbac"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
)

type Handler struct {
	service *Service
	rbac    rbac.Middleware
	pool    *pgxpool.Pool
	stock   *inventory.Service
}

func NewHandler(service *Service, middleware rbac.Middleware, pools ...*pgxpool.Pool) *Handler {
	var pool *pgxpool.Pool
	if len(pools) > 0 {
		pool = pools[0]
	}
	return &Handler{service: service, rbac: middleware, pool: pool}
}

func (h *Handler) SetInventoryService(stock *inventory.Service) { h.stock = stock }

func (h *Handler) MountRoutes(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAny("wms.manage"))
		r.Post("/waves", h.createWave)
		r.Post("/waves/{id}/release", h.releaseWave)
		r.Post("/waves/{id}/complete", h.completeWave)
		r.Post("/waves/{id}/cancel", h.cancelWave)
		r.Post("/bins", h.createBin)
		r.Post("/barcodes", h.createBarcode)
		r.Post("/pick-tasks", h.createPickTask)
		r.Post("/pick-tasks/{id}/scan", h.scan)
		r.Post("/pick-tasks/{id}/pack", h.pack)
		r.Post("/pick-tasks/{id}/ship", h.ship)

		// Advanced WMS
		r.Post("/putaway", h.createPutAwayTask)
		r.Post("/crossdocking", h.createCrossDockingPlan)
		r.Post("/mhe", h.createMHEEquipment)
	})
}

func (h *Handler) transitionWave(w http.ResponseWriter, r *http.Request, from, to string) {
	_, cid, ok := sessionIDs(r)
	if !ok {
		shared.WriteHTTPError(w, http.StatusForbidden, "")
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid wave id", http.StatusBadRequest)
		return
	}
	var response struct {
		ID, WarehouseID int64
		Number, Status  string
	}
	query := `UPDATE wms_pick_waves SET status=$1,updated_at=NOW() WHERE id=$2 AND company_id=$3 AND status=$4 RETURNING id,warehouse_id,number,status`
	args := []any{to, id, cid, from}
	if to == "COMPLETED" {
		query = `UPDATE wms_pick_waves SET status=$1,updated_at=NOW() WHERE id=$2 AND company_id=$3 AND status=$4 AND NOT EXISTS (SELECT 1 FROM wms_pick_tasks WHERE wave_id=$2 AND company_id=$3 AND status NOT IN ('PACKED','SHIPPED','CANCELLED')) RETURNING id,warehouse_id,number,status`
	}
	err = h.pool.QueryRow(r.Context(), query, args...).Scan(&response.ID, &response.WarehouseID, &response.Number, &response.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		http.Error(w, http.StatusText(http.StatusConflict), http.StatusConflict)
		return
	}
	if err != nil {
		shared.WriteErrorStatus(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *Handler) releaseWave(w http.ResponseWriter, r *http.Request) {
	h.transitionWave(w, r, "DRAFT", "RELEASED")
}
func (h *Handler) completeWave(w http.ResponseWriter, r *http.Request) {
	h.transitionWave(w, r, "RELEASED", "COMPLETED")
}
func (h *Handler) cancelWave(w http.ResponseWriter, r *http.Request) {
	h.transitionWave(w, r, "DRAFT", "CANCELLED")
}

func (h *Handler) createWave(w http.ResponseWriter, r *http.Request) {
	if h.pool == nil {
		http.Error(w, "WMS database is unavailable", 503)
		return
	}
	uid, cid, ok := sessionIDs(r)
	if !ok {
		shared.WriteHTTPError(w, 403, "")
		return
	}
	var in struct {
		WarehouseID int64  `json:"warehouse_id"`
		Number      string `json:"number"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	if in.WarehouseID == 0 || in.Number == "" {
		http.Error(w, "warehouse_id and number are required", 400)
		return
	}
	var response struct {
		ID, WarehouseID, CreatedBy int64
		Number, Status             string
	}
	err := h.pool.QueryRow(r.Context(), `INSERT INTO wms_pick_waves(company_id,warehouse_id,number,created_by) SELECT $1,id,$3,$4 FROM warehouses WHERE id=$2 AND company_id=$1 RETURNING id,warehouse_id,number,status,created_by`, cid, in.WarehouseID, in.Number, uid).Scan(&response.ID, &response.WarehouseID, &response.Number, &response.Status, &response.CreatedBy)
	if err != nil {
		shared.WriteErrorStatus(w, 400, err)
		return
	}
	writeJSON(w, 201, response)
}

func sessionIDs(r *http.Request) (int64, int64, bool) {
	sess := shared.SessionFromContext(r.Context())
	if sess == nil {
		return 0, 0, false
	}
	uid, err := strconv.ParseInt(sess.User(), 10, 64)
	if err != nil || uid <= 0 {
		return 0, 0, false
	}
	cid, err := strconv.ParseInt(sess.Get("company_id"), 10, 64)
	if err != nil || cid <= 0 {
		return 0, 0, false
	}
	return uid, cid, true
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func (h *Handler) createBin(w http.ResponseWriter, r *http.Request) {
	_, cid, ok := sessionIDs(r)
	if !ok {
		shared.WriteHTTPError(w, http.StatusForbidden, "")
		return
	}
	var input struct {
		WarehouseID int64 `json:"warehouse_id"`
		Code, Name  string
		Capacity    *float64 `json:"capacity"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	bin, err := h.service.CreateBin(r.Context(), Bin{CompanyID: cid, WarehouseID: input.WarehouseID, Code: input.Code, Name: input.Name, Capacity: input.Capacity, Active: true})
	if err != nil {
		shared.WriteErrorStatus(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusCreated, bin)
}

func (h *Handler) scan(w http.ResponseWriter, r *http.Request) {
	uid, cid, ok := sessionIDs(r)
	if !ok {
		shared.WriteHTTPError(w, http.StatusForbidden, "")
		return
	}
	taskID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid task id", http.StatusBadRequest)
		return
	}
	var input struct {
		Barcode        string  `json:"barcode"`
		Quantity       float64 `json:"quantity"`
		IdempotencyKey string  `json:"idempotency_key"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	if input.IdempotencyKey == "" {
		input.IdempotencyKey = r.Header.Get("Idempotency-Key")
	}
	result, err := h.service.Scan(r.Context(), cid, taskID, uid, input.Barcode, input.IdempotencyKey, input.Quantity)
	if errors.Is(err, ErrNotFound) {
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		return
	}
	if err != nil {
		shared.WriteErrorStatus(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) createBarcode(w http.ResponseWriter, r *http.Request) {
	_, cid, ok := sessionIDs(r)
	if !ok {
		shared.WriteHTTPError(w, http.StatusForbidden, "")
		return
	}
	var input struct {
		Barcode          string `json:"barcode"`
		ProductID, BinID int64
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	if err := h.service.RegisterBarcode(r.Context(), cid, input.Barcode, input.ProductID, input.BinID); err != nil {
		shared.WriteErrorStatus(w, http.StatusBadRequest, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) createPickTask(w http.ResponseWriter, r *http.Request) {
	_, cid, ok := sessionIDs(r)
	if !ok {
		shared.WriteHTTPError(w, http.StatusForbidden, "")
		return
	}
	var input struct {
		WaveID, DeliveryOrderID, ProductID int64
		RequestedQty                       float64 `json:"requested_qty"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	task, err := h.service.CreatePickTask(r.Context(), PickTask{CompanyID: cid, WaveID: input.WaveID, ProductID: input.ProductID, RequestedQty: input.RequestedQty})
	if err != nil {
		shared.WriteErrorStatus(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusCreated, task)
}

func (h *Handler) transition(w http.ResponseWriter, r *http.Request, status string) {
	_, cid, ok := sessionIDs(r)
	if !ok {
		shared.WriteHTTPError(w, http.StatusForbidden, "")
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid task id", http.StatusBadRequest)
		return
	}
	task, err := h.service.Transition(r.Context(), cid, id, status)
	if errors.Is(err, ErrNotFound) {
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		return
	}
	if err != nil {
		shared.WriteErrorStatus(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, task)
}

func (h *Handler) pack(w http.ResponseWriter, r *http.Request) { h.transition(w, r, "PACKED") }
func (h *Handler) ship(w http.ResponseWriter, r *http.Request) {
	uid, cid, ok := sessionIDs(r)
	if !ok {
		shared.WriteHTTPError(w, http.StatusForbidden, "")
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid task id", http.StatusBadRequest)
		return
	}
	if h.stock != nil && h.pool != nil {
		var warehouse, product int64
		var qty float64
		err = h.pool.QueryRow(r.Context(), `SELECT b.warehouse_id,t.product_id,t.picked_qty FROM wms_pick_tasks t JOIN wms_bins b ON b.id=t.source_bin_id WHERE t.company_id=$1 AND t.id=$2`, cid, id).Scan(&warehouse, &product, &qty)
		if err == nil && qty > 0 {
			if _, err = h.stock.PostAdjustment(r.Context(), inventory.AdjustmentInput{Code: fmt.Sprintf("WMS-SHIP-%d", id), WarehouseID: warehouse, ProductID: product, Qty: -qty, Note: "WMS shipment", ActorID: uid, RefModule: "WMS"}); err != nil {
				shared.WriteErrorStatus(w, http.StatusBadRequest, err)
				return
			}
		}
	}
	h.transition(w, r, "SHIPPED")
}

// ============================================================================
// Advanced WMS
// ============================================================================

func (h *Handler) createPutAwayTask(w http.ResponseWriter, r *http.Request) {
	_, cid, ok := sessionIDs(r)
	if !ok {
		shared.WriteHTTPError(w, http.StatusForbidden, "")
		return
	}
	var in PutAwayTask
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		shared.WriteErrorStatus(w, http.StatusBadRequest, err)
		return
	}
	in.CompanyID = cid
	created, err := h.service.CreatePutAwayTask(r.Context(), in)
	if err != nil {
		shared.WriteErrorStatus(w, http.StatusInternalServerError, err)
		return
	}
	shared.JSONResponse(w, http.StatusCreated, created)
}

func (h *Handler) createCrossDockingPlan(w http.ResponseWriter, r *http.Request) {
	_, cid, ok := sessionIDs(r)
	if !ok {
		shared.WriteHTTPError(w, http.StatusForbidden, "")
		return
	}
	var in CrossDockingPlan
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		shared.WriteErrorStatus(w, http.StatusBadRequest, err)
		return
	}
	in.CompanyID = cid
	created, err := h.service.CreateCrossDockingPlan(r.Context(), in)
	if err != nil {
		shared.WriteErrorStatus(w, http.StatusInternalServerError, err)
		return
	}
	shared.JSONResponse(w, http.StatusCreated, created)
}

func (h *Handler) createMHEEquipment(w http.ResponseWriter, r *http.Request) {
	_, cid, ok := sessionIDs(r)
	if !ok {
		shared.WriteHTTPError(w, http.StatusForbidden, "")
		return
	}
	var in MHEEquipment
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		shared.WriteErrorStatus(w, http.StatusBadRequest, err)
		return
	}
	in.CompanyID = cid
	created, err := h.service.CreateMHEEquipment(r.Context(), in)
	if err != nil {
		shared.WriteErrorStatus(w, http.StatusInternalServerError, err)
		return
	}
	shared.JSONResponse(w, http.StatusCreated, created)
}
