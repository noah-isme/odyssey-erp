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
		// Backwards-compatible: older seeds use `rbac.view`/`rbac.edit` while the UI uses `roles.*`.
		r.Use(h.rbac.RequireAny(shared.PermRolesView, "rbac.view"))
		r.Get("/", h.listRoles)
	})
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAny(shared.PermRolesEdit, "rbac.edit"))
		r.Get("/new", h.showCreateRoleForm)
		r.Post("/", h.createRole)
	})
}

type formErrors map[string]string

func (h *Handler) listRoles(w http.ResponseWriter, r *http.Request) {
	filters := RoleListFilters{
		SortBy:  r.URL.Query().Get("sort"),
		SortDir: r.URL.Query().Get("dir"),
	}

	roles, err := h.service.ListRoles(r.Context(), filters)
	if err != nil {
		h.logger.Error("list roles failed", slog.Any("error", err))
		h.render(w, r, "pages/roles/list.html", map[string]any{"Errors": formErrors{"general": shared.UserSafeMessage(err)}}, http.StatusInternalServerError)
		return
	}
	h.render(w, r, "pages/roles/list.html", map[string]any{
		"Roles":   roles,
		"Filters": filters,
	}, http.StatusOK)
}

func (h *Handler) showCreateRoleForm(w http.ResponseWriter, r *http.Request) {
	h.render(w, r, "pages/roles/form.html", map[string]any{"Errors": formErrors{}}, http.StatusOK)
}

func (h *Handler) createRole(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.logger.Error("parse form", slog.Any("error", err))
		h.render(w, r, "pages/roles/form.html", map[string]any{"Errors": formErrors{"general": "Invalid request"}}, http.StatusBadRequest)
		return
	}

	name := r.PostFormValue("name")
	description := r.PostFormValue("description")

	_, err := h.service.CreateRole(r.Context(), name, description)
	if err != nil {
		h.logger.Error("create role failed", slog.Any("error", err))
		h.render(w, r, "pages/roles/form.html", map[string]any{
			"Errors": formErrors{"general": shared.UserSafeMessage(err)},
			"Role":   map[string]string{"Name": name, "Description": description},
		}, http.StatusBadRequest)
		return
	}

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
