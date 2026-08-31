package app

import (
	"context"
	"encoding/json"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

	"github.com/odyssey-erp/odyssey-erp/internal/accounting"
	analytichttp "github.com/odyssey-erp/odyssey-erp/internal/analytics/http"
	"github.com/odyssey-erp/odyssey-erp/internal/ap"
	apihttp "github.com/odyssey-erp/odyssey-erp/internal/api"
	"github.com/odyssey-erp/odyssey-erp/internal/approvals"
	"github.com/odyssey-erp/odyssey-erp/internal/ar"
	audithttp "github.com/odyssey-erp/odyssey-erp/internal/audit/http"
	auth "github.com/odyssey-erp/odyssey-erp/internal/auth"
	boardpackhttp "github.com/odyssey-erp/odyssey-erp/internal/boardpack/http"
	closehttp "github.com/odyssey-erp/odyssey-erp/internal/close/http"
	cmmshttp "github.com/odyssey-erp/odyssey-erp/internal/cmms/http"
	"github.com/odyssey-erp/odyssey-erp/internal/connectors"
	consolhttp "github.com/odyssey-erp/odyssey-erp/internal/consol/http"
	"github.com/odyssey-erp/odyssey-erp/internal/crm"
	"github.com/odyssey-erp/odyssey-erp/internal/dashboard"
	"github.com/odyssey-erp/odyssey-erp/internal/delivery"
	documentshttp "github.com/odyssey-erp/odyssey-erp/internal/documents/http"
	eliminationhttp "github.com/odyssey-erp/odyssey-erp/internal/elimination/http"
	"github.com/odyssey-erp/odyssey-erp/internal/finance/bankfeeds"
	"github.com/odyssey-erp/odyssey-erp/internal/finance/banking"
	"github.com/odyssey-erp/odyssey-erp/internal/finance/forecasting"
	"github.com/odyssey-erp/odyssey-erp/internal/finance/treasury"
	"github.com/odyssey-erp/odyssey-erp/internal/freight"
	hrattendance "github.com/odyssey-erp/odyssey-erp/internal/hr/attendance"
	hremployees "github.com/odyssey-erp/odyssey-erp/internal/hr/employees"
	hrleave "github.com/odyssey-erp/odyssey-erp/internal/hr/leave"
	insightshhtp "github.com/odyssey-erp/odyssey-erp/internal/insights/http"
	"github.com/odyssey-erp/odyssey-erp/internal/inventory"
	"github.com/odyssey-erp/odyssey-erp/internal/logistics"
	"github.com/odyssey-erp/odyssey-erp/internal/masterdata"
	"github.com/odyssey-erp/odyssey-erp/internal/mrp"
	"github.com/odyssey-erp/odyssey-erp/internal/notifications"
	"github.com/odyssey-erp/odyssey-erp/internal/observability"
	"github.com/odyssey-erp/odyssey-erp/internal/payroll"
	"github.com/odyssey-erp/odyssey-erp/internal/portal"
	"github.com/odyssey-erp/odyssey-erp/internal/pos"
	"github.com/odyssey-erp/odyssey-erp/internal/procurement"
	"github.com/odyssey-erp/odyssey-erp/internal/projects"
	qmshttp "github.com/odyssey-erp/odyssey-erp/internal/qms/http"
	"github.com/odyssey-erp/odyssey-erp/internal/rbac"
	"github.com/odyssey-erp/odyssey-erp/internal/roles"
	"github.com/odyssey-erp/odyssey-erp/internal/sales"
	"github.com/odyssey-erp/odyssey-erp/internal/search"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/tax"
	"github.com/odyssey-erp/odyssey-erp/internal/users"
	"github.com/odyssey-erp/odyssey-erp/internal/variance"
	"github.com/odyssey-erp/odyssey-erp/internal/view"
	"github.com/odyssey-erp/odyssey-erp/internal/wms"
	"github.com/odyssey-erp/odyssey-erp/jobs"
	"github.com/odyssey-erp/odyssey-erp/report"
	"github.com/odyssey-erp/odyssey-erp/web"
)

