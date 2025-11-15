package closehttp

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/odyssey-erp/odyssey-erp/internal/close"
	"github.com/odyssey-erp/odyssey-erp/internal/rbac"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/view"
)

// Handler wires HTTP endpoints for managing accounting periods and close runs.
type Handler struct {
	logger    *slog.Logger
	service   *close.Service
	templates *view.Engine
	csrf      *shared.CSRFManager
	rbac      rbac.Middleware
}

// NewHandler constructs a close HTTP handler.
func NewHandler(logger *slog.Logger, service *close.Service, templates *view.Engine, csrf *shared.CSRFManager, rbac rbac.Middleware) *Handler {
	return &Handler{
		logger:    logger,
		service:   service,
		templates: templates,
		csrf:      csrf,
		rbac:      rbac,
	}
}

// MountRoutes registers HTTP routes.
func (h *Handler) MountRoutes(r chi.Router) {
	r.Route("/accounting/periods", func(r chi.Router) {
		r.Use(h.rbac.RequireAny("finance.period.close"))
		r.Get("/", h.listPeriods)
		r.Group(func(r chi.Router) {
			r.Use(h.rbac.RequireAll("finance.period.close"))
			r.Post("/", h.createPeriod)
			r.Post("/{id}/close-run", h.startCloseRun)
		})
	})
	r.Route("/close-runs", func(r chi.Router) {
		r.Use(h.rbac.RequireAny("finance.period.close"))
		r.Get("/{id}", h.showCloseRun)
		r.Group(func(r chi.Router) {
			r.Use(h.rbac.RequireAll("finance.period.close"))
			r.Post("/{id}/checklist/{itemID}", h.updateChecklist)
			r.Post("/{id}/soft-close", h.softClose)
			r.Post("/{id}/hard-close", h.hardClose)
		})
	})
}

