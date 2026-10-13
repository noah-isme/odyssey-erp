package approvals

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
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
	pool      *pgxpool.Pool
}

func NewHandler(logger *slog.Logger, service *Service, templates *view.Engine, csrf *shared.CSRFManager, rbacMW rbac.Middleware, pool *pgxpool.Pool) *Handler {
	return &Handler{logger: logger, service: service, templates: templates, csrf: csrf, rbac: rbacMW, pool: pool}
}
func (h *Handler) MountRoutes(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAny(shared.PermApprovalsInbox))
		r.Get("/", h.inbox)
		r.Post("/{id}/approve", h.approve)
		r.Post("/{id}/reject", h.reject)
	})
	r.Route("/policies", func(r chi.Router) {
		r.Use(h.rbac.RequireAny(shared.PermApprovalsPolicyAdmin))
		r.Get("/", h.policies)
		r.Post("/", h.createPolicy)
	})
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAny(shared.PermApprovalsDelegate))
		r.Post("/delegations", h.createDelegation)
	})
}
func userID(r *http.Request) int64 {
	s := shared.SessionFromContext(r.Context())
	if s == nil {
		return 0
	}
	id, _ := strconv.ParseInt(s.User(), 10, 64)
	return id
}
func (h *Handler) render(w http.ResponseWriter, r *http.Request, name, title string, data any) {
	s := shared.SessionFromContext(r.Context())
	token, _ := h.csrf.EnsureToken(r.Context(), s)
	var flash *shared.FlashMessage
	if s != nil {
		flash = s.PopFlash()
	}
	if err := h.templates.Render(w, name, view.TemplateData{Title: title, CurrentPath: r.URL.Path, CSRFToken: token, Flash: flash, Data: data}); err != nil {
		http.Error(w, http.StatusText(500), 500)
	}
}
func flashRedirect(w http.ResponseWriter, r *http.Request, path, kind, msg string) {
	if s := shared.SessionFromContext(r.Context()); s != nil {
		s.AddFlash(shared.FlashMessage{Kind: kind, Message: msg})
	}
	http.Redirect(w, r, path, http.StatusSeeOther)
}
func (h *Handler) inbox(w http.ResponseWriter, r *http.Request) {
	items, err := h.service.Inbox(r.Context(), userID(r))
	h.render(w, r, "pages/approvals/inbox.html", "My Approvals", map[string]any{"Assignments": items, "Error": err})
}
func (h *Handler) decide(w http.ResponseWriter, r *http.Request, decision string) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err == nil {
		_, err = h.service.Decide(r.Context(), id, userID(r), decision, r.PostFormValue("note"))
	}
	if err != nil {
		var finErr *FinalizeError
		if errors.As(err, &finErr) {
			// The decision is committed; only applying the result failed.
			if h.logger != nil {
				h.logger.Error("approval finalizer failed after decision", slog.Any("error", finErr.Err))
			}
			flashRedirect(w, r, "/approvals", "error", finalizeFailureMessage(decision, finErr.Err))
			return
		}
		flashRedirect(w, r, "/approvals", "error", h.decisionErrorMessage(err))
		return
	}
	flashRedirect(w, r, "/approvals", "success", "Approval decision recorded")
}

// decisionErrorMessage maps decision failures that users can cause (double
// click, a late second approver, a stale link) to specific safe messages; the
// cause of anything else is logged and hidden behind the generic message.
func (h *Handler) decisionErrorMessage(err error) string {
	switch {
	case errors.Is(err, ErrAlreadyDecided):
		return "This request has already been decided."
	case errors.Is(err, ErrNotAssigned):
		return "This request is not assigned to you."
	case errors.Is(err, pgx.ErrNoRows):
		return "The requested item was not found."
	case errors.Is(err, ErrInvalid):
		return "The provided input is invalid."
	}
	if msg := shared.UserSafeMessage(err); msg != "An unexpected error occurred. Please try again." {
		return msg
	}
	if h.logger != nil {
		h.logger.Error("approval decision failed", slog.Any("error", err))
	}
	return shared.UserSafeMessage(err)
}

const genericFinalizeReason = "an unexpected error occurred"

// finalizeFailureMessage builds the flash shown when the decision committed
// but the module finalizer failed. The finalizer's text is shown only when it
// is a plain "hr: " user-safe error; wrapped errors (which may carry database
// or internal detail) and everything else get a generic reason.
func finalizeFailureMessage(decision string, err error) string {
	verb := "Approved"
	if decision == DecisionReject {
		verb = "Rejected"
	}
	return verb + ", but applying the result failed: " + safeFinalizeReason(err) + ". HR can retry from the request page."
}

