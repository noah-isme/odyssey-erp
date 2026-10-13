package expenses

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/odyssey-erp/odyssey-erp/internal/rbac"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/view"
)

// HandlerService is the subset of *Service used by the HR admin page.
type HandlerService interface {
	ListAll(ctx context.Context, companyID int64, f Filter) ([]Claim, error)
	RetryApply(ctx context.Context, companyID, claimID, actorID int64) error
}

// Handler serves /hr/expenses, the HR admin view of expense claims.
// Decisions happen in /approvals; this page lists claims and re-runs a
// failed apply.
type Handler struct {
	logger    *slog.Logger
	service   HandlerService
	templates *view.Engine
	csrf      *shared.CSRFManager
	rbac      rbac.Middleware
}

// NewHandler builds a Handler.
func NewHandler(l *slog.Logger, s HandlerService, t *view.Engine, c *shared.CSRFManager, r rbac.Middleware) *Handler {
	if l == nil {
		l = slog.Default()
	}
	return &Handler{logger: l, service: s, templates: t, csrf: c, rbac: r}
}

// MountRoutes mounts /hr/expenses.
func (h *Handler) MountRoutes(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAny(shared.PermHRExpenseAdmin))
		r.Get("/", h.list)
		r.Post("/{id}/retry", h.retry)
	})
}

// flashError shows "hr: ..." domain errors (including finalizer apply
// errors returned by RetryApply) to the user and hides the rest.
func flashError(err error) string {
	msg := err.Error()
	if strings.HasPrefix(msg, "hr: ") {
		if trimmed := strings.TrimSpace(strings.TrimPrefix(msg, "hr: ")); trimmed != "" {
			return strings.ToUpper(trimmed[:1]) + trimmed[1:]
		}
	}
	return shared.UserSafeMessage(err)
}

func parseDate(v string) *time.Time {
	t, err := time.Parse("2006-01-02", strings.TrimSpace(v))
	if err != nil {
		return nil
	}
	return &t
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	ident, ok := shared.IdentityFromContext(r.Context())
	if !ok {
		shared.WriteHTTPError(w, http.StatusUnauthorized, shared.ErrUnauthorized.Error())
		return
	}
	q := r.URL.Query()
	statusFilter := strings.ToUpper(strings.TrimSpace(q.Get("status")))
	switch statusFilter {
	case StatusPending, StatusApproved, StatusRejected, StatusCancelled:
	default:
		statusFilter = ""
	}
	search := strings.TrimSpace(q.Get("q"))
	from, to := parseDate(q.Get("from")), parseDate(q.Get("to"))
	fromValue, toValue := "", ""
	if from != nil {
		fromValue = from.Format("2006-01-02")
	}
	if to != nil {
		toValue = to.Format("2006-01-02")
	}

	claims, err := h.service.ListAll(r.Context(), ident.CompanyID, Filter{Status: statusFilter, Search: search, From: from, To: to})
	if err != nil {
		h.logger.Error("list expense claims", slog.Any("error", err))
	}

	s := shared.SessionFromContext(r.Context())
	token, _ := h.csrf.EnsureToken(r.Context(), s)
	if rerr := h.templates.Render(w, "pages/hr/expenses.html", view.TemplateData{
		Title:       "Expense Claims",
		CurrentPath: r.URL.Path,
		CSRFToken:   token,
		Flash:       s.PopFlash(),
		Data: map[string]any{
			"Claims":       claims,
			"StatusFilter": statusFilter,
			"Search":       search,
			"From":         fromValue,
			"To":           toValue,
			"Filtered":     statusFilter != "" || search != "" || fromValue != "" || toValue != "",
			"Error":        err,
		},
	}); rerr != nil {
		h.logger.Error("render expense claims page", slog.Any("error", rerr))
		shared.WriteHTTPError(w, http.StatusInternalServerError, "")
	}
}

func (h *Handler) retry(w http.ResponseWriter, r *http.Request) {
	ident, ok := shared.IdentityFromContext(r.Context())
	if !ok {
		shared.WriteHTTPError(w, http.StatusUnauthorized, shared.ErrUnauthorized.Error())
		return
	}
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	err := h.service.RetryApply(r.Context(), ident.CompanyID, id, ident.UserID)
	s := shared.SessionFromContext(r.Context())
	if err != nil {
		s.AddFlash(shared.FlashMessage{Kind: "error", Message: flashError(err)})
	} else {
		s.AddFlash(shared.FlashMessage{Kind: "success", Message: "Expense claim decision applied; check the claim status"})
	}
	http.Redirect(w, r, "/hr/expenses", http.StatusSeeOther)
}
