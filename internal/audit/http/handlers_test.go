package audithttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/odyssey-erp/odyssey-erp/internal/audit"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/view"
)

type stubTimelineService struct {
	result      audit.Result
	exportRows  []audit.TimelineRow
	lastFilters audit.TimelineFilters
}

func (s *stubTimelineService) Timeline(ctx context.Context, filters audit.TimelineFilters) (audit.Result, error) {
	s.lastFilters = filters
	return s.result, nil
}

func (s *stubTimelineService) Export(ctx context.Context, filters audit.TimelineFilters) ([]audit.TimelineRow, error) {
	s.lastFilters = filters
	return s.exportRows, nil
}

type stubExporter struct {
	csv []byte
}

func (s stubExporter) WriteCSV(rows []audit.TimelineRow) ([]byte, error) {
	if s.csv != nil {
		return s.csv, nil
	}
	return audit.NewExporter(nil).WriteCSV(rows)
}

func (s stubExporter) RenderPDF(ctx context.Context, vm audit.ViewModel) ([]byte, error) {
	return nil, audit.ErrPDFUnavailable
}

type stubAuditRBAC struct {
	perms []string
}

func (s stubAuditRBAC) EffectivePermissions(ctx context.Context, userID int64) ([]string, error) {
	return s.perms, nil
}

func newAuditHandler(t *testing.T, service *stubTimelineService, exporter Exporter, perms []string) *Handler {
	t.Helper()
	templates, err := view.NewEngine()
	if err != nil {
		t.Fatalf("parse templates: %v", err)
	}
	handler := NewHandler(nil, service, templates, exporter, stubAuditRBAC{perms: perms})
	handler.now = func() time.Time { return time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC) }
	return handler
}

func TestTimelineRequiresPermission(t *testing.T) {
	service := &stubTimelineService{}
	handler := newAuditHandler(t, service, stubExporter{}, nil)
	req := httptest.NewRequest(http.MethodGet, "/finance/audit/timeline", nil)
	rr := httptest.NewRecorder()
	handler.handleTimeline(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rr.Code)
	}
}

func TestTimelineRendersRows(t *testing.T) {
	rows := []audit.TimelineRow{{At: time.Date(2024, 3, 10, 10, 0, 0, 0, time.UTC), Actor: "auditor", Action: "UPDATE", Entity: "journal_entries", EntityID: "1"}}
	service := &stubTimelineService{result: audit.Result{Rows: rows, Paging: audit.PagingInfo{Page: 1, PageSize: 20}}}
	handler := newAuditHandler(t, service, stubExporter{}, []string{shared.PermFinanceAuditView})
	req := httptest.NewRequest(http.MethodGet, "/finance/audit/timeline?from=2024-03-01&to=2024-03-15", nil)
	sess := &shared.Session{}
	sess.SetUser("7")
	req = req.WithContext(shared.ContextWithSession(req.Context(), sess))
	rr := httptest.NewRecorder()
	handler.handleTimeline(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "auditor") {
		t.Fatalf("expected actor in response: %s", body)
	}
	if service.lastFilters.From.Format("2006-01-02") != "2024-03-01" {
		t.Fatalf("unexpected filters: %+v", service.lastFilters)
	}
}

func TestExportCSV(t *testing.T) {
	rows := []audit.TimelineRow{{Actor: "auditor"}}
	service := &stubTimelineService{exportRows: rows}
	handler := newAuditHandler(t, service, stubExporter{csv: []byte("actor")}, []string{shared.PermFinanceAuditView, shared.PermFinanceInsightsExport})
	req := httptest.NewRequest(http.MethodGet, "/finance/audit/timeline/export.csv?from=2024-03-01&to=2024-03-05", nil)
	sess := &shared.Session{}
	sess.SetUser("7")
	req = req.WithContext(shared.ContextWithSession(req.Context(), sess))
	rr := httptest.NewRecorder()
	handler.handleExport(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if ctype := rr.Header().Get("Content-Type"); !strings.Contains(ctype, "text/csv") {
		t.Fatalf("unexpected content-type: %s", ctype)
	}
}

func TestPDFNotImplemented(t *testing.T) {
	service := &stubTimelineService{}
	handler := newAuditHandler(t, service, stubExporter{}, []string{shared.PermFinanceAuditView, shared.PermFinanceInsightsExport})
	req := httptest.NewRequest(http.MethodGet, "/finance/audit/timeline/pdf", nil)
	sess := &shared.Session{}
	sess.SetUser("7")
	req = req.WithContext(shared.ContextWithSession(req.Context(), sess))
	rr := httptest.NewRecorder()
	handler.handlePDF(rr, req)
	if rr.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501, got %d", rr.Code)
	}
}