// safeFinalizeReason walks the wrap chain and returns the first plain
// "hr: " sentinel message, so a sentinel wrapped with extra context (for
// example ErrPayrollAwaitingApproval) is still shown.
func safeFinalizeReason(err error) string {
	for e := err; e != nil; e = errors.Unwrap(e) {
		if reason, ok := plainHRReason(e); ok {
			return reason
		}
	}
	return genericFinalizeReason
}

func plainHRReason(err error) (string, bool) {
	msg := strings.TrimSpace(err.Error())
	const prefix = "hr: "
	if !strings.HasPrefix(msg, prefix) || len(msg) > 200 || strings.ContainsAny(msg, "\r\n") {
		return "", false
	}
	reason := strings.TrimRight(strings.TrimPrefix(msg, prefix), ". ")
	if reason == "" || strings.Contains(reason, ": ") {
		return "", false
	}
	return reason, true
}
func (h *Handler) approve(w http.ResponseWriter, r *http.Request) { h.decide(w, r, DecisionApprove) }
func (h *Handler) reject(w http.ResponseWriter, r *http.Request)  { h.decide(w, r, DecisionReject) }

type option struct {
	ID   int64
	Name string
}

func (h *Handler) loadOptions(r *http.Request) ([]option, []option) {
	users := []option{}
	roles := []option{}
	if h.pool == nil {
		return users, roles
	}
	rows, err := h.pool.Query(r.Context(), `SELECT id,COALESCE(NULLIF(name,''),email) FROM users WHERE is_active ORDER BY 2`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var o option
			if rows.Scan(&o.ID, &o.Name) == nil {
				users = append(users, o)
			}
		}
	}
	rr, err := h.pool.Query(r.Context(), `SELECT id,name FROM roles ORDER BY name`)
	if err == nil {
		defer rr.Close()
		for rr.Next() {
			var o option
			if rr.Scan(&o.ID, &o.Name) == nil {
				roles = append(roles, o)
			}
		}
	}
	return users, roles
}
func (h *Handler) policies(w http.ResponseWriter, r *http.Request) {
	items, err := h.service.Policies(r.Context())
	users, roles := h.loadOptions(r)
	h.render(w, r, "pages/approvals/policies.html", "Approval Policies", map[string]any{"Policies": items, "Users": users, "Roles": roles, "Error": err})
}
func parseOptionalInt(v string) *int64 {
	id, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
	if err != nil || id <= 0 {
		return nil
	}
	return &id
}
func parseOptionalFloat(v string) *float64 {
	f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
	if err != nil {
		return nil
	}
	return &f
}
func (h *Handler) createPolicy(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	min, _ := strconv.ParseFloat(r.FormValue("min_amount"), 64)
	in := CreatePolicyInput{Name: r.FormValue("name"), Module: r.FormValue("module"), CompanyID: parseOptionalInt(r.FormValue("company_id")), MinAmount: min, MaxAmount: parseOptionalFloat(r.FormValue("max_amount")), CreatedBy: userID(r)}
	names := r.Form["step_name"]
	kinds := r.Form["approver_kind"]
	for i, name := range names {
		if strings.TrimSpace(name) == "" {
			continue
		}
		step := PolicyStep{Order: i + 1, Name: name, RequiredApprovals: 1}
		kind := "user"
		if i < len(kinds) {
			kind = kinds[i]
		}
		if kind == "manager" {
			step.ApproverManager = true
		}
		if kind == "user" && i < len(r.Form["approver_user_id"]) {
			step.ApproverUserID = parseOptionalInt(r.Form["approver_user_id"][i])
		}
		if kind == "role" && i < len(r.Form["approver_role_id"]) && step.ApproverUserID == nil {
			step.ApproverRoleID = parseOptionalInt(r.Form["approver_role_id"][i])
		}
		in.Steps = append(in.Steps, step)
	}
	_, err := h.service.CreatePolicy(r.Context(), in)
	if err != nil {
		flashRedirect(w, r, "/approvals/policies", "error", shared.UserSafeMessage(err))
		return
	}
	flashRedirect(w, r, "/approvals/policies", "success", "Approval policy created")
}
func (h *Handler) createDelegation(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	end, err := time.Parse("2006-01-02", r.FormValue("ends_at"))
	delegate := parseOptionalInt(r.FormValue("delegate_id"))
	if err == nil && delegate != nil {
		err = h.service.Delegate(r.Context(), DelegationInput{DelegatorID: userID(r), DelegateID: *delegate, Module: r.FormValue("module"), StartsAt: start, EndsAt: end.Add(24*time.Hour - time.Nanosecond)})
	} else if err == nil {
		err = ErrInvalid
	}
	if err != nil {
		flashRedirect(w, r, "/approvals/policies", "error", "Invalid delegation")
		return
	}
	flashRedirect(w, r, "/approvals/policies", "success", "Delegation created")
}
