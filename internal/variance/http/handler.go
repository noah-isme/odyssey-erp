package variancehttp

import (
	"encoding/csv"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/odyssey-erp/odyssey-erp/internal/rbac"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/variance"
	"github.com/odyssey-erp/odyssey-erp/internal/view"
	"github.com/odyssey-erp/odyssey-erp/jobs"
)

// Handler wires variance SSR endpoints.
type Handler struct {
	logger    *slog.Logger
	service   *variance.Service
	templates *view.Engine
	csrf      *shared.CSRFManager
	rbac      rbac.Middleware
	jobs      *jobs.Client
}

// NewHandler constructs handler.
func NewHandler(logger *slog.Logger, service *variance.Service, templates *view.Engine, csrf *shared.CSRFManager, rbac rbac.Middleware, jobsClient *jobs.Client) *Handler {
	return &Handler{logger: logger, service: service, templates: templates, csrf: csrf, rbac: rbac, jobs: jobsClient}
}

// MountRoutes registers routes.
func (h *Handler) MountRoutes(r chi.Router) {
	r.Route("/variance", func(r chi.Router) {
		r.Use(h.rbac.RequireAny("finance.period.close"))
		r.Get("/rules", h.listRules)
		r.Post("/rules", h.createRule)
		r.Get("/snapshots", h.listSnapshots)
		r.Post("/snapshots", h.triggerSnapshot)
		r.Get("/snapshots/{id}", h.showSnapshot)
		r.Get("/snapshots/{id}/export", h.exportSnapshot)
	})
}

func (h *Handler) listRules(w http.ResponseWriter, r *http.Request) {
	rules, err := h.service.ListRules(r.Context(), 0)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	h.render(w, r, "pages/variance/rules.html", "Variance Rules", map[string]any{
		"Rules": rules,
	}, http.StatusOK)
}

func (h *Handler) createRule(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	actor := currentUser(r)
	compare := parseOptionalInt(r.PostFormValue("compare_period_id"))
	input := variance.CreateRuleInput{
		CompanyID:       parseInt64(r.PostFormValue("company_id")),
		Name:            strings.TrimSpace(r.PostFormValue("name")),
		ComparisonType:  variance.RuleComparison(strings.ToUpper(strings.TrimSpace(r.PostFormValue("comparison_type")))),
		BasePeriodID:    parseInt64(r.PostFormValue("base_period_id")),
		ComparePeriodID: compare,
		ActorID:         actor,
	}
	if _, err := h.service.CreateRule(r.Context(), input); err != nil {
		h.redirectWithFlash(w, r, "/variance/rules", "danger", err.Error())
		return
	}
	h.redirectWithFlash(w, r, "/variance/rules", "success", "Rule created")
}

func (h *Handler) listSnapshots(w http.ResponseWriter, r *http.Request) {
	snapshots, err := h.service.ListSnapshots(r.Context(), 50)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	rules, err := h.service.ListRules(r.Context(), 0)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	h.render(w, r, "pages/variance/snapshots.html", "Variance Snapshots", map[string]any{
		"Snapshots": snapshots,
		"Rules":     rules,
	}, http.StatusOK)
}

func (h *Handler) triggerSnapshot(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	req := variance.SnapshotRequest{
		RuleID:   parseInt64(r.PostFormValue("rule_id")),
		PeriodID: parseInt64(r.PostFormValue("period_id")),
		ActorID:  currentUser(r),
	}
	snapshot, err := h.service.TriggerSnapshot(r.Context(), req)
	if err != nil {
		h.redirectWithFlash(w, r, "/variance/snapshots", "danger", err.Error())
		return
	}
	if h.jobs != nil {
		if _, err := h.jobs.EnqueueVarianceSnapshot(r.Context(), snapshot.ID); err != nil && h.logger != nil {
			h.logger.Warn("enqueue variance snapshot", slog.Any("error", err))
		}
	}
	h.redirectWithFlash(w, r, "/variance/snapshots", "success", "Snapshot queued")
}

func (h *Handler) showSnapshot(w http.ResponseWriter, r *http.Request) {
	id := parseInt64(chi.URLParam(r, "id"))
	snapshot, err := h.service.GetSnapshot(r.Context(), id)
	if err != nil {
		if err == variance.ErrSnapshotNotFound {
			http.NotFound(w, r)
			return
		}
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	rows, err := h.service.LoadSnapshotPayload(r.Context(), id)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	h.render(w, r, "pages/variance/snapshot_detail.html", "Variance Snapshot", map[string]any{
		"Snapshot": snapshot,
		"Rows":     rows,
	}, http.StatusOK)
}

func (h *Handler) exportSnapshot(w http.ResponseWriter, r *http.Request) {
	id := parseInt64(chi.URLParam(r, "id"))
	rows, err := h.service.LoadSnapshotPayload(r.Context(), id)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	if len(rows) == 0 {
		http.Error(w, "snapshot empty", http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", "attachment; filename=variance_snapshot.csv")
	writer := csv.NewWriter(w)
	for _, row := range variance.ExportRows(rows) {
		if err := writer.Write(row); err != nil {
			break
		}
	}
	writer.Flush()
}

func (h *Handler) render(w http.ResponseWriter, r *http.Request, tpl, title string, data any, status int) {
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
	if err := h.templates.Render(w, tpl, viewData); err != nil && h.logger != nil {
		h.logger.Error("render variance template", slog.Any("error", err))
	}
}

func (h *Handler) redirectWithFlash(w http.ResponseWriter, r *http.Request, location, kind, message string) {
	if sess := shared.SessionFromContext(r.Context()); sess != nil {
		sess.AddFlash(shared.FlashMessage{Kind: kind, Message: message})
	}
	http.Redirect(w, r, location, http.StatusSeeOther)
}

func parseInt64(value string) int64 {
	v, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil {
		return 0
	}
	return v
}

func parseOptionalInt(value string) *int64 {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	v, err := strconv.ParseInt(value, 10, 64)
	if err != nil || v == 0 {
		return nil
	}
	return &v
}

func currentUser(r *http.Request) int64 {
	sess := shared.SessionFromContext(r.Context())
	if sess == nil {
		return 0
	}
	v, _ := strconv.ParseInt(sess.User(), 10, 64)
	return v
}
