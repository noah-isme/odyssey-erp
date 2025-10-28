package analytichttp

import (
	"bytes"
	"context"
	"html/template"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/odyssey-erp/odyssey-erp/internal/analytics"
	"github.com/odyssey-erp/odyssey-erp/internal/analytics/export"
	"github.com/odyssey-erp/odyssey-erp/internal/analytics/svg"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/view"
)

type stubService struct {
	summary analytics.KPISummary
	pl      []analytics.PLTrendPoint
	cash    []analytics.CashflowTrendPoint
	ar      []analytics.AgingBucket
	ap      []analytics.AgingBucket
}

func (s *stubService) GetKPISummary(ctx context.Context, filter analytics.KPIFilter) (analytics.KPISummary, error) {
	return s.summary, nil
}

func (s *stubService) GetPLTrend(ctx context.Context, filter analytics.TrendFilter) ([]analytics.PLTrendPoint, error) {
	return s.pl, nil
}

func (s *stubService) GetCashflowTrend(ctx context.Context, filter analytics.TrendFilter) ([]analytics.CashflowTrendPoint, error) {
	return s.cash, nil
}

func (s *stubService) GetARAging(ctx context.Context, filter analytics.AgingFilter) ([]analytics.AgingBucket, error) {
	return s.ar, nil
}

func (s *stubService) GetAPAging(ctx context.Context, filter analytics.AgingFilter) ([]analytics.AgingBucket, error) {
	return s.ap, nil
}

type stubRBAC struct {
	perms []string
	err   error
}

func (s stubRBAC) EffectivePermissions(ctx context.Context, userID int64) ([]string, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.perms, nil
}

type stubValidator struct {
	err error
}

func (s stubValidator) ValidatePeriod(ctx context.Context, period string) error {
	return s.err
}

type stubPDF struct {
	data []byte
	err  error
	last export.DashboardPayload
}

func (s *stubPDF) RenderDashboard(ctx context.Context, payload export.DashboardPayload) ([]byte, error) {
	s.last = payload
	if s.data == nil {
		content := bytes.Repeat([]byte("PDF"), 400)
		s.data = append([]byte("%PDF-1.4\n"), content...)
	}
	return s.data, s.err
}

type lineAdapter func(width, height int, series []float64, labels []string, opts svg.LineOpts) (template.HTML, error)

type barAdapter func(width, height int, seriesA, seriesB []float64, labels []string, opts svg.BarOpts) (template.HTML, error)

func (a lineAdapter) Line(width, height int, series []float64, labels []string, opts svg.LineOpts) (template.HTML, error) {
	return a(width, height, series, labels, opts)
}

func (a barAdapter) Bars(width, height int, seriesA, seriesB []float64, labels []string, opts svg.BarOpts) (template.HTML, error) {
	return a(width, height, seriesA, seriesB, labels, opts)
}

func newTestHandler(t *testing.T, rbacPerms []string) *Handler {
	t.Helper()
	templates, err := view.NewEngine()
	if err != nil {
		t.Fatalf("parse templates: %v", err)
	}
	service := &stubService{
		summary: analytics.KPISummary{Revenue: 1000, Opex: 300, NetProfit: 700, CashIn: 900, CashOut: 200, COGS: 400, AROutstanding: 500, APOutstanding: 250},
		pl:      []analytics.PLTrendPoint{{Period: "2025-01", Net: 500, Revenue: 1000, COGS: 400, Opex: 100}},
		cash:    []analytics.CashflowTrendPoint{{Period: "2025-01", In: 900, Out: 200}},
		ar:      []analytics.AgingBucket{{Bucket: "0-30", Amount: 400}},
		ap:      []analytics.AgingBucket{{Bucket: "0-30", Amount: 150}},
	}
	handler := NewHandler(nil, service, templates, lineAdapter(svg.Line), barAdapter(svg.Bars), &stubPDF{}, stubRBAC{perms: rbacPerms}, stubValidator{})
	handler.WithNow(func() time.Time { return time.Date(2025, 2, 15, 0, 0, 0, 0, time.UTC) })
	return handler
}

func TestDashboardRequiresPermission(t *testing.T) {
	handler := newTestHandler(t, nil)
	req := httptest.NewRequest(http.MethodGet, "/finance/analytics?company_id=1", nil)
	rr := httptest.NewRecorder()
	handler.handleDashboard(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rr.Code)
	}
}

func TestDashboardSuccess(t *testing.T) {
	handler := newTestHandler(t, []string{shared.PermFinanceAnalyticsView})
	req := httptest.NewRequest(http.MethodGet, "/finance/analytics?company_id=2&branch_id=5&period=2025-01", nil)
	sess := &shared.Session{}
	sess.SetUser("42")
	req = req.WithContext(shared.ContextWithSession(req.Context(), sess))
	rr := httptest.NewRecorder()
	handler.handleDashboard(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "Dashboard Keuangan") {
		t.Fatalf("expected dashboard title in response")
	}
	if !strings.Contains(body, "Rp 1.000,00") {
		t.Fatalf("expected formatted currency in response: %s", body)
	}
}

func TestCSVExport(t *testing.T) {
	handler := newTestHandler(t, []string{shared.PermFinanceAnalyticsExport})
	handler.pdf = &stubPDF{}
	req := httptest.NewRequest(http.MethodGet, "/finance/analytics/export.csv?company_id=2", nil)
	sess := &shared.Session{}
	sess.SetUser("7")
	req = req.WithContext(shared.ContextWithSession(req.Context(), sess))
	rr := httptest.NewRecorder()
	handler.handleCSV(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/csv") {
		t.Fatalf("unexpected content type %s", ct)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "Metric,Value") {
		t.Fatalf("expected KPI section in CSV")
	}
	if !strings.Contains(body, "Cash Out") {
		t.Fatalf("expected cashflow section in CSV")
	}
}

func TestPDFExport(t *testing.T) {
	pdf := &stubPDF{}
	handler := newTestHandler(t, []string{shared.PermFinanceAnalyticsExport})
	handler.pdf = pdf
	req := httptest.NewRequest(http.MethodGet, "/finance/analytics/pdf?company_id=2", nil)
	sess := &shared.Session{}
	sess.SetUser("99")
	req = req.WithContext(shared.ContextWithSession(req.Context(), sess))
	rr := httptest.NewRecorder()
	handler.handlePDF(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/pdf" {
		t.Fatalf("unexpected content type %s", ct)
	}
	if rr.Body.Len() <= 1024 {
		t.Fatalf("expected pdf body >1KB, got %d bytes", rr.Body.Len())
	}
	if len(pdf.last.PL) == 0 {
		t.Fatalf("expected payload to include trend data")
	}
}

func TestInvalidFilterReturnsBadRequest(t *testing.T) {
	handler := newTestHandler(t, []string{shared.PermFinanceAnalyticsView})
	req := httptest.NewRequest(http.MethodGet, "/finance/analytics?period=2025-13&company_id=1", nil)
	sess := &shared.Session{}
	sess.SetUser("1")
	req = req.WithContext(shared.ContextWithSession(req.Context(), sess))
	rr := httptest.NewRecorder()
	handler.handleDashboard(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid period, got %d", rr.Code)
	}
}
