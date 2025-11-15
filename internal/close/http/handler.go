package closehttp

import (
	"context"
	"fmt"
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

const periodsPageLimit = 100

type closeService interface {
	ListPeriods(ctx context.Context, companyID int64, limit, offset int) ([]close.Period, error)
	CreatePeriod(ctx context.Context, in close.CreatePeriodInput) (close.Period, error)
	StartCloseRun(ctx context.Context, in close.StartCloseRunInput) (close.CloseRun, error)
	GetCloseRun(ctx context.Context, id int64) (close.CloseRun, error)
	GetPeriod(ctx context.Context, id int64) (close.Period, error)
	UpdateChecklist(ctx context.Context, in close.ChecklistUpdateInput) (close.ChecklistItem, error)
	SoftClose(ctx context.Context, runID, actorID int64) (close.Period, error)
	HardClose(ctx context.Context, runID, actorID int64) (close.Period, error)
}

// Handler wires HTTP endpoints for managing accounting periods and close runs.
type Handler struct {
	logger    *slog.Logger
	service   closeService
	templates *view.Engine
	csrf      *shared.CSRFManager
	rbac      rbac.Middleware
}

type periodListPageData struct {
	CompanyID  int64
	YearFilter string
	YearError  string
	Periods    []periodListRow
}

type periodListRow struct {
	Period       close.Period
	StatusBadge  badgeView
	HasRun       bool
	RunURL       string
	ShowStartRun bool
}

type closeRunPageData struct {
	Period            close.Period
	Run               close.CloseRun
	PeriodBadge       badgeView
	RunBadge          badgeView
	Checklist         []checklistRowView
	ChecklistStatuses []statusOption
	Summary           checklistSummary
	SoftClose         actionState
	HardClose         actionState
}

type checklistRowView struct {
	Item        close.ChecklistItem
	StatusBadge badgeView
}

type badgeView struct {
	Label string
	Kind  string
}

type actionState struct {
	Enabled bool
	Message string
}

type checklistSummary struct {
	Completed       int
	Total           int
	Pending         int
	ProgressPercent int
	ProgressMax     int
	HasItems        bool
}

type statusOption struct {
	Value close.ChecklistStatus
	Label string
}

// NewHandler constructs a close HTTP handler.
func NewHandler(logger *slog.Logger, service closeService, templates *view.Engine, csrf *shared.CSRFManager, rbac rbac.Middleware) *Handler {
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
	periods, err := h.service.ListPeriods(r.Context(), companyID, periodsPageLimit, 0)
	if err != nil {
		h.logger.Error("list periods", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	yearFilter := strings.TrimSpace(r.URL.Query().Get("year"))
	var yearValue int
	var yearErr string
	if yearFilter != "" {
		value, err := strconv.Atoi(yearFilter)
		if err != nil || value <= 0 {
			yearErr = "Tahun tidak valid"
		} else {
			yearValue = value
		}
	}
	filtered := periods
	if yearValue > 0 {
		filtered = make([]close.Period, 0, len(periods))
		for _, period := range periods {
			if period.StartDate.Year() == yearValue || period.EndDate.Year() == yearValue {
				filtered = append(filtered, period)
			}
		}
	}
	rows := make([]periodListRow, 0, len(filtered))
	for _, period := range filtered {
		rows = append(rows, newPeriodListRow(period))
	}
	h.render(w, r, "pages/close/periods.html", "Accounting Periods", periodListPageData{
		CompanyID:  companyID,
		YearFilter: yearFilter,
		YearError:  yearErr,
		Periods:    rows,
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
		Notes:     strings.TrimSpace(r.PostFormValue("notes")),
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
	summary := summariseChecklist(run.Checklist)
	rows := make([]checklistRowView, 0, len(run.Checklist))
	for _, item := range run.Checklist {
		rows = append(rows, checklistRowView{Item: item, StatusBadge: badgeForChecklistStatus(item.Status)})
	}
	data := closeRunPageData{
		Period:            period,
		Run:               run,
		PeriodBadge:       badgeForPeriodStatus(period.Status),
		RunBadge:          badgeForRunStatus(run.Status),
		Checklist:         rows,
		ChecklistStatuses: checklistStatusOptions(),
		Summary:           summary,
		SoftClose:         softCloseState(period.Status),
		HardClose:         hardCloseState(period.Status, summary),
	}
	h.render(w, r, "pages/close/run.html", "Close Run", data, http.StatusOK)
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
		Comment: strings.TrimSpace(r.PostFormValue("comment")),
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

func (h *Handler) render(w http.ResponseWriter, r *http.Request, template string, title string, data any, status int) {
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
		if sess := shared.SessionFromContext(r.Context()); sess != nil {
			raw = strings.TrimSpace(sess.Get("company_id"))
		}
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

func newPeriodListRow(period close.Period) periodListRow {
	hasRun := period.LatestRunID > 0
	var runURL string
	if hasRun {
		runURL = "/close-runs/" + strconv.FormatInt(period.LatestRunID, 10)
	}
	return periodListRow{
		Period:       period,
		StatusBadge:  badgeForPeriodStatus(period.Status),
		HasRun:       hasRun,
		RunURL:       runURL,
		ShowStartRun: !hasRun && period.Status == close.PeriodStatusOpen,
	}
}

func summariseChecklist(items []close.ChecklistItem) checklistSummary {
	summary := checklistSummary{Total: len(items), HasItems: len(items) > 0, ProgressMax: len(items)}
	for _, item := range items {
		switch item.Status {
		case close.ChecklistStatusDone, close.ChecklistStatusSkipped:
			summary.Completed++
		default:
			summary.Pending++
		}
	}
	if summary.ProgressMax == 0 {
		summary.ProgressMax = 1
	}
	if summary.Total > 0 {
		summary.ProgressPercent = (summary.Completed*100 + summary.Total/2) / summary.Total
	}
	return summary
}

func badgeForPeriodStatus(status close.PeriodStatus) badgeView {
	switch status {
	case close.PeriodStatusSoftClosed:
		return badgeView{Label: "Soft Closed", Kind: "warning"}
	case close.PeriodStatusHardClosed:
		return badgeView{Label: "Hard Closed", Kind: "success"}
	default:
		return badgeView{Label: "Open", Kind: "info"}
	}
}

func badgeForRunStatus(status close.RunStatus) badgeView {
	switch status {
	case close.RunStatusInProgress:
		return badgeView{Label: "In Progress", Kind: "info"}
	case close.RunStatusCompleted:
		return badgeView{Label: "Selesai", Kind: "success"}
	case close.RunStatusCancelled:
		return badgeView{Label: "Dibatalkan", Kind: "danger"}
	default:
		return badgeView{Label: "Draft", Kind: "muted"}
	}
}

func badgeForChecklistStatus(status close.ChecklistStatus) badgeView {
	switch status {
	case close.ChecklistStatusDone:
		return badgeView{Label: "Done", Kind: "success"}
	case close.ChecklistStatusSkipped:
		return badgeView{Label: "Skipped", Kind: "muted"}
	case close.ChecklistStatusInProgress:
		return badgeView{Label: "In Progress", Kind: "info"}
	default:
		return badgeView{Label: "Pending", Kind: "warning"}
	}
}

func checklistStatusOptions() []statusOption {
	statuses := []close.ChecklistStatus{
		close.ChecklistStatusPending,
		close.ChecklistStatusInProgress,
		close.ChecklistStatusDone,
		close.ChecklistStatusSkipped,
	}
	options := make([]statusOption, 0, len(statuses))
	for _, status := range statuses {
		options = append(options, statusOption{Value: status, Label: humanizeStatus(string(status))})
	}
	return options
}

func softCloseState(status close.PeriodStatus) actionState {
	switch status {
	case close.PeriodStatusOpen:
		return actionState{Enabled: true}
	case close.PeriodStatusSoftClosed:
		return actionState{Enabled: false, Message: "Periode sudah soft close"}
	case close.PeriodStatusHardClosed:
		return actionState{Enabled: false, Message: "Periode sudah hard close"}
	default:
		return actionState{Enabled: false}
	}
}

func hardCloseState(status close.PeriodStatus, summary checklistSummary) actionState {
	if status == close.PeriodStatusHardClosed {
		return actionState{Enabled: false, Message: "Periode sudah hard close"}
	}
	if summary.Total == 0 {
		return actionState{Enabled: false, Message: "Checklist belum siap"}
	}
	if summary.Completed < summary.Total {
		return actionState{Enabled: false, Message: fmt.Sprintf("Checklist belum selesai (%d dari %d)", summary.Completed, summary.Total)}
	}
	return actionState{Enabled: true}
}

func humanizeStatus(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	parts := strings.Split(value, "_")
	for i, part := range parts {
		if part == "" {
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + part[1:]
	}
	return strings.Join(parts, " ")
}
