package projects

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/odyssey-erp/odyssey-erp/internal/rbac"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
)

type Handler struct {
	service *Service
	rbac    rbac.Middleware
	pool    *pgxpool.Pool
}

func NewHandler(service *Service, middleware rbac.Middleware, pools ...*pgxpool.Pool) *Handler {
	var pool *pgxpool.Pool
	if len(pools) > 0 {
		pool = pools[0]
	}
	return &Handler{service: service, rbac: middleware, pool: pool}
}
func (h *Handler) MountRoutes(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAny("projects.manage"))
		r.Post("/", h.createProject)
		r.Post("/{id}/tasks", h.createTask)
		r.Post("/timesheets", h.create)
		r.Post("/timesheets/{id}/submit", h.submit)
		r.Post("/timesheets/{id}/approve", h.approve)
		r.Post("/timesheets/{id}/reject", h.reject)
		r.Post("/timesheets/{id}/lock", h.lock)
	})
}

func (h *Handler) createProject(w http.ResponseWriter, r *http.Request) {
	if h.pool == nil {
		http.Error(w, "projects database is unavailable", 503)
		return
	}
	u, c, ok := ids(r)
	if !ok {
		http.Error(w, "unauthorized", 401)
		return
	}
	var in struct {
		Code, Name, Currency string
		ManagerID            *int64 `json:"manager_id"`
	}
	if !decode(w, r, &in) || in.Code == "" || in.Name == "" {
		if in.Code == "" {
			http.Error(w, "code and name are required", 400)
		}
		return
	}
	if in.Currency == "" {
		in.Currency = "IDR"
	}
	var response struct {
		ID                           int64 `json:"id"`
		Code, Name, Currency, Status string
	}
	err := h.pool.QueryRow(r.Context(), `INSERT INTO projects(company_id,code,name,currency,status,manager_id,created_by) VALUES($1,$2,$3,$4,'OPEN',$5,$6) RETURNING id,code,name,currency,status`, c, in.Code, in.Name, in.Currency, in.ManagerID, u).Scan(&response.ID, &response.Code, &response.Name, &response.Currency, &response.Status)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	jsonOut(w, 201, response)
}

func (h *Handler) createTask(w http.ResponseWriter, r *http.Request) {
	if h.pool == nil {
		http.Error(w, "projects database is unavailable", 503)
		return
	}
	_, c, ok := ids(r)
	if !ok {
		http.Error(w, "unauthorized", 401)
		return
	}
	projectID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid project id", 400)
		return
	}
	var in struct{ Code, Name string }
	if !decode(w, r, &in) || in.Code == "" || in.Name == "" {
		http.Error(w, "code and name are required", 400)
		return
	}
	var response struct {
		ID, ProjectID      int64
		Code, Name, Status string
	}
	err = h.pool.QueryRow(r.Context(), `INSERT INTO project_tasks(project_id,code,name) SELECT id,$2,$3 FROM projects WHERE id=$1 AND company_id=$4 RETURNING id,project_id,code,name,status`, projectID, in.Code, in.Name, c).Scan(&response.ID, &response.ProjectID, &response.Code, &response.Name, &response.Status)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	jsonOut(w, 201, response)
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
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if e := json.NewDecoder(r.Body).Decode(v); e != nil {
		http.Error(w, "invalid JSON", 400)
		return false
	}
	return true
}
func jsonOut(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	u, c, ok := ids(r)
	if !ok {
		http.Error(w, "unauthorized", 401)
		return
	}
	var s Timesheet
	if !decode(w, r, &s) {
		return
	}
	s.CompanyID = c
	s.EmployeeID = u
	out, e := h.service.CreateTimesheet(r.Context(), s)
	if e != nil {
		http.Error(w, e.Error(), 400)
		return
	}
	jsonOut(w, 201, out)
}
func (h *Handler) transition(w http.ResponseWriter, r *http.Request, fn func(context.Context, int64, int64, int64) (Timesheet, error)) {
	u, c, ok := ids(r)
	if !ok {
		http.Error(w, "unauthorized", 401)
		return
	}
	id, e := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if e != nil {
		http.Error(w, "invalid timesheet id", 400)
		return
	}
	s, e := fn(r.Context(), c, u, id)
	if errors.Is(e, ErrNotFound) {
		http.Error(w, "not found", 404)
		return
	}
	if e != nil {
		http.Error(w, e.Error(), 400)
		return
	}
	jsonOut(w, 200, s)
}
func (h *Handler) submit(w http.ResponseWriter, r *http.Request) {
	h.transition(w, r, h.service.Submit)
}
func (h *Handler) approve(w http.ResponseWriter, r *http.Request) {
	h.transition(w, r, h.service.Approve)
}
func (h *Handler) reject(w http.ResponseWriter, r *http.Request) {
	h.transition(w, r, h.service.Reject)
}
func (h *Handler) lock(w http.ResponseWriter, r *http.Request) { h.transition(w, r, h.service.Lock) }
