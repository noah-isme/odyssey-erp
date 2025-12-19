package app

import (
	"io/fs"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/odyssey-erp/odyssey-erp/internal/accounting"
	analytichttp "github.com/odyssey-erp/odyssey-erp/internal/analytics/http"
	"github.com/odyssey-erp/odyssey-erp/internal/ar"
	audithttp "github.com/odyssey-erp/odyssey-erp/internal/audit/http"
	auth "github.com/odyssey-erp/odyssey-erp/internal/auth"
	boardpackhttp "github.com/odyssey-erp/odyssey-erp/internal/boardpack/http"
	closehttp "github.com/odyssey-erp/odyssey-erp/internal/close/http"
	consolhttp "github.com/odyssey-erp/odyssey-erp/internal/consol/http"
	"github.com/odyssey-erp/odyssey-erp/internal/delivery"
	eliminationhttp "github.com/odyssey-erp/odyssey-erp/internal/elimination/http"
	insightshhtp "github.com/odyssey-erp/odyssey-erp/internal/insights/http"
	"github.com/odyssey-erp/odyssey-erp/internal/inventory"
	"github.com/odyssey-erp/odyssey-erp/internal/masterdata"
	"github.com/odyssey-erp/odyssey-erp/internal/observability"
	"github.com/odyssey-erp/odyssey-erp/internal/procurement"
	"github.com/odyssey-erp/odyssey-erp/internal/rbac"
	"github.com/odyssey-erp/odyssey-erp/internal/roles"
	"github.com/odyssey-erp/odyssey-erp/internal/sales"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/users"
	variancehttp "github.com/odyssey-erp/odyssey-erp/internal/variance/http"
	"github.com/odyssey-erp/odyssey-erp/internal/view"
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
	VarianceHandler    *variancehttp.Handler
	InsightsHandler    *insightshhtp.Handler
	AuditHandler       *audithttp.Handler
	InventoryHandler   *inventory.Handler
	ProcurementHandler *procurement.Handler
	SalesHandler       *sales.Handler
	MasterDataHandler  *masterdata.Handler
	Pool               *pgxpool.Pool
	RBACMiddleware     rbac.Middleware

	ReportHandler      *report.Handler
	BoardPackHandler   *boardpackhttp.Handler
	JobHandler         *jobs.Handler
	AnalyticsHandler   *analytichttp.Handler
	ConsolHandler      *consolhttp.Handler
	PermissionsHandler *rbac.PermissionsHandler
	Metrics            *observability.Metrics
}

// NewRouter constructs the chi.Router with Odyssey defaults.
func NewRouter(params RouterParams) http.Handler {
	r := chi.NewRouter()

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

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
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
		data := view.TemplateData{
			Title:     "Odyssey ERP",
			CSRFToken: csrfToken,
			Flash:     flash,
			Data: map[string]any{
				"AppEnv": params.Config.AppEnv,
			},
		}
		if err := params.Templates.Render(w, "pages/home.html", data); err != nil {
			params.Logger.Error("render home", slog.Any("error", err))
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		}
	})

	r.Route("/auth", params.AuthHandler.MountRoutes)
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
	r.Route("/inventory", params.InventoryHandler.MountRoutes)
	r.Route("/procurement", params.ProcurementHandler.MountRoutes)
	if params.SalesHandler != nil {
		r.Route("/sales", params.SalesHandler.MountRoutes)
	}
	if params.MasterDataHandler != nil {
		r.Route("/masterdata", params.MasterDataHandler.MountRoutes)
	}
	r.Route("/delivery", func(r chi.Router) {
		delivery.MountRoutes(r, params.Pool, params.Logger, params.Templates, params.CSRFManager, params.RBACMiddleware)
	})
	r.Route("/report", params.ReportHandler.MountRoutes)
	if params.ConsolHandler != nil {
		params.ConsolHandler.MountRoutes(r)
	}
	r.Route("/jobs", params.JobHandler.MountRoutes)
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
		r.Method(http.MethodGet, "/metrics", params.Metrics.Handler())
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
