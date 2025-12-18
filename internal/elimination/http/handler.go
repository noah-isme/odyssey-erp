package eliminationhttp

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/odyssey-erp/odyssey-erp/internal/elimination"
	"github.com/odyssey-erp/odyssey-erp/internal/rbac"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/view"
)

// Handler exposes SSR endpoints for elimination flows.
type Handler struct {
	logger    *slog.Logger
	service   *elimination.Service
	templates *view.Engine
	csrf      *shared.CSRFManager
	rbac      rbac.Middleware
}

// NewHandler constructs the HTTP handler.
func NewHandler(logger *slog.Logger, service *elimination.Service, templates *view.Engine, csrf *shared.CSRFManager, rbac rbac.Middleware) *Handler {
	return &Handler{logger: logger, service: service, templates: templates, csrf: csrf, rbac: rbac}
}

// MountRoutes registers elimination routes.
func (h *Handler) MountRoutes(r chi.Router) {
	r.Route("/eliminations", func(r chi.Router) {
		r.Use(h.rbac.RequireAny(shared.PermFinanceConsolManage))
		r.Get("/rules", h.listRules)
		r.Post("/rules", h.createRule)
		r.Get("/runs", h.listRuns)
		r.Post("/runs", h.createRun)
		r.Route("/runs/{id}", func(r chi.Router) {
			r.Get("/", h.showRun)
			r.Post("/simulate", h.simulateRun)
			r.Post("/post", h.postRun)
		})
	})
}

func (h *Handler) listRules(w http.ResponseWriter, r *http.Request) {
	rules, err := h.service.ListRules(r.Context(), 100)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	h.render(w, r, "pages/eliminations/rules.html", "Elimination Rules", map[string]any{
		"Rules": rules,
	}, http.StatusOK)
}

func (h *Handler) createRule(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	actor := currentUser(r)
	groupID := parseOptionalInt(r.PostFormValue("group_id"))
	input := elimination.CreateRuleInput{
		GroupID:         groupID,
		Name:            strings.TrimSpace(r.PostFormValue("name")),
		SourceCompanyID: parseInt64(r.PostFormValue("source_company_id")),
		TargetCompanyID: parseInt64(r.PostFormValue("target_company_id")),
		AccountSource:   strings.TrimSpace(r.PostFormValue("account_src")),
		AccountTarget:   strings.TrimSpace(r.PostFormValue("account_tgt")),
		MatchCriteria:   map[string]any{},
		ActorID:         actor,
	}
	if _, err := h.service.CreateRule(r.Context(), input); err != nil {
		h.redirectWithFlash(w, r, "/eliminations/rules", "danger", err.Error())
		return
	}
	h.redirectWithFlash(w, r, "/eliminations/rules", "success", "Rule created")
}

func (h *Handler) listRuns(w http.ResponseWriter, r *http.Request) {
	runs, err := h.service.ListRuns(r.Context(), 50)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	rules, err := h.service.ListRules(r.Context(), 100)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	periods, err := h.service.RecentPeriods(r.Context(), 12)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	h.render(w, r, "pages/eliminations/runs.html", "Elimination Runs", map[string]any{
		"Runs":    runs,
		"Rules":   rules,
		"Periods": periods,
	}, http.StatusOK)
}

func (h *Handler) createRun(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	actor := currentUser(r)
	input := elimination.CreateRunInput{
		PeriodID: parseInt64(r.PostFormValue("period_id")),
		RuleID:   parseInt64(r.PostFormValue("rule_id")),
		ActorID:  actor,
	}
	if _, err := h.service.CreateRun(r.Context(), input); err != nil {
		h.redirectWithFlash(w, r, "/eliminations/runs", "danger", err.Error())
		return
	}
	h.redirectWithFlash(w, r, "/eliminations/runs", "success", "Run created")
}

func (h *Handler) showRun(w http.ResponseWriter, r *http.Request) {
	id := parseInt64(chi.URLParam(r, "id"))
	run, err := h.service.GetRun(r.Context(), id)
	if err != nil {
		if err == elimination.ErrRunNotFound {
			http.NotFound(w, r)
			return
		}
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	h.render(w, r, "pages/eliminations/run_detail.html", "Elimination Run", map[string]any{
		"Run": run,
	}, http.StatusOK)
}

func (h *Handler) simulateRun(w http.ResponseWriter, r *http.Request) {
	id := parseInt64(chi.URLParam(r, "id"))
	if id == 0 {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	if _, _, err := h.service.SimulateRun(r.Context(), id); err != nil {
		h.redirectWithFlash(w, r, "/eliminations/runs/"+strconv.FormatInt(id, 10), "danger", err.Error())
		return
	}
	h.redirectWithFlash(w, r, "/eliminations/runs/"+strconv.FormatInt(id, 10), "success", "Simulation updated")
}

func (h *Handler) postRun(w http.ResponseWriter, r *http.Request) {
	id := parseInt64(chi.URLParam(r, "id"))
	actor := currentUser(r)
	if actor == 0 {
		http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
		return
	}
	if _, err := h.service.PostRun(r.Context(), id, actor); err != nil {
		h.redirectWithFlash(w, r, "/eliminations/runs/"+strconv.FormatInt(id, 10), "danger", err.Error())
		return
	}
	h.redirectWithFlash(w, r, "/eliminations/runs/"+strconv.FormatInt(id, 10), "success", "Journal posted")
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
	if err := h.templates.Render(w, tpl, viewData); err != nil {
		h.logger.Error("render template", slog.Any("error", err))
	}
}

func (h *Handler) redirectWithFlash(w http.ResponseWriter, r *http.Request, location, kind, message string) {
	if sess := shared.SessionFromContext(r.Context()); sess != nil {
		sess.AddFlash(shared.FlashMessage{Kind: kind, Message: message})
	}
	http.Redirect(w, r, location, http.StatusSeeOther)
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

func parseInt64(value string) int64 {
	v, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil {
		return 0
	}
	return v
}

func currentUser(r *http.Request) int64 {
	sess := shared.SessionFromContext(r.Context())
	if sess == nil {
		return 0
	}
	v, _ := strconv.ParseInt(sess.User(), 10, 64)
	return v
}
