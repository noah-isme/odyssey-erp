package app

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"github.com/odyssey-erp/odyssey-erp/internal/auth"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/view"
	"github.com/odyssey-erp/odyssey-erp/jobs"
	"github.com/odyssey-erp/odyssey-erp/report"
	"github.com/odyssey-erp/odyssey-erp/web"
)

// RouterParams groups dependencies for building the HTTP router.
type RouterParams struct {
	Logger         *slog.Logger
	Config         *Config
	Templates      *view.Engine
	SessionManager *shared.SessionManager
	CSRFManager    *shared.CSRFManager
	AuthHandler    *auth.Handler
	ReportHandler  *report.Handler
	JobHandler     *jobs.Handler
}

// NewRouter constructs the chi.Router with Odyssey defaults.
func NewRouter(params RouterParams) http.Handler {
	r := chi.NewRouter()

	for _, mw := range MiddlewareStack(MiddlewareConfig{
		Logger:         params.Logger,
		Config:         params.Config,
		SessionManager: params.SessionManager,
		CSRFManager:    params.CSRFManager,
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
	r.Route("/report", params.ReportHandler.MountRoutes)
	r.Route("/jobs", params.JobHandler.MountRoutes)

	fileServer := http.StripPrefix("/static/", http.FileServer(http.FS(web.Static)))
	r.Handle("/static/*", fileServer)

	return r
}