// RouterParams groups dependencies for building the HTTP router.
type RouterParams struct {
	Logger             *slog.Logger
	Config             *Config
	Templates          *view.Engine
	SessionManager     *shared.SessionManager
	CSRFManager        *shared.CSRFManager
	AuthHandler        *auth.Handler
	AccountingHandler  *accounting.Handler
	ARHandler          *ar.Handler
	RolesHandler       *roles.Handler
	UsersHandler       *users.Handler
	CloseHandler       *closehttp.Handler
	EliminationHandler *eliminationhttp.Handler
	VarianceHandler    *variance.Handler
	InsightsHandler    *insightshhtp.Handler
	AuditHandler       *audithttp.Handler
	InventoryHandler   *inventory.Handler
	ProcurementHandler *procurement.Handler
	SalesHandler       *sales.Handler
	MasterDataHandler  *masterdata.Handler
	APHandler          *ap.Handler
	Pool               *pgxpool.Pool
	RBACMiddleware     rbac.Middleware

	ReportHandler          *report.Handler
	BoardPackHandler       *boardpackhttp.Handler
	JobHandler             *jobs.Handler
	AnalyticsHandler       *analytichttp.Handler
	ConsolHandler          *consolhttp.Handler
	PermissionsHandler     *rbac.PermissionsHandler
	Metrics                *observability.Metrics
	DashboardHandler       *dashboard.Handler
	SearchHandler          *search.Handler
	BankingHandler         *banking.Handler
	BankFeedsHandler       *bankfeeds.Handler
	ForecastingHandler     *forecasting.Handler
	TreasuryHandler        *treasury.Handler
	InventoryService       *inventory.Service
	NotificationHandler    *notifications.Handler
	NotificationDispatcher *notifications.Dispatcher
	ApprovalsHandler       *approvals.Handler
	HREmployeesHandler     *hremployees.Handler
	HRLeaveHandler         *hrleave.Handler
	HRAttendanceHandler    *hrattendance.Handler
	PayrollHandler         *payroll.Handler
	TaxHandler             *tax.Handler
	CRMHandler             *crm.Handler
	WMSHandler             *wms.Handler
	APIHandler             *apihttp.Handler
	PortalHandler          *portal.Handler
	POSHandler             *pos.Handler
	ProjectsHandler        *projects.Handler
	MRPHandler             *mrp.Handler
	DocumentsHandler       *documentshttp.Handler
	CMMSHandler            *cmmshttp.Handler
	QMSHandler             *qmshttp.Handler
	ConnectorsHandler      *connectors.WebhookHandler
	ConnectorsAdminHandler *connectors.AdminHandler
	LogisticsService       *logistics.Service
	FreightService         freight.Service
}

type workspaceUser struct {
	ID            int64     `json:"id"`
	Email         string    `json:"email"`
	Name          string    `json:"name"`
	IsActive      bool      `json:"isActive"`
	CreatedAt     time.Time `json:"createdAt"`
	Theme         string    `json:"theme"`
	Language      string    `json:"language"`
	Notifications bool      `json:"notifications"`
}

type workspaceCompany struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

func loadWorkspaceUser(ctx context.Context, pool *pgxpool.Pool, id int64) (workspaceUser, error) {
	var user workspaceUser
	err := pool.QueryRow(ctx, `SELECT id, email, name, is_active, created_at, ui_theme, ui_language, ui_notifications
		FROM users WHERE id = $1`, id).Scan(
		&user.ID, &user.Email, &user.Name, &user.IsActive, &user.CreatedAt,
		&user.Theme, &user.Language, &user.Notifications,
	)
	if user.Language == "" || user.Language == "bilingual" {
		user.Language = "id"
	}
	return user, err
}

