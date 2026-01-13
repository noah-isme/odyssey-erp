package branches

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/odyssey-erp/odyssey-erp/internal/masterdata/companies"
	"github.com/odyssey-erp/odyssey-erp/internal/masterdata/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/rbac"
	internalShared "github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/view"
)

type Handler struct {
	logger         *slog.Logger
	service        *Service
	companyService *companies.Service
	templates      *view.Engine
	csrf           *internalShared.CSRFManager
	sessions       *internalShared.SessionManager
	rbac           rbac.Middleware
}

func NewHandler(logger *slog.Logger, service *Service, companyService *companies.Service, templates *view.Engine, csrf *internalShared.CSRFManager, sessions *internalShared.SessionManager, rbac rbac.Middleware) *Handler {
	return &Handler{logger: logger, service: service, companyService: companyService, templates: templates, csrf: csrf, sessions: sessions, rbac: rbac}
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

	companyIDStr := r.URL.Query().Get("company_id")
	if companyIDStr != "" {
		if parsed, err := strconv.ParseInt(companyIDStr, 10, 64); err == nil {
			filters.CompanyID = &parsed
		}
	}

	branches, total, err := h.service.List(r.Context(), filters)
	if err != nil {
		h.logger.Error("list branches failed", "error", err)
		http.Error(w, "Failed to load branches", http.StatusInternalServerError)
		return
	}

	h.render(w, r, "pages/masterdata/branches_list.html", map[string]any{
		"Branches": branches,
		"Filters":  filters,
		"Total":    total,
	}, http.StatusOK)
}

func (h *Handler) Show(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid branch ID", http.StatusBadRequest)
		return
	}

	branch, err := h.service.Get(r.Context(), id)
	if err != nil {
		h.logger.Error("get branch failed", "error", err, "id", id)
		http.Error(w, "Branch not found", http.StatusNotFound)
		return
	}

	h.render(w, r, "pages/masterdata/branch_detail.html", map[string]any{
		"Branch": branch,
	}, http.StatusOK)
}

func (h *Handler) Form(w http.ResponseWriter, r *http.Request) {
	companiesList, _, err := h.companyService.List(r.Context(), shared.ListFilters{})
	if err != nil {
		h.logger.Error("list companies failed", "error", err)
		companiesList = []companies.Company{}
	}

	h.render(w, r, "pages/masterdata/branch_form.html", map[string]any{
		"Errors":    map[string]string{},
		"Branch":    nil,
		"Companies": companiesList,
	}, http.StatusOK)
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	companyID, _ := strconv.ParseInt(r.PostFormValue("company_id"), 10, 64)
	branch := Branch{
		CompanyID: companyID,
		Code:      r.PostFormValue("code"),
		Name:      r.PostFormValue("name"),
		Address:   r.PostFormValue("address"),
	}

	created, err := h.service.Create(r.Context(), branch)
	if err != nil {
		h.logger.Error("create branch failed", "error", err)
		companiesList, _, _ := h.companyService.List(r.Context(), shared.ListFilters{})
		h.render(w, r, "pages/masterdata/branch_form.html", map[string]any{
			"Errors":    map[string]string{"general": internalShared.UserSafeMessage(err)},
			"Branch":    nil,
			"Companies": companiesList,
		}, http.StatusBadRequest)
		return
	}

	h.redirectWithFlash(w, r, "/masterdata/branches/"+strconv.FormatInt(created.ID, 10), "success", "Branch created successfully")
}

func (h *Handler) EditForm(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid branch ID", http.StatusBadRequest)
		return
	}

	branch, err := h.service.Get(r.Context(), id)
	if err != nil {
		h.logger.Error("get branch failed", "error", err, "id", id)
		http.Error(w, "Branch not found", http.StatusNotFound)
		return
	}

	companiesList, _, err := h.companyService.List(r.Context(), shared.ListFilters{})
	if err != nil {
		h.logger.Error("list companies failed", "error", err)
		companiesList = []companies.Company{}
	}

	h.render(w, r, "pages/masterdata/branch_form.html", map[string]any{
		"Errors":    map[string]string{},
		"Branch":    branch,
		"Companies": companiesList,
	}, http.StatusOK)
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid branch ID", http.StatusBadRequest)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	companyID, _ := strconv.ParseInt(r.PostFormValue("company_id"), 10, 64)
	branch := Branch{
		CompanyID: companyID,
		Code:      r.PostFormValue("code"),
		Name:      r.PostFormValue("name"),
		Address:   r.PostFormValue("address"),
	}

	err = h.service.Update(r.Context(), id, branch)
	if err != nil {
		h.logger.Error("update branch failed", "error", err, "id", id)
		companiesList, _, _ := h.companyService.List(r.Context(), shared.ListFilters{})
		h.render(w, r, "pages/masterdata/branch_form.html", map[string]any{
			"Errors":    map[string]string{"general": internalShared.UserSafeMessage(err)},
			"Branch":    branch,
			"Companies": companiesList,
		}, http.StatusBadRequest)
		return
	}

	h.redirectWithFlash(w, r, "/masterdata/branches/"+strconv.FormatInt(id, 10), "success", "Branch updated successfully")
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid branch ID", http.StatusBadRequest)
		return
	}

	err = h.service.Delete(r.Context(), id)
	if err != nil {
		h.logger.Error("delete branch failed", "error", err, "id", id)
		h.redirectWithFlash(w, r, "/masterdata/branches", "error", internalShared.UserSafeMessage(err))
		return
	}

	h.redirectWithFlash(w, r, "/masterdata/branches", "success", "Branch deleted successfully")
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
