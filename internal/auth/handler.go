package auth

import (
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"

	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/view"
)

// Handler wires HTTP endpoints for authentication flows.
type Handler struct {
	logger         *slog.Logger
	service        *Service
	templates      *view.Engine
	sessionManager *shared.SessionManager
	csrfManager    *shared.CSRFManager
	validator      *validator.Validate
}

// NewHandler constructs a Handler instance.
func NewHandler(logger *slog.Logger, service *Service, templates *view.Engine, sessions *shared.SessionManager, csrf *shared.CSRFManager) *Handler {
	return &Handler{
		logger:         logger,
		service:        service,
		templates:      templates,
		sessionManager: sessions,
		csrfManager:    csrf,
		validator:      validator.New(),
	}
}

// MountRoutes registers auth routes on provided router.
func (h *Handler) MountRoutes(r chi.Router) {
	r.Get("/login", h.showLogin)
	r.Post("/login", h.handleLogin)
	r.Post("/logout", h.handleLogout)
}

type loginForm struct {
	Email    string `validate:"required,email"`
	Password string `validate:"required,min=8"`
}

type loginPageData struct {
	Form   loginForm
	Errors map[string]string
}

func (h *Handler) showLogin(w http.ResponseWriter, r *http.Request) {
	sess := shared.SessionFromContext(r.Context())
	csrfToken, _ := h.csrfManager.EnsureToken(r.Context(), sess)
	data := loginPageData{Form: loginForm{}}
	var flash *shared.FlashMessage
	if sess != nil {
		flash = sess.PopFlash()
	}
	viewData := view.TemplateData{
		Title:       "Masuk",
		CSRFToken:   csrfToken,
		Flash:       flash,
		CurrentPath: r.URL.Path,
		Data:        data,
	}
	if err := h.templates.Render(w, "pages/login.html", viewData); err != nil {
		h.logger.Error("render login", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
	}
}

func (h *Handler) handleLogin(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	sess := shared.SessionFromContext(r.Context())
	csrfToken, _ := h.csrfManager.EnsureToken(r.Context(), sess)

	form := loginForm{
		Email:    r.PostFormValue("email"),
		Password: r.PostFormValue("password"),
	}
	errors := make(map[string]string)
	if err := h.validator.Struct(form); err != nil {
		for _, fieldErr := range err.(validator.ValidationErrors) {
			errors[fieldErr.Field()] = fieldErr.Error()
		}
	}

	if len(errors) == 0 {
		user, err := h.service.Authenticate(r.Context(), form.Email, form.Password)
		if err != nil {
			errors["general"] = "Email atau password tidak valid"
		} else {
			if sess != nil {
				sess.SetUser(strconv.FormatInt(user.ID, 10))
				sess.AddFlash(shared.FlashMessage{Kind: "success", Message: "Selamat datang kembali"})
			}
			expiresAt := time.Now().Add(h.sessionManager.TTL())
			sessionID := ""
			if sess != nil {
				sessionID = sess.ID
			}
			if sessionID != "" {
				if err := h.service.RegisterSession(r.Context(), sessionID, user.ID, expiresAt, r.RemoteAddr, r.UserAgent()); err != nil {
					h.logger.Warn("register session", slog.Any("error", err))
				}
			}
			if sess == nil {
				h.logger.Error("session missing during login")
			}
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
	}

	data := loginPageData{Form: form, Errors: errors}
	var flash *shared.FlashMessage
	if sess != nil {
		flash = sess.PopFlash()
	}
	viewData := view.TemplateData{
		Title:       "Masuk",
		CSRFToken:   csrfToken,
		Flash:       flash,
		CurrentPath: r.URL.Path,
		Data:        data,
	}
	w.WriteHeader(http.StatusBadRequest)
	if err := h.templates.Render(w, "pages/login.html", viewData); err != nil {
		h.logger.Error("render login invalid", slog.Any("error", err))
	}
}

func (h *Handler) handleLogout(w http.ResponseWriter, r *http.Request) {
	sess := shared.SessionFromContext(r.Context())
	if sess != nil {
		if err := h.service.RemoveSession(r.Context(), sess.ID); err != nil {
			h.logger.Warn("remove session", slog.Any("error", err))
		}
		h.sessionManager.Destroy(sess)
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// ShowLoginForTest exposes the GET handler for tests.
func (h *Handler) ShowLoginForTest(w http.ResponseWriter, r *http.Request) {
	h.showLogin(w, r)
}

// HandleLoginForTest exposes the POST handler for tests.
func (h *Handler) HandleLoginForTest(w http.ResponseWriter, r *http.Request) {
	h.handleLogin(w, r)
}
