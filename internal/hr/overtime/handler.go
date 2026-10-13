package overtime

import (
	"context"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/odyssey-erp/odyssey-erp/internal/rbac"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/view"
)

// HandlerService is the subset of *Service used by the HTTP handler.
type HandlerService interface {
	Submit(ctx context.Context, in SubmitInput) (Request, error)
	Cancel(ctx context.Context, companyID, requestID, userID int64) error
	ListOwn(ctx context.Context, companyID, userID int64) ([]Request, error)
	ListAll(ctx context.Context, companyID int64, f Filter) ([]Request, error)
	RetryApply(ctx context.Context, companyID, requestID, actorID int64) error
}

// Handler serves /hr/overtime.
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

// MountRoutes mounts /hr/overtime.
func (h *Handler) MountRoutes(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAny(shared.PermHROvertimeRequest, shared.PermHROvertimeAdmin))
		r.Get("/", h.list)
	})
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAny(shared.PermHROvertimeRequest))
		r.Post("/", h.submit)
		r.Post("/{id}/cancel", h.cancel)
	})
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAny(shared.PermHROvertimeAdmin))
		r.Post("/{id}/retry", h.retry)
	})
}

// permissions reports which overtime permissions the user holds.
func (h *Handler) permissions(r *http.Request, userID int64) (canRequest, isAdmin bool) {
	if h.rbac.Service == nil {
		return false, false
	}
	perms, err := h.rbac.Service.EffectivePermissions(r.Context(), userID)
	if err != nil {
		return false, false
	}
	for _, p := range perms {
		switch strings.ToLower(p) {
		case "admin", "*":
			return true, true
		case shared.PermHROvertimeRequest:
			canRequest = true
		case shared.PermHROvertimeAdmin:
			isAdmin = true
		}
	}
	return canRequest, isAdmin
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

func (h *Handler) redirect(w http.ResponseWriter, r *http.Request, to string, err error, success string) {
	s := shared.SessionFromContext(r.Context())
	if err != nil {
		s.AddFlash(shared.FlashMessage{Kind: "error", Message: flashError(err)})
	} else {
		s.AddFlash(shared.FlashMessage{Kind: "success", Message: success})
	}
	http.Redirect(w, r, to, http.StatusSeeOther)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	ident, ok := shared.IdentityFromContext(r.Context())
	if !ok {
		shared.WriteHTTPError(w, http.StatusUnauthorized, shared.ErrUnauthorized.Error())
		return
	}
	canRequest, isAdmin := h.permissions(r, ident.UserID)

	tab := strings.TrimSpace(r.URL.Query().Get("tab"))
	if tab != "all-requests" || !isAdmin {
		tab = "my-requests"
	}

	own, err := h.service.ListOwn(r.Context(), ident.CompanyID, ident.UserID)
	if err != nil {
		h.logger.Error("list own overtime", slog.Any("error", err))
	}
	statusFilter := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("status")))
	search := strings.TrimSpace(r.URL.Query().Get("q"))
	var all []Request
	if isAdmin {
		var allErr error
		all, allErr = h.service.ListAll(r.Context(), ident.CompanyID, Filter{Status: statusFilter, Search: search})
		if allErr != nil {
			h.logger.Error("list all overtime", slog.Any("error", allErr))
			if err == nil {
				err = allErr
			}
		}
	}

	s := shared.SessionFromContext(r.Context())
	token, _ := h.csrf.EnsureToken(r.Context(), s)
	if rerr := h.templates.Render(w, "pages/hr/overtime.html", view.TemplateData{
		Title:       "Overtime",
		CurrentPath: r.URL.Path,
		CSRFToken:   token,
		Flash:       s.PopFlash(),
		Data: map[string]any{
			"Requests":     own,
			"AllRequests":  all,
			"CanRequest":   canRequest,
			"IsAdmin":      isAdmin,
			"Tab":          tab,
			"StatusFilter": statusFilter,
			"Search":       search,
			"MaxHours":     MaxMinutes / 60,
			"Today":        time.Now().Format("2006-01-02"),
			"Error":        err,
		},
	}); rerr != nil {
		h.logger.Error("render overtime page", slog.Any("error", rerr))
		shared.WriteHTTPError(w, http.StatusInternalServerError, "")
	}
}

// parseHours converts an hours value ("1.5") to whole minutes; invalid input
// yields 0, which the service rejects.
func parseHours(v string) int {
	hours, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
	if err != nil || math.IsNaN(hours) || math.IsInf(hours, 0) || hours <= 0 || hours > 24 {
		return 0
	}
	return int(math.Round(hours * 60))
}

func (h *Handler) submit(w http.ResponseWriter, r *http.Request) {
	ident, ok := shared.IdentityFromContext(r.Context())
	if !ok {
		shared.WriteHTTPError(w, http.StatusUnauthorized, shared.ErrUnauthorized.Error())
		return
	}
	workedOn, _ := time.Parse("2006-01-02", strings.TrimSpace(r.FormValue("worked_on")))
	_, err := h.service.Submit(r.Context(), SubmitInput{
		UserID:    ident.UserID,
		CompanyID: ident.CompanyID,
		WorkedOn:  workedOn,
		Minutes:   parseHours(r.FormValue("hours")),
		Reason:    r.FormValue("reason"),
	})
	h.redirect(w, r, "/hr/overtime?tab=my-requests", err, "Overtime request submitted")
}

func (h *Handler) cancel(w http.ResponseWriter, r *http.Request) {
	ident, ok := shared.IdentityFromContext(r.Context())
	if !ok {
		shared.WriteHTTPError(w, http.StatusUnauthorized, shared.ErrUnauthorized.Error())
		return
	}
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	err := h.service.Cancel(r.Context(), ident.CompanyID, id, ident.UserID)
	h.redirect(w, r, "/hr/overtime?tab=my-requests", err, "Overtime request cancelled")
}

func (h *Handler) retry(w http.ResponseWriter, r *http.Request) {
	ident, ok := shared.IdentityFromContext(r.Context())
	if !ok {
		shared.WriteHTTPError(w, http.StatusUnauthorized, shared.ErrUnauthorized.Error())
		return
	}
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	err := h.service.RetryApply(r.Context(), ident.CompanyID, id, ident.UserID)
	h.redirect(w, r, "/hr/overtime?tab=all-requests", err, "Overtime decision applied; check the request status")
}
