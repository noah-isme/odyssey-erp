package insightshhtp

import (
	"context"
	"html/template"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/odyssey-erp/odyssey-erp/internal/insights"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/view"
)

type stubInsightsService struct {
	result      insights.Result
	err         error
	lastFilters insights.CompareFilters
}

func (s *stubInsightsService) Load(ctx context.Context, filters insights.CompareFilters) (insights.Result, error) {
	s.lastFilters = filters
	return s.result, s.err
}

type stubInsightsRBAC struct {
	perms []string
	err   error
}

func (s stubInsightsRBAC) EffectivePermissions(ctx context.Context, userID int64) ([]string, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.perms, nil
}

func newInsightsHandler(t *testing.T, service *stubInsightsService, rbacPerms []string) *Handler {
	t.Helper()
	templates, err := view.NewEngine()
	if err != nil {
		t.Fatalf("parse templates: %v", err)
	}
	handler := NewHandler(nil, service, templates, stubInsightsRBAC{perms: rbacPerms})
	handler.chart = func(width, height int, seriesA, seriesB []float64, labels []string) (template.HTML, error) {
		return template.HTML("<svg>mock</svg>"), nil
	}
	handler.now = func() time.Time { return time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC) }
	return handler
}

func TestInsightsRequiresPermission(t *testing.T) {
	service := &stubInsightsService{}
	handler := newInsightsHandler(t, service, nil)
	req := httptest.NewRequest(http.MethodGet, "/finance/insights?company_id=1", nil)
	rr := httptest.NewRecorder()
	handler.handleInsights(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rr.Code)
	}
}

func TestInsightsRendersChart(t *testing.T) {
	service := &stubInsightsService{
		result: insights.Result{
			Series:       []insights.MonthlySeries{{Month: "2024-01", Net: 10, Revenue: 20}},
			Variance:     []insights.VarianceMetric{{Metric: "Net", MoMPct: 5, YoYPct: 10}},
			Contribution: []insights.ContributionShare{{Branch: "Branch 1", NetPct: 60, RevenuePct: 40}},
		},
	}
	handler := newInsightsHandler(t, service, []string{shared.PermFinanceInsightsView})
	req := httptest.NewRequest(http.MethodGet, "/finance/insights?from=2024-01&to=2024-03&company_id=2", nil)
	sess := &shared.Session{}
	sess.SetUser("42")
	ctx := shared.ContextWithSession(req.Context(), sess)
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()

	handler.handleInsights(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "<svg>mock</svg>") {
		t.Fatalf("expected svg chart in response: %s", body)
	}
	if service.lastFilters.From != "2024-01" || service.lastFilters.To != "2024-03" {
		t.Fatalf("unexpected filters passed to service: %+v", service.lastFilters)
	}
}

func TestInsightsInvalidFilter(t *testing.T) {
	service := &stubInsightsService{}
	handler := newInsightsHandler(t, service, []string{shared.PermFinanceInsightsView})
	req := httptest.NewRequest(http.MethodGet, "/finance/insights?from=bad&to=2024-03", nil)
	sess := &shared.Session{}
	sess.SetUser("77")
	req = req.WithContext(shared.ContextWithSession(req.Context(), sess))
	rr := httptest.NewRecorder()

	handler.handleInsights(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid filter, got %d", rr.Code)
	}
}