func (h *Handler) listPeriods(w http.ResponseWriter, r *http.Request) {
	companyID := h.resolveCompanyID(r)
	periods, err := h.service.ListPeriods(r.Context(), companyID, 50, 0)
	if err != nil {
		h.logger.Error("list periods", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	h.render(w, r, "pages/close/periods.html", map[string]any{
		"CompanyID": companyID,
		"Periods":   periods,
	}, http.StatusOK)
}

func (h *Handler) createPeriod(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	companyID := h.resolveCompanyID(r)
	name := strings.TrimSpace(r.PostFormValue("name"))
	startDate, err := time.Parse("2006-01-02", r.PostFormValue("start_date"))
	if err != nil {
		h.redirectWithFlash(w, r, "/accounting/periods", "danger", "Tanggal mulai tidak valid")
		return
	}
	endDate, err := time.Parse("2006-01-02", r.PostFormValue("end_date"))
	if err != nil {
		h.redirectWithFlash(w, r, "/accounting/periods", "danger", "Tanggal selesai tidak valid")
		return
	}
	input := close.CreatePeriodInput{
		CompanyID: companyID,
		Name:      name,
		StartDate: startDate,
		EndDate:   endDate,
	}
	if _, err := h.service.CreatePeriod(r.Context(), input); err != nil {
		h.logger.Warn("create period", slog.Any("error", err))
		h.redirectWithFlash(w, r, "/accounting/periods", "danger", err.Error())
		return
	}
	h.redirectWithFlash(w, r, "/accounting/periods", "success", "Periode berhasil dibuat")
}

func (h *Handler) startCloseRun(w http.ResponseWriter, r *http.Request) {
	periodID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || periodID == 0 {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	companyID := h.resolveCompanyID(r)
	run, err := h.service.StartCloseRun(r.Context(), close.StartCloseRunInput{
		CompanyID: companyID,
		PeriodID:  periodID,
		ActorID:   currentUser(r),
		Notes:     r.PostFormValue("notes"),
	})
	if err != nil {
		h.logger.Warn("start close run", slog.Any("error", err))
		h.redirectWithFlash(w, r, "/accounting/periods", "danger", err.Error())
		return
	}
	h.redirectWithFlash(w, r, "/close-runs/"+strconv.FormatInt(run.ID, 10), "success", "Close run dimulai")
}

func (h *Handler) showCloseRun(w http.ResponseWriter, r *http.Request) {
	runID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || runID == 0 {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	run, err := h.service.GetCloseRun(r.Context(), runID)
	if err != nil {
		h.logger.Error("get close run", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		return
	}
	period, err := h.service.GetPeriod(r.Context(), run.PeriodID)
	if err != nil {
		h.logger.Error("get period for run", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	h.render(w, r, "pages/close/run.html", map[string]any{
		"Run":               run,
		"Period":            period,
		"ChecklistStatuses": []close.ChecklistStatus{close.ChecklistStatusPending, close.ChecklistStatusInProgress, close.ChecklistStatusDone, close.ChecklistStatusSkipped},
	}, http.StatusOK)
}

func (h *Handler) updateChecklist(w http.ResponseWriter, r *http.Request) {
	runID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || runID == 0 {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	itemID, err := strconv.ParseInt(chi.URLParam(r, "itemID"), 10, 64)
	if err != nil || itemID == 0 {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	status := close.ChecklistStatus(strings.ToUpper(strings.TrimSpace(r.PostFormValue("status"))))
	_, err = h.service.UpdateChecklist(r.Context(), close.ChecklistUpdateInput{
		ItemID:  itemID,
		Status:  status,
		ActorID: currentUser(r),
		Comment: r.PostFormValue("comment"),
	})
	if err != nil {
		h.logger.Warn("update checklist", slog.Any("error", err))
		h.redirectWithFlash(w, r, "/close-runs/"+strconv.FormatInt(runID, 10), "danger", err.Error())
		return
	}
	h.redirectWithFlash(w, r, "/close-runs/"+strconv.FormatInt(runID, 10), "success", "Checklist diperbarui")
}

func (h *Handler) softClose(w http.ResponseWriter, r *http.Request) {
	runID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || runID == 0 {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	if _, err := h.service.SoftClose(r.Context(), runID, currentUser(r)); err != nil {
		h.logger.Warn("soft close", slog.Any("error", err))
		h.redirectWithFlash(w, r, "/close-runs/"+strconv.FormatInt(runID, 10), "danger", err.Error())
		return
	}
	h.redirectWithFlash(w, r, "/close-runs/"+strconv.FormatInt(runID, 10), "success", "Periode disoft-close")
}

func (h *Handler) hardClose(w http.ResponseWriter, r *http.Request) {
	runID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || runID == 0 {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	if _, err := h.service.HardClose(r.Context(), runID, currentUser(r)); err != nil {
		h.logger.Warn("hard close", slog.Any("error", err))
		h.redirectWithFlash(w, r, "/close-runs/"+strconv.FormatInt(runID, 10), "danger", err.Error())
		return
	}
	h.redirectWithFlash(w, r, "/close-runs/"+strconv.FormatInt(runID, 10), "success", "Periode di-hard-close")
}

func (h *Handler) render(w http.ResponseWriter, r *http.Request, template string, data map[string]any, status int) {
	sess := shared.SessionFromContext(r.Context())
	csrfToken, _ := h.csrf.EnsureToken(r.Context(), sess)
	var flash *shared.FlashMessage
	if sess != nil {
		flash = sess.PopFlash()
	}
	viewData := view.TemplateData{
		Title:       "Period Close",
		CSRFToken:   csrfToken,
		Flash:       flash,
		CurrentPath: r.URL.Path,
		Data:        data,
	}
	w.WriteHeader(status)
	if err := h.templates.Render(w, template, viewData); err != nil {
		h.logger.Error("render template", slog.Any("error", err))
	}
}

func (h *Handler) redirectWithFlash(w http.ResponseWriter, r *http.Request, location, kind, message string) {
	if sess := shared.SessionFromContext(r.Context()); sess != nil {
		sess.AddFlash(shared.FlashMessage{Kind: kind, Message: message})
	}
	http.Redirect(w, r, location, http.StatusSeeOther)
}

func (h *Handler) resolveCompanyID(r *http.Request) int64 {
	raw := strings.TrimSpace(r.FormValue("company_id"))
	if raw == "" {
		raw = strings.TrimSpace(r.URL.Query().Get("company_id"))
	}
	if raw == "" {
		return 0
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id < 0 {
		return 0
	}
	return id
}

func currentUser(r *http.Request) int64 {
	sess := shared.SessionFromContext(r.Context())
	if sess == nil {
		return 0
	}
	id, _ := strconv.ParseInt(sess.User(), 10, 64)
	return id
}
