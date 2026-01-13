package users

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/odyssey-erp/odyssey-erp/internal/rbac"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/view"
)

// Handler manages user management endpoints.
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

// MountRoutes registers user routes.
func (h *Handler) MountRoutes(r chi.Router) {
	r.Group(func(r chi.Router) {
		// Backwards-compatible: older seeds use `rbac.view`/`rbac.edit` while the UI uses `users.*`.
		r.Use(h.rbac.RequireAny(shared.PermUsersView, "rbac.view"))
		r.Get("/", h.listUsers)
	})
	r.Group(func(r chi.Router) {
		r.Use(h.rbac.RequireAny(shared.PermUsersEdit, "rbac.edit"))
		r.Get("/new", h.showCreateUserForm)
		r.Post("/", h.createUser)
	})
}

type formErrors map[string]string

func (h *Handler) listUsers(w http.ResponseWriter, r *http.Request) {
	users, err := h.service.ListUsers(r.Context())
	if err != nil {
		h.logger.Error("list users failed", slog.Any("error", err))
		h.render(w, r, "pages/users/list.html", map[string]any{"Errors": formErrors{"general": shared.UserSafeMessage(err)}}, http.StatusInternalServerError)
		return
	}
	h.render(w, r, "pages/users/list.html", map[string]any{"Users": users}, http.StatusOK)
}

func (h *Handler) showCreateUserForm(w http.ResponseWriter, r *http.Request) {
	h.render(w, r, "pages/users/form.html", map[string]any{"Errors": formErrors{}}, http.StatusOK)
}

func (h *Handler) createUser(w http.ResponseWriter, r *http.Request) {
	// TODO: implement
	h.redirectWithFlash(w, r, "/users", "success", "User created")
}

func (h *Handler) render(w http.ResponseWriter, r *http.Request, template string, data map[string]any, status int) {
	sess := shared.SessionFromContext(r.Context())
	csrfToken, _ := h.csrf.EnsureToken(r.Context(), sess)
	var flash *shared.FlashMessage
	if sess != nil {
		flash = sess.PopFlash()
	}
	viewData := view.TemplateData{Title: "Users", CSRFToken: csrfToken, Flash: flash, CurrentPath: r.URL.Path, Data: data}
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
