package observability

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestMetricsHandlerExposesPrometheusMetrics(t *testing.T) {
	metrics := NewMetrics()

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)

	metrics.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", rr.Code)
	}

	body := rr.Body.String()
	if !strings.Contains(body, "odyssey_jobs_total") {
		t.Fatalf("expected body to contain odyssey_jobs_total, got: %s", body)
	}
}

func TestMetricsMiddlewareRecordsRequest(t *testing.T) {
	metrics := NewMetrics()

	handler := metrics.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))

	routeCtx := chi.NewRouteContext()
	routeCtx.RoutePatterns = append(routeCtx.RoutePatterns, "/test")

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx)
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusTeapot {
		t.Fatalf("expected status %d, got %d", http.StatusTeapot, rr.Code)
	}

	metricsRR := httptest.NewRecorder()
	metrics.Handler().ServeHTTP(metricsRR, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	metricsBody := metricsRR.Body.String()
	if !strings.Contains(metricsBody, "http_requests_total{code=\"418\",route=\"/test\"} 1") {
		t.Fatalf("expected metrics to record request, got: %s", metricsBody)
	}
	if !strings.Contains(metricsBody, "http_request_duration_seconds_bucket{route=\"/test\"") {
		t.Fatalf("expected duration histogram to be present, got: %s", metricsBody)
	}
}
