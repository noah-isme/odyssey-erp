package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/httprate"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"github.com/pquerna/otp/totp"
	"golang.org/x/oauth2"

	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/sqlc"
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
	queries        *sqlc.Queries
}

// NewHandler constructs a Handler instance.
func NewHandler(logger *slog.Logger, service *Service, templates *view.Engine, sessions *shared.SessionManager, csrf *shared.CSRFManager, queries *sqlc.Queries) *Handler {
	return &Handler{
		logger:         logger,
		service:        service,
		templates:      templates,
		sessionManager: sessions,
		csrfManager:    csrf,
		validator:      validator.New(),
		queries:        queries,
	}
}

// MountRoutes registers auth routes on provided router.
func (h *Handler) MountRoutes(r chi.Router) {
	r.Get("/login", h.showLogin)
	r.Get("/sso/login", h.showSSOLogin)
	r.Get("/sso/callback", h.handleSSOCallback)
	r.Get("/mfa/verify", h.showMFAVerify)
	r.Get("/mfa/setup", h.showMFASetup)
	r.Group(func(r chi.Router) {
		r.Use(loginRateLimiter())
		r.Post("/login", h.handleLogin)
		r.Post("/mfa/verify", h.handleMFAVerify)
		r.Post("/mfa/setup", h.handleMFASetup)
	})
	r.Post("/logout", h.handleLogout)
}

func loginRateLimiter() func(http.Handler) http.Handler {
	return httprate.Limit(5, time.Minute,
		httprate.WithKeyFuncs(httprate.KeyByIP),
		httprate.WithLimitHandler(func(w http.ResponseWriter, _ *http.Request) {
			shared.WriteHTTPError(w, http.StatusTooManyRequests, "Terlalu banyak percobaan masuk. Coba lagi dalam satu menit.")
		}),
	)
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
		shared.WriteHTTPError(w, http.StatusInternalServerError, "")
	}
}

