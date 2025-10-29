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
