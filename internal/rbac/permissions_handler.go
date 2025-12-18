package rbac

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/view"
)

// PermissionsHandler manages permission listing.
type PermissionsHandler struct {
	logger    *slog.Logger
	service   *Service
	templates *view.Engine
	csrf      *shared.CSRFManager
	sessions  *shared.SessionManager
	rbac      Middleware
}

// NewPermissionsHandler builds PermissionsHandler instance.
func NewPermissionsHandler(logger *slog.Logger, service *Service, templates *view.Engine, csrf *shared.CSRFManager, sessions *shared.SessionManager, rbac Middleware) *PermissionsHandler {
	return &PermissionsHandler{logger: logger, service: service, templates: templates, csrf: csrf, sessions: sessions, rbac: rbac}
}

// MountRoutes registers permission routes.
func (h *PermissionsHandler) MountRoutes(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAny("permissions.view"))
		r.Get("/", h.listPermissions)
	})
}

type formErrors map[string]string

func (h *PermissionsHandler) listPermissions(w http.ResponseWriter, r *http.Request) {
	perms, err := h.service.ListPermissions(r.Context())
	if err != nil {
		h.render(w, r, "pages/permissions/list.html", map[string]any{"Errors": formErrors{"general": err.Error()}}, http.StatusInternalServerError)
		return
	}
	h.render(w, r, "pages/permissions/list.html", map[string]any{"Permissions": perms}, http.StatusOK)
}

func (h *PermissionsHandler) render(w http.ResponseWriter, r *http.Request, template string, data map[string]any, status int) {
	sess := shared.SessionFromContext(r.Context())
	csrfToken, _ := h.csrf.EnsureToken(r.Context(), sess)
	var flash *shared.FlashMessage
	if sess != nil {
		flash = sess.PopFlash()
	}
	viewData := view.TemplateData{Title: "Permissions", CSRFToken: csrfToken, Flash: flash, CurrentPath: r.URL.Path, Data: data}
	w.WriteHeader(status)
	if err := h.templates.Render(w, template, viewData); err != nil {
		h.logger.Error("render template", slog.Any("error", err))
	}
}
