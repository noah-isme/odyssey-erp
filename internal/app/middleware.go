package app

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/httprate"
	"github.com/unrolled/secure"


	"github.com/odyssey-erp/odyssey-erp/internal/observability"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
)

// MiddlewareConfig aggregates dependencies shared by the middleware stack.
type MiddlewareConfig struct {
	Logger         *slog.Logger
	Config         *Config
	SessionManager *shared.SessionManager
	CSRFManager    *shared.CSRFManager
	Metrics        *observability.Metrics
}

type responseWriterWithCommit struct {
	http.ResponseWriter
	sess          *shared.Session
	manager       *shared.SessionManager
	ctx           context.Context
	req           *http.Request
	headerWritten bool
}

func (w *responseWriterWithCommit) WriteHeader(statusCode int) {
	if !w.headerWritten {
		w.headerWritten = true
		_ = w.manager.Commit(w.ctx, w.ResponseWriter, w.req, w.sess)
	}
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *responseWriterWithCommit) Write(data []byte) (int, error) {
	if !w.headerWritten {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(data)
}

// MiddlewareStack installs the Odyssey middleware chain.
func MiddlewareStack(cfg MiddlewareConfig) []func(http.Handler) http.Handler {
	// CSP allows:
	// - 'self' for scripts, styles, images, etc.
	// - fonts.googleapis.com and fonts.gstatic.com for Google Fonts
	// - 'unsafe-inline' for styles (needed for inline style attributes in templates)
	csp := "default-src 'self'; " +
		"style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; " +
		"font-src 'self' https://fonts.gstatic.com; " +
		"img-src 'self' data:; " +
		"script-src 'self'"

	secureMiddleware := secure.New(secure.Options{
		FrameDeny:             true,
		ContentTypeNosniff:    true,
		BrowserXssFilter:      true,
		ReferrerPolicy:        "strict-origin-when-cross-origin",
		FeaturePolicy:         "none",
		ContentSecurityPolicy: csp,
		SSLRedirect:           cfg.Config != nil && cfg.Config.IsProduction(),
		SSLProxyHeaders:       map[string]string{"X-Forwarded-Proto": "https"},
	})

	sessionMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			sess, err := cfg.SessionManager.Load(ctx, r)
			if err != nil {
				cfg.Logger.Error("failed to load session", slog.Any("error", err))
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
				return
			}
			ctx = shared.ContextWithSession(ctx, sess)
			
			// Wrap to intercept WriteHeader
			wrapped := &responseWriterWithCommit{
				ResponseWriter: w,
				sess:           sess,
				manager:        cfg.SessionManager,
				ctx:            ctx,
				req:            r.WithContext(ctx),
			}
			
			next.ServeHTTP(wrapped, r.WithContext(ctx))
		})
	}

	csrfMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
				next.ServeHTTP(w, r)
				return
			}
			sess := shared.SessionFromContext(r.Context())
			if sess == nil {
				http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
				return
			}
			token := r.PostFormValue(shared.CSRFFormField)
			if token == "" {
				token = r.Header.Get("X-CSRF-Token")
			}
			if err := cfg.CSRFManager.VerifyToken(r.Context(), sess, token); err != nil {
				cfg.Logger.Warn("csrf validation failed", slog.String("path", r.URL.Path))
				http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}

	timeout := 30 * time.Second
	if cfg.Config != nil && cfg.Config.AppRequestTimeout > 0 {
		timeout = cfg.Config.AppRequestTimeout
	}

	middlewares := []func(http.Handler) http.Handler{
		middleware.RealIP,
		middleware.RequestID,
		sessionMiddleware,
		middleware.Recoverer,
		middleware.Timeout(timeout),
		func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if err := secureMiddleware.Process(w, r); err != nil {
					cfg.Logger.Warn("secure headers blocked request", slog.Any("error", err))
					http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
					return
				}
				next.ServeHTTP(w, r)
			})
		},
		middleware.Compress(5),
		conditionalRateLimiter(60, time.Minute),
		csrfMiddleware,

	}
	if cfg.Metrics != nil {
		middlewares = append(middlewares, func(next http.Handler) http.Handler {
			return cfg.Metrics.Middleware(next)
		})
	}
	return middlewares
}

// conditionalRateLimiter returns a rate limiting middleware that skips static files.
// Static assets (JS, CSS, images, fonts) don't need rate limiting and can be safely
// loaded multiple times without counting against the request limit.
func conditionalRateLimiter(requestLimit int, windowLength time.Duration) func(http.Handler) http.Handler {
	limiter := httprate.Limit(requestLimit, windowLength, httprate.WithKeyFuncs(httprate.KeyByIP))

	return func(next http.Handler) http.Handler {
		limited := limiter(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip rate limiting for static files
			if strings.HasPrefix(r.URL.Path, "/static") {
				next.ServeHTTP(w, r)
				return
			}
			// Apply rate limiting to all other requests
			limited.ServeHTTP(w, r)
		})
	}
}
