package export

import (
	"bytes"
	"context"
	"encoding/csv"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/odyssey-erp/odyssey-erp/internal/analytics"
)

func TestWriteKPICSV(t *testing.T) {
	summary := analytics.KPISummary{NetProfit: 100, Revenue: 200}
	buf := &bytes.Buffer{}
	if err := WriteKPICSV(buf, summary, "2025-01"); err != nil {
		t.Fatalf("kpi csv error: %v", err)
	}
	reader := csv.NewReader(bytes.NewReader(buf.Bytes()))
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("csv read error: %v", err)
	}
	if len(records) < 2 {
		t.Fatalf("expected data rows, got %d", len(records))
	}
}

func TestPDFExporterRender(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/forms/chromium/convert/html" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if err := r.ParseMultipartForm(4 << 10); err != nil {
			t.Fatalf("unexpected parse error: %v", err)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("PDF"))
	}))
	defer srv.Close()

	exporter := &PDFExporter{Endpoint: srv.URL}
	payload := DashboardPayload{Period: "2025-01"}
	data, err := exporter.RenderDashboard(context.Background(), payload)
	if err != nil {
		t.Fatalf("pdf render error: %v", err)
	}
	if string(data) != "PDF" {
		t.Fatalf("unexpected payload %q", string(data))
	}
}
