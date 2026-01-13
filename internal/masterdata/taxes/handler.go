package taxes

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/odyssey-erp/odyssey-erp/internal/masterdata/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/rbac"
	internalShared "github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/view"
)

type Handler struct {
	logger    *slog.Logger
	service   *Service
	templates *view.Engine
	csrf      *internalShared.CSRFManager
	sessions  *internalShared.SessionManager
	rbac      rbac.Middleware
}

func NewHandler(logger *slog.Logger, service *Service, templates *view.Engine, csrf *internalShared.CSRFManager, sessions *internalShared.SessionManager, rbac rbac.Middleware) *Handler {
	return &Handler{logger: logger, service: service, templates: templates, csrf: csrf, sessions: sessions, rbac: rbac}
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

	taxes, total, err := h.service.List(r.Context(), filters)
	if err != nil {
		h.logger.Error("list taxes failed", "error", err)
		http.Error(w, "Failed to load taxes", http.StatusInternalServerError)
		return
	}

	h.render(w, r, "pages/masterdata/taxes_list.html", map[string]any{
		"Taxes":   taxes,
		"Filters": filters,
		"Total":   total,
	}, http.StatusOK)
}

func (h *Handler) Show(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid tax ID", http.StatusBadRequest)
		return
	}

	tax, err := h.service.Get(r.Context(), id)
	if err != nil {
		h.logger.Error("get tax failed", "error", err, "id", id)
		http.Error(w, "Tax not found", http.StatusNotFound)
		return
	}

	h.render(w, r, "pages/masterdata/tax_detail.html", map[string]any{
		"Tax": tax,
	}, http.StatusOK)
}

func (h *Handler) Form(w http.ResponseWriter, r *http.Request) {
	h.render(w, r, "pages/masterdata/tax_form.html", map[string]any{
		"Errors": map[string]string{},
		"Tax":    nil,
	}, http.StatusOK)
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	rate, _ := strconv.ParseFloat(r.PostFormValue("rate"), 64)
	tax := Tax{
		Code: r.PostFormValue("code"),
		Name: r.PostFormValue("name"),
		Rate: rate,
	}

	created, err := h.service.Create(r.Context(), tax)
	if err != nil {
		h.logger.Error("create tax failed", "error", err)
		h.render(w, r, "pages/masterdata/tax_form.html", map[string]any{
			"Errors": map[string]string{"general": internalShared.UserSafeMessage(err)},
			"Tax":    nil,
		}, http.StatusBadRequest)
		return
	}

	h.redirectWithFlash(w, r, "/masterdata/taxes/"+strconv.FormatInt(created.ID, 10), "success", "Tax created successfully")
}

func (h *Handler) EditForm(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid tax ID", http.StatusBadRequest)
		return
	}

	tax, err := h.service.Get(r.Context(), id)
	if err != nil {
		h.logger.Error("get tax failed", "error", err, "id", id)
		http.Error(w, "Tax not found", http.StatusNotFound)
		return
	}

	h.render(w, r, "pages/masterdata/tax_form.html", map[string]any{
		"Errors": map[string]string{},
		"Tax":    tax,
	}, http.StatusOK)
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid tax ID", http.StatusBadRequest)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	rate, _ := strconv.ParseFloat(r.PostFormValue("rate"), 64)
	tax := Tax{
		Code: r.PostFormValue("code"),
		Name: r.PostFormValue("name"),
		Rate: rate,
	}

	err = h.service.Update(r.Context(), id, tax)
	if err != nil {
		h.logger.Error("update tax failed", "error", err, "id", id)
		h.render(w, r, "pages/masterdata/tax_form.html", map[string]any{
			"Errors": map[string]string{"general": internalShared.UserSafeMessage(err)},
			"Tax":    tax,
		}, http.StatusBadRequest)
		return
	}

	h.redirectWithFlash(w, r, "/masterdata/taxes/"+strconv.FormatInt(id, 10), "success", "Tax updated successfully")
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid tax ID", http.StatusBadRequest)
		return
	}

	err = h.service.Delete(r.Context(), id)
	if err != nil {
		h.logger.Error("delete tax failed", "error", err, "id", id)
		h.redirectWithFlash(w, r, "/masterdata/taxes", "error", internalShared.UserSafeMessage(err))
		return
	}

	h.redirectWithFlash(w, r, "/masterdata/taxes", "success", "Tax deleted successfully")
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
