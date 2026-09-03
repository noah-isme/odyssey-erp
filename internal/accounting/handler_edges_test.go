package accounting

import (
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/odyssey-erp/odyssey-erp/internal/accounting/reports"
	"log/slog"
)

func TestNewHandlerMountsAccountingRoutes(t *testing.T) {
	h := NewHandler(slog.Default(), nil, nil, nil, nil, nil)
	if h.accountHandler == nil || h.journalHandler == nil || h.banksHandler == nil || h.assets == nil || h.dimensions == nil || h.schedules == nil {
		t.Fatalf("NewHandler() did not wire all accounting services: %#v", h)
	}
	h.MountRoutes(chi.NewRouter())
}

func TestAccountingRequestHelpers(t *testing.T) {
	h := &Handler{}
	request := httptest.NewRequest("GET", "/?department_id=7&cost_center_id=8", nil)
	filter := dimensionFilter(request)
	if filter != (reports.DimensionFilter{DepartmentID: 7, CostCenterID: 8}) {
		t.Fatalf("dimensionFilter() = %#v", filter)
	}
	if h.companyID(request) != 0 || h.csrfToken(request) != "" {
		t.Fatal("request helpers returned values without a session/CSRF manager")
	}
	w := httptest.NewRecorder()
	h.logger = slog.Default()
	h.handleNotImplemented(w, request)
	if w.Code != 501 {
		t.Fatalf("handleNotImplemented() status = %d", w.Code)
	}
}

type dimensionRows struct {
	rows  [][3]string
	index int
}

func (r *dimensionRows) Next() bool {
	if r.index >= len(r.rows) {
		return false
	}
	r.index++
	return true
}

func (r *dimensionRows) Scan(dest ...any) error {
	row := r.rows[r.index-1]
	*(dest[0].(*int64)) = 1
	*(dest[1].(*string)) = row[1]
	*(dest[2].(*string)) = row[2]
	return nil
}

func TestRowsToDimensionMaps(t *testing.T) {
	rows := &dimensionRows{rows: [][3]string{{"1", "D1", "Sales"}, {"2", "D2", "Finance"}}}
	got := rowsToDimensionMaps(rows)
	if len(got) != 2 || got[0]["Code"] != "D1" || got[1]["Name"] != "Finance" {
		t.Fatalf("rowsToDimensionMaps() = %#v", got)
	}
	if got := rowsToDimensionMaps(nil); len(got) != 0 {
		t.Fatal("rowsToDimensionMaps(nil) should be empty")
	}
}
