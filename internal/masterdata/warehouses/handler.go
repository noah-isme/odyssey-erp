package warehouses

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/odyssey-erp/odyssey-erp/internal/masterdata/branches"
	"github.com/odyssey-erp/odyssey-erp/internal/masterdata/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/rbac"
	internalShared "github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/view"
)

type Handler struct {
	logger        *slog.Logger
	service       *Service
	branchService *branches.Service
	templates     *view.Engine
	csrf          *internalShared.CSRFManager
	sessions      *internalShared.SessionManager
	rbac          rbac.Middleware
}

func NewHandler(logger *slog.Logger, service *Service, branchService *branches.Service, templates *view.Engine, csrf *internalShared.CSRFManager, sessions *internalShared.SessionManager, rbac rbac.Middleware) *Handler {
	return &Handler{logger: logger, service: service, branchService: branchService, templates: templates, csrf: csrf, sessions: sessions, rbac: rbac}
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit < 1 {
		limit = 10
	}

	filters := shared.ListFilters{
		Page:    page,
		Limit:   limit,
		Search:  r.URL.Query().Get("search"),
		SortBy:  r.URL.Query().Get("sort"),
		SortDir: r.URL.Query().Get("dir"),
	}

	branchIDStr := r.URL.Query().Get("branch_id")
	if branchIDStr != "" {
		if parsed, err := strconv.ParseInt(branchIDStr, 10, 64); err == nil {
			filters.BranchID = &parsed
		}
	}

	warehouses, total, err := h.service.List(r.Context(), filters)
	if err != nil {
		h.logger.Error("list warehouses failed", "error", err)
		http.Error(w, "Failed to load warehouses", http.StatusInternalServerError)
		return
	}

	h.render(w, r, "pages/masterdata/warehouses_list.html", map[string]any{
		"Warehouses": warehouses,
		"Filters":    filters,
		"Total":      total,
	}, http.StatusOK)
}

func (h *Handler) Show(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid warehouse ID", http.StatusBadRequest)
		return
	}

	warehouse, err := h.service.Get(r.Context(), id)
	if err != nil {
		h.logger.Error("get warehouse failed", "error", err, "id", id)
		http.Error(w, "Warehouse not found", http.StatusNotFound)
		return
	}

	h.render(w, r, "pages/masterdata/warehouse_detail.html", map[string]any{
		"Warehouse": warehouse,
	}, http.StatusOK)
}

func (h *Handler) Form(w http.ResponseWriter, r *http.Request) {
	branchesList, _, err := h.branchService.List(r.Context(), shared.ListFilters{})
	if err != nil {
		h.logger.Error("list branches failed", "error", err)
		branchesList = []branches.Branch{}
	}

	h.render(w, r, "pages/masterdata/warehouse_form.html", map[string]any{
		"Errors":    map[string]string{},
		"Warehouse": nil,
		"Branches":  branchesList,
	}, http.StatusOK)
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	branchID, _ := strconv.ParseInt(r.PostFormValue("branch_id"), 10, 64)
	warehouse := Warehouse{
		BranchID: branchID,
		Code:     r.PostFormValue("code"),
		Name:     r.PostFormValue("name"),
		Address:  r.PostFormValue("address"),
	}

	created, err := h.service.Create(r.Context(), warehouse)
	if err != nil {
		h.logger.Error("create warehouse failed", "error", err)
		branchesList, _, _ := h.branchService.List(r.Context(), shared.ListFilters{})
		h.render(w, r, "pages/masterdata/warehouse_form.html", map[string]any{
			"Errors":    map[string]string{"general": internalShared.UserSafeMessage(err)},
			"Warehouse": nil,
			"Branches":  branchesList,
		}, http.StatusBadRequest)
		return
	}

	h.redirectWithFlash(w, r, "/masterdata/warehouses/"+strconv.FormatInt(created.ID, 10), "success", "Warehouse created successfully")
}

func (h *Handler) EditForm(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid warehouse ID", http.StatusBadRequest)
		return
	}

	warehouse, err := h.service.Get(r.Context(), id)
	if err != nil {
		h.logger.Error("get warehouse failed", "error", err, "id", id)
		http.Error(w, "Warehouse not found", http.StatusNotFound)
		return
	}

	branchesList, _, err := h.branchService.List(r.Context(), shared.ListFilters{})
	if err != nil {
		h.logger.Error("list branches failed", "error", err)
		branchesList = []branches.Branch{}
	}

	h.render(w, r, "pages/masterdata/warehouse_form.html", map[string]any{
		"Errors":    map[string]string{},
		"Warehouse": warehouse,
		"Branches":  branchesList,
	}, http.StatusOK)
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid warehouse ID", http.StatusBadRequest)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	branchID, _ := strconv.ParseInt(r.PostFormValue("branch_id"), 10, 64)
	warehouse := Warehouse{
		BranchID: branchID,
		Code:     r.PostFormValue("code"),
		Name:     r.PostFormValue("name"),
		Address:  r.PostFormValue("address"),
	}

	err = h.service.Update(r.Context(), id, warehouse)
	if err != nil {
		h.logger.Error("update warehouse failed", "error", err, "id", id)
		branchesList, _, _ := h.branchService.List(r.Context(), shared.ListFilters{})
		h.render(w, r, "pages/masterdata/warehouse_form.html", map[string]any{
			"Errors":    map[string]string{"general": internalShared.UserSafeMessage(err)},
			"Warehouse": warehouse,
			"Branches":  branchesList,
		}, http.StatusBadRequest)
		return
	}

	h.redirectWithFlash(w, r, "/masterdata/warehouses/"+strconv.FormatInt(id, 10), "success", "Warehouse updated successfully")
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid warehouse ID", http.StatusBadRequest)
		return
	}

	err = h.service.Delete(r.Context(), id)
	if err != nil {
		h.logger.Error("delete warehouse failed", "error", err, "id", id)
		h.redirectWithFlash(w, r, "/masterdata/warehouses", "error", internalShared.UserSafeMessage(err))
		return
	}

	h.redirectWithFlash(w, r, "/masterdata/warehouses", "success", "Warehouse deleted successfully")
}

func (h *Handler) render(w http.ResponseWriter, r *http.Request, template string, data map[string]any, status int) {
	sess := internalShared.SessionFromContext(r.Context())
	csrfToken, _ := h.csrf.EnsureToken(r.Context(), sess)
	var flash *internalShared.FlashMessage
	if sess != nil {
		flash = sess.PopFlash()
	}
	viewData := view.TemplateData{
		Title:       "Master Data",
		CSRFToken:   csrfToken,
		Flash:       flash,
		CurrentPath: r.URL.Path,
		Data:        data,
	}
	w.WriteHeader(status)
	if err := h.templates.Render(w, template, viewData); err != nil {
		h.logger.Error("render template", "error", err, "template", template)
	}
}

func (h *Handler) redirectWithFlash(w http.ResponseWriter, r *http.Request, location, kind, message string) {
	if sess := internalShared.SessionFromContext(r.Context()); sess != nil {
		sess.AddFlash(internalShared.FlashMessage{Kind: kind, Message: message})
	}
	http.Redirect(w, r, location, http.StatusSeeOther)
}
