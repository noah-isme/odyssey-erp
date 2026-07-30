package tax

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/odyssey-erp/odyssey-erp/internal/rbac"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/view"
)

type Handler struct {
	logger    *slog.Logger
	service   *Service
	templates *view.Engine
	csrf      *shared.CSRFManager
	rbac      rbac.Middleware
}

func NewHandler(logger *slog.Logger, service *Service, templates *view.Engine, csrf *shared.CSRFManager, middleware rbac.Middleware) *Handler {
	return &Handler{logger: logger, service: service, templates: templates, csrf: csrf, rbac: middleware}
}

func (h *Handler) MountRoutes(r chi.Router) {
	r.Group(func(r chi.Router) { r.Use(h.rbac.RequireAny("tax.view")); r.Get("/", h.dashboard) })
	r.Group(func(r chi.Router) { r.Use(h.rbac.RequireAny("tax.period.lock")); r.Post("/periods/{id}/lock", h.lock) })
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAny("tax.config.manage"))
		r.Post("/periods/{id}/build", h.build)
	})
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAny("tax.document.correct"))
		r.Post("/documents/{id}/cancel", h.cancel)
		r.Post("/documents/{id}/replace", h.replace)
	})
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAny("tax.report.export"))
		r.Get("/periods/{id}/export", h.export)
	})
}

func taxActorID(r *http.Request) int64 {
	s := shared.SessionFromContext(r.Context())
	if s == nil {
		return 0
	}
	id, _ := strconv.ParseInt(s.User(), 10, 64)
	return id
}
func taxCompanyID(r *http.Request) int64 {
	s := shared.SessionFromContext(r.Context())
	if s == nil {
		return 0
	}
	id, _ := strconv.ParseInt(s.Get("company_id"), 10, 64)
	return id
}

func (h *Handler) dashboard(w http.ResponseWriter, r *http.Request) {
	companyID := taxCompanyID(r)
	periodID, _ := strconv.ParseInt(r.URL.Query().Get("period_id"), 10, 64)
	periods, periodErr := h.service.Periods(r.Context(), companyID)
	if periodID == 0 && len(periods) > 0 {
		periodID = periods[0].ID
	}
	documents, docErr := h.service.Documents(r.Context(), companyID, periodID)
	recap, recapErr := h.service.Recap(r.Context(), companyID, periodID)
	s := shared.SessionFromContext(r.Context())
	token, _ := h.csrf.EnsureToken(r.Context(), s)
	data := map[string]any{"Periods": periods, "PeriodID": periodID, "Documents": documents, "Recap": recap, "Error": errors.Join(periodErr, docErr, recapErr)}
	if err := h.templates.Render(w, "pages/tax/dashboard.html", view.TemplateData{Title: "Tax Compliance", CurrentPath: r.URL.Path, CSRFToken: token, Flash: s.PopFlash(), Data: data}); err != nil {
		h.logger.Error("render tax dashboard", slog.Any("error", err))
		http.Error(w, http.StatusText(500), 500)
	}
}

func (h *Handler) lock(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	err := h.service.Lock(r.Context(), taxCompanyID(r), id, taxActorID(r))
	h.redirect(w, r, err, "Tax period locked")
}
func (h *Handler) build(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	err := h.service.BuildPeriod(r.Context(), taxCompanyID(r), id, taxActorID(r))
	h.redirect(w, r, err, "Tax ledger rebuilt from posted documents")
}
func (h *Handler) cancel(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	err := h.service.Cancel(r.Context(), id, taxActorID(r), r.FormValue("reason"))
	h.redirect(w, r, err, "Tax document cancelled")
}
func (h *Handler) replace(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	replacement, _ := strconv.ParseInt(r.FormValue("replacement_id"), 10, 64)
	err := h.service.Replace(r.Context(), id, replacement, taxActorID(r), r.FormValue("reason"))
	h.redirect(w, r, err, "Tax document replaced")
}
func (h *Handler) export(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	kind := r.URL.Query().Get("kind")
	if kind == "" {
		kind = "CORETAX_OUTPUT_VAT"
	}
	result, err := h.service.Export(r.Context(), taxCompanyID(r), id, taxActorID(r), kind)
	if err != nil {
		shared.WriteHTTPError(w, http.StatusConflict, shared.UserSafeMessage(err))
		return
	}
	w.Header().Set("Content-Type", result.MediaType)
	w.Header().Set("Content-Disposition", "attachment; filename=coretax-"+result.SchemaVersion+".xml")
	_, _ = w.Write([]byte(result.Content))
}
func (h *Handler) redirect(w http.ResponseWriter, r *http.Request, err error, success string) {
	s := shared.SessionFromContext(r.Context())
	kind, msg := "success", success
	if err != nil {
		kind, msg = "error", shared.UserSafeMessage(err)
	}
	if s != nil {
		s.AddFlash(shared.FlashMessage{Kind: kind, Message: msg})
	}
	http.Redirect(w, r, "/tax", http.StatusSeeOther)
}
