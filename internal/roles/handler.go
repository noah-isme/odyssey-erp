package roles

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/odyssey-erp/odyssey-erp/internal/rbac"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/view"
)

// Handler manages role management endpoints.
type Handler struct {
	logger    *slog.Logger
	service   *Service
	templates *view.Engine
	csrf      *shared.CSRFManager
	sessions  *shared.SessionManager
	rbac      rbac.Middleware
}

// NewHandler builds Handler instance.
func NewHandler(logger *slog.Logger, service *Service, templates *view.Engine, csrf *shared.CSRFManager, sessions *shared.SessionManager, rbac rbac.Middleware) *Handler {
	return &Handler{logger: logger, service: service, templates: templates, csrf: csrf, sessions: sessions, rbac: rbac}
}

// MountRoutes registers role routes.
func (h *Handler) MountRoutes(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAny("roles.view"))
		r.Get("/", h.listRoles)
	})
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAll("roles.edit"))
		r.Get("/new", h.showCreateRoleForm)
		r.Post("/", h.createRole)
	})
}

type formErrors map[string]string

func (h *Handler) listRoles(w http.ResponseWriter, r *http.Request) {
	roles, err := h.service.ListRoles(r.Context())
	if err != nil {
		h.render(w, r, "pages/roles/list.html", map[string]any{"Errors": formErrors{"general": err.Error()}}, http.StatusInternalServerError)
		return
	}
	h.render(w, r, "pages/roles/list.html", map[string]any{"Roles": roles}, http.StatusOK)
}

func (h *Handler) showCreateRoleForm(w http.ResponseWriter, r *http.Request) {
	h.render(w, r, "pages/roles/form.html", map[string]any{"Errors": formErrors{}}, http.StatusOK)
}

func (h *Handler) createRole(w http.ResponseWriter, r *http.Request) {
	// TODO: implement
	h.redirectWithFlash(w, r, "/roles", "success", "Role created")
}

func (h *Handler) render(w http.ResponseWriter, r *http.Request, template string, data map[string]any, status int) {
	sess := shared.SessionFromContext(r.Context())
	csrfToken, _ := h.csrf.EnsureToken(r.Context(), sess)
	var flash *shared.FlashMessage
	if sess != nil {
		flash = sess.PopFlash()
	}
	viewData := view.TemplateData{Title: "Roles", CSRFToken: csrfToken, Flash: flash, CurrentPath: r.URL.Path, Data: data}
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