func (h *Handler) handleLogin(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		shared.WriteHTTPError(w, http.StatusBadRequest, "")
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
			if user.MFAEnabled {
				if sess != nil {
					sess.Set("mfa_pending_user_id", strconv.FormatInt(user.ID, 10))
				}
				http.Redirect(w, r, "/auth/mfa/verify", http.StatusSeeOther)
				return
			}
			
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

func (h *Handler) getOIDCConfig(ctx context.Context, connectionID int64, companyID int64) (*oauth2.Config, *oidc.Provider, error) {
	if h.queries == nil {
		return nil, nil, fmt.Errorf("auth: queries not initialized")
	}
	conn, err := h.queries.GetConnection(ctx, sqlc.GetConnectionParams{
		ID:        connectionID,
		CompanyID: companyID,
	})
	if err != nil {
		return nil, nil, err
	}
	if conn.Provider != "oidc" {
		return nil, nil, fmt.Errorf("connection is not oidc")
	}

	var config struct {
		Issuer       string `json:"issuer"`
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
	}
	if err := json.Unmarshal([]byte(conn.SecretRef), &config); err != nil {
		return nil, nil, err
	}

	provider, err := oidc.NewProvider(ctx, config.Issuer)
	if err != nil {
		return nil, nil, err
	}

	oauth2Config := &oauth2.Config{
		ClientID:     config.ClientID,
		ClientSecret: config.ClientSecret,
		Endpoint:     provider.Endpoint(),
		// We'll dynamically set the scheme in the handler to handle ngrok/https correctly, 
		// but default to standard here for simplicity.
		RedirectURL:  "http://localhost:8080/auth/sso/callback",
		Scopes:       []string{oidc.ScopeOpenID, "profile", "email"},
	}
	return oauth2Config, provider, nil
}

func (h *Handler) showSSOLogin(w http.ResponseWriter, r *http.Request) {
	companyID := int64(1)
	connIDStr := r.URL.Query().Get("connection_id")
	connID, _ := strconv.ParseInt(connIDStr, 10, 64)
	if connID == 0 {
		connID = 1 // Default to 1 for MVP
	}

	oauth2Config, _, err := h.getOIDCConfig(r.Context(), connID, companyID)
	if err != nil {
		h.logger.Error("failed to get oidc config", slog.Any("error", err))
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	oauth2Config.RedirectURL = fmt.Sprintf("%s://%s/auth/sso/callback", scheme, r.Host)

	state := uuid.New().String()
	sess := shared.SessionFromContext(r.Context())
	if sess != nil {
		sess.Set("oauth_state", state)
		sess.Set("oauth_company_id", fmt.Sprintf("%d", companyID))
		sess.Set("oauth_connection_id", fmt.Sprintf("%d", connID))
	}

	url := oauth2Config.AuthCodeURL(state)
	http.Redirect(w, r, url, http.StatusFound)
}

func (h *Handler) handleSSOCallback(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sess := shared.SessionFromContext(ctx)
	if sess == nil {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	state := r.URL.Query().Get("state")
	if sess.Get("oauth_state") != state || state == "" {
		h.logger.Error("invalid oauth state")
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	code := r.URL.Query().Get("code")

	companyIDStr := sess.Get("oauth_company_id")
	connIDStr := sess.Get("oauth_connection_id")
	companyID, _ := strconv.ParseInt(companyIDStr, 10, 64)
	connID, _ := strconv.ParseInt(connIDStr, 10, 64)

	oauth2Config, provider, err := h.getOIDCConfig(ctx, connID, companyID)
	if err != nil {
		h.logger.Error("failed to get oidc config", slog.Any("error", err))
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	oauth2Config.RedirectURL = fmt.Sprintf("%s://%s/auth/sso/callback", scheme, r.Host)

	oauth2Token, err := oauth2Config.Exchange(ctx, code)
	if err != nil {
		h.logger.Error("failed to exchange token", slog.Any("error", err))
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	rawIDToken, ok := oauth2Token.Extra("id_token").(string)
	if !ok {
		h.logger.Error("missing id token")
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	var verifier = provider.Verifier(&oidc.Config{ClientID: oauth2Config.ClientID})
	idToken, err := verifier.Verify(ctx, rawIDToken)
	if err != nil {
		h.logger.Error("failed to verify id token", slog.Any("error", err))
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	var claims struct {
		Email string `json:"email"`
	}
	if err := idToken.Claims(&claims); err != nil {
		h.logger.Error("failed to parse claims", slog.Any("error", err))
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	user, err := h.service.AuthenticateSSO(ctx, claims.Email)
	if err != nil {
		h.logger.Error("sso user not found", slog.Any("error", err), slog.String("email", claims.Email))
		sess.AddFlash(shared.FlashMessage{Kind: "error", Message: "Pengguna tidak ditemukan untuk akun SSO ini."})
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	if user.MFAEnabled {
		sess.Set("mfa_pending_user_id", strconv.FormatInt(user.ID, 10))
		http.Redirect(w, r, "/auth/mfa/verify", http.StatusSeeOther)
		return
	}

	sess.SetUser(fmt.Sprintf("%d", user.ID))
	h.service.RegisterSession(ctx, sess.ID, user.ID, time.Now().Add(24*time.Hour), r.RemoteAddr, r.UserAgent())
	
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

func (h *Handler) showMFAVerify(w http.ResponseWriter, r *http.Request) {
	sess := shared.SessionFromContext(r.Context())
	if sess == nil || sess.Get("mfa_pending_user_id") == "" {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}
	csrfToken, _ := h.csrfManager.EnsureToken(r.Context(), sess)
	viewData := view.TemplateData{
		Title:       "Verifikasi MFA",
		CSRFToken:   csrfToken,
		Flash:       sess.PopFlash(),
	}
	if err := h.templates.Render(w, "pages/mfa_verify.html", viewData); err != nil {
		h.logger.Error("render mfa verify", slog.Any("error", err))
		shared.WriteHTTPError(w, http.StatusInternalServerError, "")
	}
}

func (h *Handler) handleMFAVerify(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sess := shared.SessionFromContext(ctx)
	if sess == nil || sess.Get("mfa_pending_user_id") == "" {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	userIDStr := sess.Get("mfa_pending_user_id")
	userID, _ := strconv.ParseInt(userIDStr, 10, 64)
	
	code := r.FormValue("code")
	
	user, err := h.service.repo.FindByID(ctx, userID)
	if err != nil || !user.MFAEnabled {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	// Wait, we need to import "github.com/pquerna/otp/totp" in handler.go to validate
	valid := totp.Validate(code, user.TOTPSecret)
	if !valid {
		sess.AddFlash(shared.FlashMessage{Kind: "error", Message: "Kode OTP tidak valid."})
		http.Redirect(w, r, "/auth/mfa/verify", http.StatusSeeOther)
		return
	}

	// Login successful
	sess.Delete("mfa_pending_user_id")
	sess.SetUser(fmt.Sprintf("%d", user.ID))
	h.service.RegisterSession(ctx, sess.ID, user.ID, time.Now().Add(24*time.Hour), r.RemoteAddr, r.UserAgent())
	
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *Handler) showMFASetup(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sess := shared.SessionFromContext(ctx)
	if sess == nil || !sess.IsAuthenticated() {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	userID, _ := strconv.ParseInt(sess.UserID(), 10, 64)
	user, err := h.service.repo.FindByID(ctx, userID)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	if user.MFAEnabled {
		sess.AddFlash(shared.FlashMessage{Kind: "info", Message: "MFA sudah aktif."})
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	// Generate a new TOTP secret if not present in session
	secret := sess.Get("mfa_setup_secret")
	if secret == "" {
		key, err := totp.Generate(totp.GenerateOpts{
			Issuer:      "Odyssey ERP",
			AccountName: user.Email,
		})
		if err != nil {
			h.logger.Error("failed to generate totp", slog.Any("error", err))
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		secret = key.Secret()
		sess.Set("mfa_setup_secret", secret)
	}

	csrfToken, _ := h.csrfManager.EnsureToken(r.Context(), sess)
	viewData := view.TemplateData{
		Title:       "Setup MFA",
		CSRFToken:   csrfToken,
		Flash:       sess.PopFlash(),
		Data: map[string]any{
			"Secret": secret,
		},
	}
	if err := h.templates.Render(w, "pages/mfa_setup.html", viewData); err != nil {
		h.logger.Error("render mfa setup", slog.Any("error", err))
		shared.WriteHTTPError(w, http.StatusInternalServerError, "")
	}
}

func (h *Handler) handleMFASetup(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sess := shared.SessionFromContext(ctx)
	if sess == nil || !sess.IsAuthenticated() {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	secret := sess.Get("mfa_setup_secret")
	if secret == "" {
		http.Redirect(w, r, "/auth/mfa/setup", http.StatusSeeOther)
		return
	}

	code := r.FormValue("code")
	valid := totp.Validate(code, secret)
	if !valid {
		sess.AddFlash(shared.FlashMessage{Kind: "error", Message: "Kode OTP tidak valid. Silakan coba lagi."})
		http.Redirect(w, r, "/auth/mfa/setup", http.StatusSeeOther)
		return
	}

	// Save to DB
	userID, _ := strconv.ParseInt(sess.UserID(), 10, 64)
	if err := h.service.repo.UpdateMFA(ctx, userID, true, secret); err != nil {
		h.logger.Error("failed to update mfa", slog.Any("error", err))
		sess.AddFlash(shared.FlashMessage{Kind: "error", Message: "Gagal menyimpan MFA."})
		http.Redirect(w, r, "/auth/mfa/setup", http.StatusSeeOther)
		return
	}

	sess.Delete("mfa_setup_secret")
	sess.AddFlash(shared.FlashMessage{Kind: "success", Message: "MFA berhasil diaktifkan!"})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
