package treasury

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/odyssey-erp/odyssey-erp/internal/rbac"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/view"
)

// OperationsHandler renders the company-scoped payment operations workbench.
// It is separate from the existing JSON treasury lifecycle handler so read
// models and operator recovery actions do not couple templates to pgx or the
// batch proposal service.
type OperationsHandler struct {
	logger    *slog.Logger
	service   *OperationsService
	templates *view.Engine
	csrf      *shared.CSRFManager
	rbac      rbac.Middleware
}

func NewOperationsHandler(logger *slog.Logger, service *OperationsService, templates *view.Engine, csrf *shared.CSRFManager, middleware rbac.Middleware) *OperationsHandler {
	return &OperationsHandler{logger: logger, service: service, templates: templates, csrf: csrf, rbac: middleware}
}

func (h *OperationsHandler) MountRoutes(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAny(shared.PermFinancePaymentView))
		r.Get("/", h.list)
		r.Get("/{instruction_id}", h.detail)
	})
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAny(shared.PermFinancePaymentExecute))
		r.Post("/{instruction_id}/recover", h.recover)
		r.Post("/results/{result_id}/retry-effects", h.retryEffects)
	})
}

func (h *OperationsHandler) list(w http.ResponseWriter, r *http.Request) {
	identity, ok := shared.IdentityFromContext(r.Context())
	if !ok {
		shared.WriteHTTPError(w, http.StatusUnauthorized, "")
		return
	}
	filter := OperationsFilter{
		State: r.URL.Query().Get("state"),
		Query: r.URL.Query().Get("q"),
	}
	operations, err := h.service.List(r.Context(), identity.CompanyID, filter)
	if err != nil {
		h.renderError(w, r, http.StatusInternalServerError, err)
		return
	}
	h.render(w, r, "pages/finance/payment_operations.html", "Payment operations", map[string]any{
		"Operations": operations,
		"Filter":     filter,
	})
}

func (h *OperationsHandler) detail(w http.ResponseWriter, r *http.Request) {
	identity, ok := shared.IdentityFromContext(r.Context())
	if !ok {
		shared.WriteHTTPError(w, http.StatusUnauthorized, "")
		return
	}
	instructionID := strings.TrimSpace(chi.URLParam(r, "instruction_id"))
	operation, err := h.service.Get(r.Context(), identity.CompanyID, instructionID)
	if errors.Is(err, ErrOperationsNotFound) {
		h.renderError(w, r, http.StatusNotFound, err)
		return
	}
	if err != nil {
		h.renderError(w, r, http.StatusInternalServerError, err)
		return
	}
	h.render(w, r, "pages/finance/payment_operation_detail.html", "Payment operation", map[string]any{
		"Operation": operation,
	})
}

func (h *OperationsHandler) recover(w http.ResponseWriter, r *http.Request) {
	identity, ok := shared.IdentityFromContext(r.Context())
	if !ok {
		shared.WriteHTTPError(w, http.StatusUnauthorized, "")
		return
	}
	err := h.service.Recover(r.Context(), identity.CompanyID, chi.URLParam(r, "instruction_id"), identity.UserID)
	if err != nil {
		h.flashRedirect(w, r, "/finance/treasury/operations/"+urlPathSegment(chi.URLParam(r, "instruction_id")), "error", shared.UserSafeMessage(err))
		return
	}
	h.flashRedirect(w, r, "/finance/treasury/operations/"+urlPathSegment(chi.URLParam(r, "instruction_id")), "success", "Recovery queued; provider status will be checked before any resubmission")
}

func (h *OperationsHandler) retryEffects(w http.ResponseWriter, r *http.Request) {
	identity, ok := shared.IdentityFromContext(r.Context())
	if !ok {
		shared.WriteHTTPError(w, http.StatusUnauthorized, "")
		return
	}
	err := h.service.RetryEffects(r.Context(), identity.CompanyID, chi.URLParam(r, "result_id"), identity.UserID)
	if err != nil {
		h.flashRedirect(w, r, "/finance/treasury/operations", "error", shared.UserSafeMessage(err))
		return
	}
	h.flashRedirect(w, r, "/finance/treasury/operations", "success", "Settlement effects retry queued")
}

func (h *OperationsHandler) render(w http.ResponseWriter, r *http.Request, name, title string, data any) {
	if h == nil || h.templates == nil {
		h.renderError(w, r, http.StatusInternalServerError, errors.New("operations templates are not configured"))
		return
	}
	sess := shared.SessionFromContext(r.Context())
	var token string
	var flash *shared.FlashMessage
	if sess != nil {
		flash = sess.PopFlash()
		if h.csrf != nil {
			token, _ = h.csrf.EnsureToken(r.Context(), sess)
		}
	}
	if err := h.templates.Render(w, name, view.TemplateData{Title: title, CurrentPath: r.URL.Path, CSRFToken: token, Flash: flash, Data: data}); err != nil {
		h.renderError(w, r, http.StatusInternalServerError, err)
	}
}

func (h *OperationsHandler) renderError(w http.ResponseWriter, _ *http.Request, status int, err error) {
	if h != nil && h.logger != nil && err != nil {
		h.logger.Error("payment operations request failed", slog.Any("error", err))
	}
	http.Error(w, http.StatusText(status), status)
}

func (h *OperationsHandler) flashRedirect(w http.ResponseWriter, r *http.Request, path, kind, message string) {
	if sess := shared.SessionFromContext(r.Context()); sess != nil {
		sess.AddFlash(shared.FlashMessage{Kind: kind, Message: message})
	}
	http.Redirect(w, r, path, http.StatusSeeOther)
}

func urlPathSegment(value string) string {
	// Instruction and result IDs are generated by the application and do not
	// contain slashes. Keep the helper defensive for manually supplied IDs.
	return strings.ReplaceAll(strings.TrimSpace(value), "/", "")
}
