package observability

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics mengumpulkan metrik Prometheus untuk aplikasi.
type Metrics struct {
	registry        *prometheus.Registry
	handler         http.Handler
	requestsTotal   *prometheus.CounterVec
	requestDuration *prometheus.HistogramVec
}

// NewMetrics menginisialisasi registry dan metrik dasar.
func NewMetrics() *Metrics {
	registry := prometheus.NewRegistry()
	requests := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "odyssey_http_requests_total",
		Help: "Jumlah permintaan HTTP berdasarkan route dan status.",
	}, []string{"route", "code"})
	duration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "odyssey_http_request_duration_seconds",
		Help:    "Durasi permintaan HTTP per route.",
		Buckets: prometheus.DefBuckets,
	}, []string{"route"})
	registry.MustRegister(requests, duration)
	return &Metrics{
		registry:        registry,
		handler:         promhttp.HandlerFor(registry, promhttp.HandlerOpts{}),
		requestsTotal:   requests,
		requestDuration: duration,
	}
}

// Handler mengembalikan http.Handler untuk endpoint /metrics.
func (m *Metrics) Handler() http.Handler {
	if m == nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
		})
	}
	return m.handler
}

// Middleware mencatat metrik untuk setiap permintaan HTTP.
func (m *Metrics) Middleware(next http.Handler) http.Handler {
	if m == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		recorder := statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(&recorder, r)
		route := routePattern(r)
		m.requestsTotal.WithLabelValues(route, strconv.Itoa(recorder.status)).Inc()
		m.requestDuration.WithLabelValues(route).Observe(time.Since(start).Seconds())
	})
}

// Registerer mengekspos registry untuk pendaftaran metrik khusus.
func (m *Metrics) Registerer() prometheus.Registerer {
	if m == nil {
		return prometheus.DefaultRegisterer
	}
	return m.registry
}

// HTMLHandler returns an http.Handler serving a styled metrics dashboard.
func (m *Metrics) HTMLHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		html := `<!DOCTYPE html>
<html lang="id" data-theme="dark">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Metrics Dashboard - Odyssey ERP</title>
    <link rel="stylesheet" href="/static/css/main.css">
</head>
<body style="background: var(--bg-app); min-height: 100vh;">
    <main class="metrics-page">
        <a href="/" class="metrics-back">← Back to Dashboard</a>
        
        <header class="metrics-header">
            <h1 class="metrics-header__title">Metrics Dashboard</h1>
            <p class="metrics-header__subtitle">Application observability and performance metrics</p>
        </header>

        <section class="metrics-section">
            <header class="metrics-section__header">
                <h2 class="metrics-section__title">Available Metrics</h2>
            </header>
            <div class="metrics-section__body">
                <ul class="metrics-list">
                    <li class="metrics-list__item">
                        <code class="metrics-list__code">odyssey_http_requests_total</code>
                        <span>Total HTTP requests by route and status code</span>
                    </li>
                    <li class="metrics-list__item">
                        <code class="metrics-list__code">odyssey_http_request_duration_seconds</code>
                        <span>HTTP request duration histogram per route</span>
                    </li>
                </ul>
            </div>
        </section>

        <section class="metrics-section">
            <header class="metrics-section__header">
                <h2 class="metrics-section__title">Prometheus Endpoint</h2>
            </header>
            <div class="metrics-section__body">
                <a href="/metrics/prometheus" class="metrics-action">
                    <div>
                        <div class="metrics-action__title">/metrics/prometheus</div>
                        <div class="metrics-action__desc">Raw Prometheus format for scraping</div>
                    </div>
                </a>
            </div>
        </section>

        <section class="metrics-section">
            <header class="metrics-section__header">
                <h2 class="metrics-section__title">Integration</h2>
            </header>
            <div class="metrics-section__body">
                <p class="metrics-info">Configure your Prometheus server to scrape:</p>
                <pre class="metrics-code-block">scrape_configs:
  - job_name: 'odyssey-erp'
    static_configs:
      - targets: ['localhost:8080']
    metrics_path: '/metrics/prometheus'</pre>
            </div>
        </section>
    </main>
</body>
</html>`
		_, _ = w.Write([]byte(html))
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func routePattern(r *http.Request) string {
	if routeCtx := chi.RouteContext(r.Context()); routeCtx != nil {
		if pattern := routeCtx.RoutePattern(); pattern != "" {
			return pattern
		}
	}
	return "unknown"
}
