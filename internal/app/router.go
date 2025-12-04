package app

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	analytichttp "github.com/odyssey-erp/odyssey-erp/internal/analytics/http"
	audithttp "github.com/odyssey-erp/odyssey-erp/internal/audit/http"
	"github.com/odyssey-erp/odyssey-erp/internal/auth"
	boardpackhttp "github.com/odyssey-erp/odyssey-erp/internal/boardpack/http"
	closehttp "github.com/odyssey-erp/odyssey-erp/internal/close/http"
	consolhttp "github.com/odyssey-erp/odyssey-erp/internal/consol/http"
	"github.com/odyssey-erp/odyssey-erp/internal/delivery"
	eliminationhttp "github.com/odyssey-erp/odyssey-erp/internal/elimination/http"
	insightshhtp "github.com/odyssey-erp/odyssey-erp/internal/insights/http"
	"github.com/odyssey-erp/odyssey-erp/internal/inventory"
	"github.com/odyssey-erp/odyssey-erp/internal/observability"
	"github.com/odyssey-erp/odyssey-erp/internal/procurement"
	"github.com/odyssey-erp/odyssey-erp/internal/sales"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
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
	CloseHandler       *closehttp.Handler
	EliminationHandler *eliminationhttp.Handler
	VarianceHandler    *variancehttp.Handler
	InsightsHandler    *insightshhtp.Handler
	AuditHandler       *audithttp.Handler
	InventoryHandler   *inventory.Handler
	ProcurementHandler *procurement.Handler
	SalesHandler       *sales.Handler
	DeliveryHandler    *delivery.Handler
	ReportHandler      *report.Handler
	BoardPackHandler   *boardpackhttp.Handler
	JobHandler         *jobs.Handler
	AnalyticsHandler   *analytichttp.Handler
	ConsolHandler      *consolhttp.Handler
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

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
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
	if params.DeliveryHandler != nil {
		r.Route("/delivery", params.DeliveryHandler.MountRoutes)
	}
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
	if params.Metrics != nil {
		r.Method(http.MethodGet, "/metrics", params.Metrics.Handler())
	}

	fileServer := http.StripPrefix("/static/", http.FileServer(http.FS(web.Static)))
	r.Handle("/static/*", fileServer)

	return r
}