func loadWorkspaceCompanies(ctx context.Context, pool *pgxpool.Pool) ([]workspaceCompany, error) {
	rows, err := pool.Query(ctx, "SELECT id, name FROM companies ORDER BY name, id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	companies := make([]workspaceCompany, 0)
	for rows.Next() {
		var company workspaceCompany
		if err := rows.Scan(&company.ID, &company.Name); err != nil {
			return nil, err
		}
		companies = append(companies, company)
	}
	return companies, rows.Err()
}

func activeCompanyMiddleware(pool *pgxpool.Pool, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sess := shared.SessionFromContext(r.Context())
			if pool != nil && sess != nil && sess.User() != "" && sess.Get("company_id") == "" {
				var companyID int64
				if err := pool.QueryRow(r.Context(), "SELECT id FROM companies ORDER BY id LIMIT 1").Scan(&companyID); err == nil {
					sess.Set("company_id", strconv.FormatInt(companyID, 10))
				} else if logger != nil {
					logger.Warn("initialize active company", slog.Any("error", err))
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

func redirectBackToWorkspace(w http.ResponseWriter, r *http.Request) {
	target := "/"
	if referer, err := url.Parse(r.Referer()); err == nil && referer.Path != "" && (referer.Host == "" || referer.Host == r.Host) {
		target = referer.RequestURI()
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}

// NewRouter constructs the chi.Router with Odyssey defaults.
func NewRouter(params RouterParams) http.Handler {
	r := chi.NewRouter()

	if params.Config != nil {
		// Apply the release boundary before session, company, and auth work so
		// unsupported preview routes cannot trigger any downstream side effects.
		r.Use(ReleaseProfileMiddleware(params.Config.ReleaseProfile))
	}

	for _, mw := range MiddlewareStack(MiddlewareConfig{
		Logger:         params.Logger,
		Config:         params.Config,
		SessionManager: params.SessionManager,
		CSRFManager:    params.CSRFManager,
		Metrics:        params.Metrics,
	}) {
		r.Use(mw)
	}

	r.Use(chimw.Logger)
	r.Use(activeCompanyMiddleware(params.Pool, params.Logger))

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if params.Pool != nil {
			ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
			defer cancel()
			if err := params.Pool.Ping(ctx); err != nil {
				if params.Logger != nil {
					params.Logger.Error("health check postgres", slog.Any("error", err))
				}
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = w.Write([]byte(`{"status":"unavailable"}`))
				return
			}
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	// Landing page for unauthenticated users
	r.Get("/welcome", func(w http.ResponseWriter, r *http.Request) {
		sess := shared.SessionFromContext(r.Context())
		csrfToken, _ := params.CSRFManager.EnsureToken(r.Context(), sess)
		var flash *shared.FlashMessage
		if sess != nil {
			flash = sess.PopFlash()
		}
		data := view.TemplateData{
			Title:     "Odyssey ERP",
			CSRFToken: csrfToken,
			Flash:     flash,
		}
		if err := params.Templates.Render(w, "pages/landing.html", data); err != nil {
			params.Logger.Error("render landing", slog.Any("error", err))
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		}
	})

	// Public legal and compliance documents
	renderLegal := func(w http.ResponseWriter, r *http.Request, topic, title, currentPath string) {
		sess := shared.SessionFromContext(r.Context())
		csrfToken, _ := params.CSRFManager.EnsureToken(r.Context(), sess)
		var flash *shared.FlashMessage
		if sess != nil {
			flash = sess.PopFlash()
		}
		data := view.TemplateData{
			Title:       title,
			CSRFToken:   csrfToken,
			Flash:       flash,
			CurrentPath: currentPath,
			Data: map[string]any{
				"Topic":         topic,
				"EffectiveDate": "1 Januari 2026",
				"Version":       "2026.2",
			},
		}
		if err := params.Templates.Render(w, "pages/legal.html", data); err != nil {
			params.Logger.Error("render legal", slog.String("topic", topic), slog.Any("error", err))
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		}
	}

	r.Route("/legal", func(r chi.Router) {
		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/legal/privacy", http.StatusMovedPermanently)
		})
		r.Get("/privacy", func(w http.ResponseWriter, r *http.Request) {
			renderLegal(w, r, "privacy", "Kebijakan Privasi · Odyssey ERP", "/legal/privacy")
		})
		r.Get("/terms", func(w http.ResponseWriter, r *http.Request) {
			renderLegal(w, r, "terms", "Syarat & Ketentuan Layanan · Odyssey ERP", "/legal/terms")
		})
		r.Get("/security", func(w http.ResponseWriter, r *http.Request) {
			renderLegal(w, r, "security", "Pernyataan Keamanan Sistem · Odyssey ERP", "/legal/security")
		})
	})

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		sess := shared.SessionFromContext(r.Context())

		// Redirect to landing page if not authenticated
		if sess == nil || sess.User() == "" {
			http.Redirect(w, r, "/welcome", http.StatusSeeOther)
			return
		}

		csrfToken, _ := params.CSRFManager.EnsureToken(r.Context(), sess)
		var flash *shared.FlashMessage
		if sess != nil {
			flash = sess.PopFlash()
		}
		appEnv := ""
		if params.Config != nil {
			appEnv = params.Config.AppEnv
		}
		data := view.TemplateData{
			Title:     "Odyssey ERP",
			CSRFToken: csrfToken,
			Flash:     flash,
			Data: map[string]any{
				"AppEnv": appEnv,
			},
		}
		if err := params.Templates.Render(w, "pages/home.html", data); err != nil {
			params.Logger.Error("render home", slog.Any("error", err))
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		}
	})

	if params.AuthHandler != nil {
		r.Route("/auth", params.AuthHandler.MountRoutes)
	}
	// Personal workspace pages and preferences.
	r.Get("/profile", func(w http.ResponseWriter, r *http.Request) {
		sess := shared.SessionFromContext(r.Context())
		if sess == nil || sess.User() == "" {
			http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
			return
		}
		id, err := strconv.ParseInt(sess.User(), 10, 64)
		if err != nil || params.Pool == nil {
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
		user, err := loadWorkspaceUser(r.Context(), params.Pool, id)
		if err != nil {
			http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
			return
		}
		csrfToken, _ := params.CSRFManager.EnsureToken(r.Context(), sess)
		_ = params.Templates.Render(w, "pages/profile.html", view.TemplateData{Title: "Profil", CSRFToken: csrfToken, Flash: sess.PopFlash(), Data: map[string]any{"User": user}})
	})
	r.Post("/profile", func(w http.ResponseWriter, r *http.Request) {
		sess := shared.SessionFromContext(r.Context())
		if sess == nil || sess.User() == "" {
			http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
			return
		}
		id, err := strconv.ParseInt(sess.User(), 10, 64)
		name := strings.TrimSpace(r.PostFormValue("name"))
		if err != nil || name == "" || params.Pool == nil {
			sess.AddFlash(shared.FlashMessage{Kind: "error", Message: "Nama wajib diisi"})
			http.Redirect(w, r, "/profile", http.StatusSeeOther)
			return
		}
		if _, err = params.Pool.Exec(r.Context(), "UPDATE users SET name = $1, updated_at = NOW() WHERE id = $2", name, id); err != nil {
			sess.AddFlash(shared.FlashMessage{Kind: "error", Message: "Profil tidak dapat diperbarui"})
		} else {
			sess.Set("user.name", name)
			sess.AddFlash(shared.FlashMessage{Kind: "success", Message: "Profil berhasil diperbarui"})
		}
		http.Redirect(w, r, "/profile", http.StatusSeeOther)
	})
	r.Get("/settings", func(w http.ResponseWriter, r *http.Request) {
		sess := shared.SessionFromContext(r.Context())
		if sess == nil || sess.User() == "" {
			http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
			return
		}
		id, err := strconv.ParseInt(sess.User(), 10, 64)
		if err != nil || params.Pool == nil {
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
		user, err := loadWorkspaceUser(r.Context(), params.Pool, id)
		if err != nil {
			http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
			return
		}
		csrfToken, _ := params.CSRFManager.EnsureToken(r.Context(), sess)
		_ = params.Templates.Render(w, "pages/settings.html", view.TemplateData{Title: "Pengaturan", CSRFToken: csrfToken, Flash: sess.PopFlash(), Data: map[string]any{"User": user}})
	})

	if params.ConnectorsAdminHandler != nil {
		r.Route("/settings/integrations", params.ConnectorsAdminHandler.MountSettingsRoutes)
	} else {
		r.Get("/settings/integrations", func(w http.ResponseWriter, r *http.Request) {
			sess := shared.SessionFromContext(r.Context())
			if sess == nil || sess.User() == "" {
				http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
				return
			}

			providers := []map[string]any{
				{"Provider": "Stripe", "Name": "Stripe", "Icon": "credit-card", "Description": "Process payments and subscriptions.", "Active": true},
				{"Provider": "MockPay", "Name": "MockPay", "Icon": "banknote", "Description": "Test payment gateway for sandbox environments.", "Active": true},
				{"Provider": "Shopify", "Name": "Shopify", "Icon": "shopping-bag", "Description": "Sync products, orders, and customers.", "Active": false},
				{"Provider": "WhatsApp", "Name": "WhatsApp", "Icon": "message-circle", "Description": "Send notifications and chat with customers.", "Active": false},
				{"Provider": "OpenAI", "Name": "OpenAI", "Icon": "cpu", "Description": "AI generation and automation features.", "Active": false},
				{"Provider": "DHL", "Name": "DHL", "Icon": "truck", "Description": "Book shipments and track deliveries.", "Active": false},
				{"Provider": "OIDC/SSO", "Name": "OIDC/SSO", "Icon": "shield-check", "Description": "Single Sign-On and directory sync.", "Active": true},
			}

			_ = params.Templates.Render(w, "pages/integrations.html", view.TemplateData{
				Title: "Integrations",
				Data:  map[string]any{"Providers": providers},
			})
		})
	}

	// Module UI Frontend Endpoints
	r.Get("/pos/terminal", func(w http.ResponseWriter, r *http.Request) {
		sess := shared.SessionFromContext(r.Context())
		var csrfToken string
		if sess != nil && params.CSRFManager != nil {
			csrfToken, _ = params.CSRFManager.EnsureToken(r.Context(), sess)
		}
		_ = params.Templates.Render(w, "pages/pos/terminal.html", view.TemplateData{Title: "POS Terminal", CSRFToken: csrfToken, CurrentPath: "/pos/terminal"})
	})
	r.Get("/cmms/dashboard", func(w http.ResponseWriter, r *http.Request) {
		sess := shared.SessionFromContext(r.Context())
		var csrfToken string
		if sess != nil && params.CSRFManager != nil {
			csrfToken, _ = params.CSRFManager.EnsureToken(r.Context(), sess)
		}
		_ = params.Templates.Render(w, "pages/cmms/dashboard.html", view.TemplateData{Title: "CMMS Dashboard", CSRFToken: csrfToken, CurrentPath: "/cmms/dashboard"})
	})
	r.Get("/qms/dashboard", func(w http.ResponseWriter, r *http.Request) {
		sess := shared.SessionFromContext(r.Context())
		var csrfToken string
		if sess != nil && params.CSRFManager != nil {
			csrfToken, _ = params.CSRFManager.EnsureToken(r.Context(), sess)
		}
		_ = params.Templates.Render(w, "pages/qms/dashboard.html", view.TemplateData{Title: "QMS Dashboard", CSRFToken: csrfToken, CurrentPath: "/qms/dashboard"})
	})
	r.Get("/documents/workspace", func(w http.ResponseWriter, r *http.Request) {
		sess := shared.SessionFromContext(r.Context())
		var csrfToken string
		if sess != nil && params.CSRFManager != nil {
			csrfToken, _ = params.CSRFManager.EnsureToken(r.Context(), sess)
		}
		_ = params.Templates.Render(w, "pages/documents/workspace.html", view.TemplateData{Title: "Document Workspace", CSRFToken: csrfToken, CurrentPath: "/documents/workspace"})
	})
	r.Get("/wms/operations", func(w http.ResponseWriter, r *http.Request) {
		sess := shared.SessionFromContext(r.Context())
		var csrfToken string
		if sess != nil && params.CSRFManager != nil {
			csrfToken, _ = params.CSRFManager.EnsureToken(r.Context(), sess)
		}
		_ = params.Templates.Render(w, "pages/wms/operations.html", view.TemplateData{Title: "WMS Operations", CSRFToken: csrfToken, CurrentPath: "/wms/operations"})
	})
	r.Get("/projects/gantt", func(w http.ResponseWriter, r *http.Request) {
		sess := shared.SessionFromContext(r.Context())
		var csrfToken string
		if sess != nil && params.CSRFManager != nil {
			csrfToken, _ = params.CSRFManager.EnsureToken(r.Context(), sess)
		}
		_ = params.Templates.Render(w, "pages/projects/gantt.html", view.TemplateData{Title: "Project Management", CSRFToken: csrfToken, CurrentPath: "/projects/gantt"})
	})

	r.Post("/settings", func(w http.ResponseWriter, r *http.Request) {
		sess := shared.SessionFromContext(r.Context())
		if sess == nil || sess.User() == "" {
			http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
			return
		}
		theme := r.PostFormValue("theme")
		if theme != "light" && theme != "dark" {
			theme = "system"
		}
		language := r.PostFormValue("language")
		if language != "id" && language != "en" {
			language = "id"
		}
		id, err := strconv.ParseInt(sess.User(), 10, 64)
		if err != nil || params.Pool == nil {
			sess.AddFlash(shared.FlashMessage{Kind: "error", Message: "Pengaturan tidak dapat disimpan"})
			http.Redirect(w, r, "/settings", http.StatusSeeOther)
			return
		}
		notifications := r.PostFormValue("notifications") == "enabled"
		if _, err = params.Pool.Exec(r.Context(), "UPDATE users SET ui_theme = $1, ui_language = $2, ui_notifications = $3, updated_at = NOW() WHERE id = $4", theme, language, notifications, id); err != nil {
			sess.AddFlash(shared.FlashMessage{Kind: "error", Message: "Pengaturan tidak dapat disimpan"})
		} else {
			sess.AddFlash(shared.FlashMessage{Kind: "success", Message: "Pengaturan berhasil disimpan"})
		}
		http.Redirect(w, r, "/settings", http.StatusSeeOther)
	})
	r.Post("/settings/security/password", func(w http.ResponseWriter, r *http.Request) {
		sess := shared.SessionFromContext(r.Context())
		if sess == nil || sess.User() == "" {
			http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
			return
		}
		id, err := strconv.ParseInt(sess.User(), 10, 64)
		currentPassword := r.PostFormValue("current_password")
		newPassword := r.PostFormValue("new_password")
		confirmation := r.PostFormValue("confirm_password")
		if err != nil || params.Pool == nil || len(newPassword) < 8 || newPassword != confirmation {
			sess.AddFlash(shared.FlashMessage{Kind: "error", Message: "Password baru minimal 8 karakter dan konfirmasi harus sama"})
			http.Redirect(w, r, "/settings", http.StatusSeeOther)
			return
		}
		var currentHash string
		if err = params.Pool.QueryRow(r.Context(), "SELECT password_hash FROM users WHERE id = $1", id).Scan(&currentHash); err != nil || bcrypt.CompareHashAndPassword([]byte(currentHash), []byte(currentPassword)) != nil {
			sess.AddFlash(shared.FlashMessage{Kind: "error", Message: "Password saat ini tidak tepat"})
			http.Redirect(w, r, "/settings", http.StatusSeeOther)
			return
		}
		newHash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
		if err != nil {
			sess.AddFlash(shared.FlashMessage{Kind: "error", Message: "Password tidak dapat diperbarui"})
		} else if _, err = params.Pool.Exec(r.Context(), "UPDATE users SET password_hash = $1, updated_at = NOW() WHERE id = $2", string(newHash), id); err != nil {
			sess.AddFlash(shared.FlashMessage{Kind: "error", Message: "Password tidak dapat diperbarui"})
		} else {
			sess.AddFlash(shared.FlashMessage{Kind: "success", Message: "Password berhasil diperbarui"})
			if params.NotificationDispatcher != nil {
				if notifyErr := params.NotificationDispatcher.Dispatch(r.Context(), notifications.PasswordReset(id, chimw.GetReqID(r.Context()))); notifyErr != nil && params.Logger != nil {
					params.Logger.Warn("dispatch password notification", slog.Any("error", notifyErr))
				}
			}
		}
		http.Redirect(w, r, "/settings", http.StatusSeeOther)
	})
	r.Post("/company/select", func(w http.ResponseWriter, r *http.Request) {
		sess := shared.SessionFromContext(r.Context())
		if sess == nil || sess.User() == "" {
			http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
			return
		}
		companyID, err := strconv.ParseInt(r.PostFormValue("company_id"), 10, 64)
		if err != nil || companyID <= 0 || params.Pool == nil {
			sess.AddFlash(shared.FlashMessage{Kind: "error", Message: "Perusahaan tidak valid"})
			redirectBackToWorkspace(w, r)
			return
		}
		var exists bool
		if err := params.Pool.QueryRow(r.Context(), "SELECT EXISTS(SELECT 1 FROM companies WHERE id = $1)", companyID).Scan(&exists); err != nil || !exists {
			sess.AddFlash(shared.FlashMessage{Kind: "error", Message: "Perusahaan tidak ditemukan"})
			redirectBackToWorkspace(w, r)
			return
		}
		sess.Set("company_id", strconv.FormatInt(companyID, 10))
		sess.AddFlash(shared.FlashMessage{Kind: "success", Message: "Perusahaan aktif diperbarui"})
		redirectBackToWorkspace(w, r)
	})
	// Register freight routes
	freight.NewHandler(params.FreightService).RegisterRoutes(r)

	// Register Logistics UI form routes
	r.Get("/logistics/fleet/new", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		companyID := int64(1)
		fleets, _ := params.LogisticsService.ListFleets(ctx, companyID)
		_ = params.Templates.Render(w, "pages/logistics/new_vehicle.html", view.TemplateData{
			Title:       "Register Vehicle",
			CurrentPath: "/logistics/fleet",
			Data:        map[string]interface{}{"Fleets": fleets},
		})
	})
	r.Post("/logistics/fleet/new", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		companyID := int64(1)
		userID := int64(1)
		_ = r.ParseForm()
		fleetID, _ := strconv.ParseInt(r.FormValue("fleet_id"), 10, 64)
		input := logistics.CreateVehicleInput{
			CompanyID:           companyID,
			FleetID:             fleetID,
			VehicleRegistration: r.FormValue("registration"),
			VehicleType:         logistics.VehicleType(r.FormValue("type")),
			LicensePlate:        r.FormValue("license_plate"),
			Make:                r.FormValue("make"),
			Model:               r.FormValue("model"),
			VIN:                 r.FormValue("vin"),
			CreatedBy:           userID,
		}
		if maxW := r.FormValue("max_weight"); maxW != "" {
			w, _ := strconv.ParseFloat(maxW, 64)
			input.MaxWeightKg = &w
		}
		_, err := params.LogisticsService.RegisterVehicle(ctx, input)
		if err != nil {
			http.Redirect(w, r, "/logistics/fleet/new?error=1", http.StatusSeeOther)
			return
		}
		http.Redirect(w, r, "/logistics/fleet", http.StatusSeeOther)
	})

	r.Get("/logistics/trips/new", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		companyID := int64(1)
		vehicles, _ := params.LogisticsService.ListAvailableVehicles(ctx, companyID)
		drivers, _ := params.LogisticsService.ListAvailableDrivers(ctx, companyID)
		_ = params.Templates.Render(w, "pages/logistics/new_trip.html", view.TemplateData{
			Title:       "Plan New Trip",
			CurrentPath: "/logistics/trips",
			Data:        map[string]interface{}{"Vehicles": vehicles, "Drivers": drivers},
		})
	})
	r.Post("/logistics/trips/new", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		companyID := int64(1)
		userID := int64(1)
		_ = r.ParseForm()
		vehicleID, _ := strconv.ParseInt(r.FormValue("vehicle_id"), 10, 64)
		driverID, _ := strconv.ParseInt(r.FormValue("driver_id"), 10, 64)
		input := logistics.CreateTripInput{
			CompanyID:  companyID,
			TripNumber: r.FormValue("trip_number"),
			VehicleID:  vehicleID,
			DriverID:   driverID,
			CreatedBy:  userID,
		}
		if startStr := r.FormValue("planned_start"); startStr != "" {
			if t, err := time.Parse("2006-01-02T15:04", startStr); err == nil {
				input.PlannedStartAt = &t
			}
		}
		_, err := params.LogisticsService.PlanTrip(ctx, input)
		if err != nil {
			http.Redirect(w, r, "/logistics/trips/new?error=1", http.StatusSeeOther)
			return
		}
		http.Redirect(w, r, "/logistics/trips", http.StatusSeeOther)
	})

	r.Get("/logistics/rate-cards", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		companyID := int64(1)

		filter := freight.RateCardFilter{} // fetch all
		rateCards, _ := params.FreightService.ListRateCards(ctx, companyID, filter)
		_ = params.Templates.Render(w, "pages/logistics/rate_cards.html", view.TemplateData{
			Title:       "Rate Cards",
			CurrentPath: "/logistics/rate-cards",
			Data:        map[string]interface{}{"RateCards": rateCards},
		})
	})

	r.Get("/logistics/freight", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		companyID := int64(1)

		filter := freight.FreightChargeFilter{}
		charges, _ := params.FreightService.ListFreightCharges(ctx, companyID, filter)
		_ = params.Templates.Render(w, "pages/logistics/freight_charges.html", view.TemplateData{
			Title:       "Freight Charges",
			CurrentPath: "/logistics/freight",
			Data:        map[string]interface{}{"FreightCharges": charges},
		})
	})

	r.Post("/logistics/freight/{id}/invoice", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		companyID := int64(1)
		chargeIDStr := chi.URLParam(r, "id")
		chargeID, _ := strconv.ParseInt(chargeIDStr, 10, 64)

		_, err := params.FreightService.MarkFreightChargeInvoiced(ctx, companyID, chargeID)
		if err != nil {
			params.Logger.Error("failed to mark freight charge invoiced", "error", err)
			http.Redirect(w, r, "/logistics/freight?error=1", http.StatusSeeOther)
			return
		}
		http.Redirect(w, r, "/logistics/freight", http.StatusSeeOther)
	})

	r.Get("/api/me", func(w http.ResponseWriter, r *http.Request) {
		sess := shared.SessionFromContext(r.Context())
		if sess == nil || sess.User() == "" {
			http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
			return
		}
		id, err := strconv.ParseInt(sess.User(), 10, 64)
		if err != nil || params.Pool == nil {
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
		user, err := loadWorkspaceUser(r.Context(), params.Pool, id)
		if err != nil {
			http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
			return
		}
		companies, err := loadWorkspaceCompanies(r.Context(), params.Pool)
		if err != nil {
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
		activeCompanyID, _ := strconv.ParseInt(sess.Get("company_id"), 10, 64)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"user": user, "companies": companies, "activeCompanyID": activeCompanyID})
	})
	if params.NotificationHandler != nil {
		params.NotificationHandler.MountRoutes(r)
	}
	if params.ApprovalsHandler != nil {
		r.Route("/approvals", params.ApprovalsHandler.MountRoutes)
	}
	if params.HREmployeesHandler != nil {
		r.Route("/hr/employees", params.HREmployeesHandler.MountRoutes)
	}
	if params.HRLeaveHandler != nil {
		r.Route("/hr/leave", params.HRLeaveHandler.MountRoutes)
	}
	if params.HRAttendanceHandler != nil {
		r.Route("/hr/attendance", params.HRAttendanceHandler.MountRoutes)
	}
	if params.PayrollHandler != nil {
		r.Route("/payroll", params.PayrollHandler.MountRoutes)
	}
	if params.TaxHandler != nil {
		r.Route("/tax", params.TaxHandler.MountRoutes)
	}
	if params.CRMHandler != nil {
		r.Route("/crm", params.CRMHandler.MountRoutes)
	}
	if params.AccountingHandler != nil {
		r.Route("/accounting", func(r chi.Router) {
			params.AccountingHandler.MountRoutes(r)
		})
	}
	if params.ARHandler != nil {
		r.Route("/finance/ar", func(r chi.Router) {
			params.ARHandler.MountRoutes(r)
		})
	}
	if params.APHandler != nil {
		r.Route("/finance/ap", func(r chi.Router) {
			params.APHandler.MountRoutes(r)
		})
	}
	if params.RolesHandler != nil {
		r.Route("/roles", func(r chi.Router) {
			params.RolesHandler.MountRoutes(r)
		})
	}
	if params.UsersHandler != nil {
		r.Route("/users", func(r chi.Router) {
			params.UsersHandler.MountRoutes(r)
		})
	}
	if params.CloseHandler != nil {
		params.CloseHandler.MountRoutes(r)
	}
	if params.BoardPackHandler != nil {
		params.BoardPackHandler.MountRoutes(r)
	}
	if params.EliminationHandler != nil {
		params.EliminationHandler.MountRoutes(r)
	}
	if params.VarianceHandler != nil {
		params.VarianceHandler.MountRoutes(r)
	}
	if params.InventoryHandler != nil {
		r.Route("/inventory", params.InventoryHandler.MountRoutes)
	}
	if params.ProcurementHandler != nil {
		r.Route("/procurement", params.ProcurementHandler.MountRoutes)
	}
	if params.SalesHandler != nil {
		r.Route("/sales", params.SalesHandler.MountRoutes)
	}
	if params.MasterDataHandler != nil {
		r.Route("/masterdata", params.MasterDataHandler.MountRoutes)
	}
	if params.WMSHandler != nil {
		r.Route("/wms", params.WMSHandler.MountRoutes)
	}
	if params.APIHandler != nil {
		params.APIHandler.MountRoutes(r)
	}
	if params.PortalHandler != nil {
		params.PortalHandler.MountRoutes(r)
	}
	if params.POSHandler != nil {
		r.Route("/pos", params.POSHandler.MountRoutes)
	}
	if params.ProjectsHandler != nil {
		r.Route("/projects", params.ProjectsHandler.MountRoutes)
	}
	if params.MRPHandler != nil {
		r.Route("/mrp", params.MRPHandler.MountRoutes)
	}
	if params.DocumentsHandler != nil {
		r.Route("/documents", params.DocumentsHandler.MountRoutes)
	}
	if params.CMMSHandler != nil {
		r.Route("/cmms", params.CMMSHandler.MountRoutes)
	}
	if params.QMSHandler != nil {
		r.Route("/qms", params.QMSHandler.MountRoutes)
	}
	if params.ConnectorsHandler != nil {
		r.Route("/webhooks/connectors", params.ConnectorsHandler.MountRoutes)
	}
	if params.InventoryService != nil {
		r.Route("/delivery", func(r chi.Router) {
			delivery.MountRoutes(r, params.Pool, params.Logger, params.Templates, params.CSRFManager, params.RBACMiddleware, params.InventoryService)
		})
	}
	if params.ReportHandler != nil {
		r.Route("/report", params.ReportHandler.MountRoutes)
	}
	if params.ConsolHandler != nil {
		params.ConsolHandler.MountRoutes(r)
	}
	if params.BankingHandler != nil {
		r.Route("/finance/banking", params.BankingHandler.MountRoutes)
		r.Route("/finance/bankfeeds", params.BankFeedsHandler.MountRoutes)
		r.Route("/finance/forecasting", params.ForecastingHandler.MountRoutes)
		r.Route("/finance/treasury", params.TreasuryHandler.MountRoutes)
	}
	if params.JobHandler != nil {
		r.Route("/jobs", params.JobHandler.MountRoutes)
	}
	if params.AnalyticsHandler != nil {
		params.AnalyticsHandler.MountRoutes(r)
	}
	if params.InsightsHandler != nil {
		params.InsightsHandler.MountRoutes(r)
	}
	if params.AuditHandler != nil {
		params.AuditHandler.MountRoutes(r)
	}
	if params.PermissionsHandler != nil {
		r.Route("/permissions", params.PermissionsHandler.MountRoutes)
	}
	if params.Metrics != nil {
		r.Get("/metrics", params.Metrics.HTMLHandler().ServeHTTP)
		r.Method(http.MethodGet, "/metrics/prometheus", params.Metrics.Handler())
	}
	if params.DashboardHandler != nil {
		params.DashboardHandler.MountRoutes(r)
	}
	if params.SearchHandler != nil {
		params.SearchHandler.MountRoutes(r)
	}

	staticFS, err := fs.Sub(web.Static, "static")
	if err != nil {
		params.Logger.Error("create static sub filesystem", slog.Any("error", err))
	} else {
		// Static file server with Cache-Control headers
		// Note: Static files are served without rate limiting (no session/CSRF needed)
		fileServer := http.StripPrefix("/static/", http.FileServer(http.FS(staticFS)))
		r.Handle("/static/*", staticCacheHandler(fileServer))
	}

	return r
}

// staticCacheHandler wraps a file server with Cache-Control headers.
// Static assets (JS, CSS, fonts, images) are cached for 1 hour in browser.
func staticCacheHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Set Cache-Control header for static assets
		// public: can be cached by browsers and CDNs
		// max-age=3600: cache for 1 hour (3600 seconds)
		w.Header().Set("Cache-Control", "public, max-age=3600")
		next.ServeHTTP(w, r)
	})
}
