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
	"net/http"
	"strconv"
)

type Handler struct {
	service *Service
	rbac    rbac.Middleware
	pool    *pgxpool.Pool
	stock   *inventory.Service
}

func NewHandler(s *Service, m rbac.Middleware, pools ...*pgxpool.Pool) *Handler {
	var p *pgxpool.Pool
	if len(pools) > 0 {
		p = pools[0]
	}
	return &Handler{service: s, rbac: m, pool: p}
}
func (h *Handler) SetInventoryService(stock *inventory.Service) { h.stock = stock }
func (h *Handler) MountRoutes(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAny("mrp.view", "mrp.manage"))
		r.Post("/boms", h.createBOM)
		r.Post("/work-orders", h.createWorkOrder)
		r.Post("/work-orders/{id}/release", h.release)
		r.Post("/work-orders/{id}/start", h.start)
		r.Post("/work-orders/{id}/complete", h.complete)
	})
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
		Quantity float64 `json:"quantity"`
	}
	if e = json.NewDecoder(r.Body).Decode(&in); e != nil {
		http.Error(w, "invalid JSON", 400)
		return
	}
	o, e := h.service.Complete(r.Context(), c, id, in.Quantity)
	if errors.Is(e, ErrNotFound) {
		http.Error(w, "not found", 404)
		return
	}
	if e != nil {
		http.Error(w, e.Error(), 400)
		return
	}
	if h.stock != nil && h.pool != nil {
		var warehouse, product, bom int64
		if qerr := h.pool.QueryRow(r.Context(), `SELECT warehouse_id,product_id,COALESCE(bom_id,0) FROM mrp_work_orders WHERE id=$1 AND company_id=$2`, id, c).Scan(&warehouse, &product, &bom); qerr == nil && warehouse > 0 && bom > 0 {
			rows, qerr := h.pool.Query(r.Context(), `SELECT component_product_id,quantity::float8 FROM mrp_bom_lines WHERE bom_id=$1`, bom)
			if qerr == nil {
				defer rows.Close()
				for rows.Next() {
					var component int64
					var qty float64
					if rows.Scan(&component, &qty) == nil {
						if _, qerr = h.stock.PostAdjustment(r.Context(), inventory.AdjustmentInput{Code: fmt.Sprintf("MRP-CONSUME-%d-%d", id, component), WarehouseID: warehouse, ProductID: component, Qty: -qty * in.Quantity, Note: "MRP material consumption", ActorID: u, RefModule: "MRP"}); qerr != nil {
							http.Error(w, qerr.Error(), 400)
							return
						}
					}
				}
			}
			if _, qerr = h.stock.PostAdjustment(r.Context(), inventory.AdjustmentInput{Code: fmt.Sprintf("MRP-RECEIPT-%d", id), WarehouseID: warehouse, ProductID: product, Qty: in.Quantity, Note: "MRP finished goods receipt", ActorID: u, RefModule: "MRP"}); qerr != nil {
				http.Error(w, qerr.Error(), 400)
				return
			}
		}
	}
	out(w, 200, o)
}
