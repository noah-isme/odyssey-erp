package connectors

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/view"
)

type AdminHandler struct {
	service   *Service
	logger    *slog.Logger
	templates *view.Engine
}

func NewAdminHandler(service *Service, logger *slog.Logger, templates *view.Engine) *AdminHandler {
	return &AdminHandler{
		service:   service,
		logger:    logger,
		templates: templates,
	}
}

func (h *AdminHandler) MountSettingsRoutes(r chi.Router) {
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sess := shared.SessionFromContext(r.Context())
			if sess == nil || sess.User() == "" {
				http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
				return
			}
			next.ServeHTTP(w, r)
		})
	})

	r.Get("/", h.handleList)
	r.Post("/", h.handleCreate)
	r.Post("/{id}/status", h.handleUpdateStatus)
}

func (h *AdminHandler) handleList(w http.ResponseWriter, r *http.Request) {
	sess := shared.SessionFromContext(r.Context())
	companyIDStr := sess.Get("company_id")
	if companyIDStr == "" {
		h.logger.Error("no active company selected")
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	companyID, err := strconv.ParseInt(companyIDStr, 10, 64)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	connections, err := h.service.ListConnections(r.Context(), companyID)
	if err != nil {
		h.logger.Error("list connections", slog.Any("error", err))
		http.Error(w, "Failed to load connections", http.StatusInternalServerError)
		return
	}

	// We can still pass the available catalog of providers alongside the active connections.
	catalog := []map[string]any{
		{"Provider": "Stripe", "Type": "payment", "Icon": "💳", "Description": "Process payments and subscriptions."},
		{"Provider": "MockPay", "Type": "payment", "Icon": "💵", "Description": "Test payment gateway for sandbox environments."},
		{"Provider": "Shopify", "Type": "marketplace", "Icon": "🛍️", "Description": "Sync products, orders, and customers."},
		{"Provider": "WhatsApp", "Type": "messaging", "Icon": "💬", "Description": "Send notifications and chat with customers."},
		{"Provider": "OpenAI", "Type": "ai", "Icon": "🧠", "Description": "AI generation and automation features."},
		{"Provider": "DHL", "Type": "shipping", "Icon": "📦", "Description": "Book shipments and track deliveries."},
		{"Provider": "OIDC/SSO", "Type": "identity", "Icon": "🔐", "Description": "Single Sign-On and directory sync."},
	}

	_ = h.templates.Render(w, "pages/integrations.html", view.TemplateData{
		Title:     "Integrations",
		CSRFToken: shared.CSRFTokenFromContext(r.Context()),
		Data: map[string]any{
			"Connections": connections,
			"Catalog":     catalog,
		},
		Flash: sess.PopFlash(),
	})
}

func (h *AdminHandler) handleCreate(w http.ResponseWriter, r *http.Request) {
	sess := shared.SessionFromContext(r.Context())
	companyID, _ := strconv.ParseInt(sess.Get("company_id"), 10, 64)

	provider := r.FormValue("provider")
	connType := r.FormValue("type")
	name := r.FormValue("name")
	secret := r.FormValue("secret")

	if provider == "" || connType == "" || name == "" || secret == "" {
		sess.AddFlash(shared.FlashMessage{Kind: "error", Message: "All fields are required"})
		http.Redirect(w, r, "/settings/integrations", http.StatusSeeOther)
		return
	}

	_, err := h.service.CreateConnection(r.Context(), CreateConnectionParams{
		CompanyID:       companyID,
		Provider:        provider,
		Type:            connType,
		Name:            name,
		SecretPlaintext: secret,
	})

	if err != nil {
		h.logger.Error("create connection", slog.Any("error", err))
		sess.AddFlash(shared.FlashMessage{Kind: "error", Message: "Failed to create connection"})
	} else {
		sess.AddFlash(shared.FlashMessage{Kind: "success", Message: "Connection created successfully"})
	}

	http.Redirect(w, r, "/settings/integrations", http.StatusSeeOther)
}

func (h *AdminHandler) handleUpdateStatus(w http.ResponseWriter, r *http.Request) {
	sess := shared.SessionFromContext(r.Context())
	companyID, _ := strconv.ParseInt(sess.Get("company_id"), 10, 64)

	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	status := r.FormValue("status")
	if status == "" {
		http.Error(w, "Status is required", http.StatusBadRequest)
		return
	}

	_, err = h.service.UpdateConnectionStatus(r.Context(), companyID, id, status)
	if err != nil {
		h.logger.Error("update connection status", slog.Any("error", err))
		sess.AddFlash(shared.FlashMessage{Kind: "error", Message: "Failed to update connection status"})
	} else {
		sess.AddFlash(shared.FlashMessage{Kind: "success", Message: "Connection status updated"})
	}

	http.Redirect(w, r, "/settings/integrations", http.StatusSeeOther)
}
