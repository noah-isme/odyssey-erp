package export

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPDFExporter_RenderPackingList_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/forms/chromium/convert/html", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Contains(t, r.Header.Get("Content-Type"), "multipart/form-data")

		err := r.ParseMultipartForm(10 << 20)
		if err != nil {
			t.Fatalf("failed to parse multipart form: %v", err)
		}

		file, _, err := r.FormFile("files")
		if err != nil {
			t.Fatalf("failed to get files: %v", err)
		}
		defer file.Close()

		htmlContent, err := io.ReadAll(file)
		if err != nil {
			t.Fatalf("failed to read file: %v", err)
		}

		html := string(htmlContent)
		assert.Contains(t, html, "PACKING LIST")
		assert.Contains(t, html, "DO-202501-0001")
		assert.Contains(t, html, "Acme Corporation")
		assert.Contains(t, html, "WIDGET-A")

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("MOCK-PDF-CONTENT"))
	}))
	defer srv.Close()

	exporter, err := NewPDFExporter(srv.URL, srv.Client())
	require.NoError(t, err)

	payload := createTestPayload()

	pdfBytes, err := exporter.RenderPackingList(context.Background(), payload)
	require.NoError(t, err)
	assert.Equal(t, "MOCK-PDF-CONTENT", string(pdfBytes))
}

func TestPDFExporter_RenderPackingList_NilExporter(t *testing.T) {
	var exporter *PDFExporter
	payload := createTestPayload()

	_, err := exporter.RenderPackingList(context.Background(), payload)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not initialized")
}

func TestPDFExporter_RenderPackingList_EmptyEndpoint(t *testing.T) {
	exporter, err := NewPDFExporter("", nil)
	require.NoError(t, err)

	payload := createTestPayload()

	_, err = exporter.RenderPackingList(context.Background(), payload)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "endpoint required")
}

func TestPDFExporter_RenderPackingList_GotenbergError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("Invalid HTML"))
	}))
	defer srv.Close()

	exporter, err := NewPDFExporter(srv.URL, srv.Client())
	require.NoError(t, err)

	payload := createTestPayload()

	_, err = exporter.RenderPackingList(context.Background(), payload)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "gotenberg response 400")
	assert.Contains(t, err.Error(), "Invalid HTML")
}

func TestPDFExporter_RenderPackingList_ContextCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("PDF"))
	}))
	defer srv.Close()

	exporter, err := NewPDFExporter(srv.URL, srv.Client())
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	payload := createTestPayload()

	_, err = exporter.RenderPackingList(ctx, payload)
	require.Error(t, err)
}

func TestBuildPackingListHTML_BasicStructure(t *testing.T) {
	exporter, err := NewPDFExporter("http://localhost", nil)
	require.NoError(t, err)

	payload := createTestPayload()
	html, err := exporter.buildPackingListHTML(payload)
	require.NoError(t, err)

	assert.Contains(t, html, "<!DOCTYPE html>")
	assert.Contains(t, html, "<html lang=\"en\">")
	assert.Contains(t, html, "</html>")
	assert.Contains(t, html, "<head>")
	assert.Contains(t, html, "</head>")
	assert.Contains(t, html, "<body>")
	assert.Contains(t, html, "</body>")

	assert.Contains(t, html, "<title>Packing List - DO-202501-0001</title>")

	assert.Contains(t, html, "<style>")
	assert.Contains(t, html, ".header")
	assert.Contains(t, html, ".items-table")
	assert.Contains(t, html, ".status-badge")
}

func TestBuildPackingListHTML_HeaderInformation(t *testing.T) {
	exporter, err := NewPDFExporter("http://localhost", nil)
	require.NoError(t, err)

	payload := createTestPayload()
	html, err := exporter.buildPackingListHTML(payload)
	require.NoError(t, err)

	assert.Contains(t, html, "PACKING LIST")
	assert.Contains(t, html, "Delivery Order Document")
}

func TestBuildPackingListHTML_DocumentInformation(t *testing.T) {
	exporter, err := NewPDFExporter("http://localhost", nil)
	require.NoError(t, err)

	payload := createTestPayload()
	html, err := exporter.buildPackingListHTML(payload)
	require.NoError(t, err)

	assert.Contains(t, html, "DO-202501-0001")
	assert.Contains(t, html, "SO-202501-0100")
	assert.Contains(t, html, "January 15, 2025")
}

func TestBuildPackingListHTML_CustomerInformation(t *testing.T) {
	exporter, err := NewPDFExporter("http://localhost", nil)
	require.NoError(t, err)

	payload := createTestPayload()
	html, err := exporter.buildPackingListHTML(payload)
	require.NoError(t, err)

	assert.Contains(t, html, "Acme Corporation")
	assert.Contains(t, html, "123 Business St")
	assert.Contains(t, html, "New York, NY 10001")
}

func TestBuildPackingListHTML_ShippingInformation(t *testing.T) {
	exporter, err := NewPDFExporter("http://localhost", nil)
	require.NoError(t, err)

	payload := createTestPayload()
	html, err := exporter.buildPackingListHTML(payload)
	require.NoError(t, err)

	assert.Contains(t, html, "Main Warehouse")
	assert.Contains(t, html, "FedEx")
	assert.Contains(t, html, "TRACK-12345")
}

func TestBuildPackingListHTML_LineItems(t *testing.T) {
	exporter, err := NewPDFExporter("http://localhost", nil)
	require.NoError(t, err)

	payload := createTestPayload()
	html, err := exporter.buildPackingListHTML(payload)
	require.NoError(t, err)

	assert.Contains(t, html, "<table class=\"items-table\">")
	assert.Contains(t, html, "<thead>")
	assert.Contains(t, html, "<tbody>")

	assert.Contains(t, html, "Product Code")
	assert.Contains(t, html, "Product Name")
	assert.Contains(t, html, "Quantity")
	assert.Contains(t, html, "UOM")

	assert.Contains(t, html, "WIDGET-A")
	assert.Contains(t, html, "Premium Widget Model A")
	assert.Contains(t, html, "50")
	assert.Contains(t, html, "PCS")
	assert.Contains(t, html, "BATCH-2025-001")

	assert.Contains(t, html, "WIDGET-B")
	assert.Contains(t, html, "Deluxe Widget Model B")
	assert.Contains(t, html, "25")
}

func TestBuildPackingListHTML_StatusBadges(t *testing.T) {
	exporter, err := NewPDFExporter("http://localhost", nil)
	require.NoError(t, err)

	statuses := []struct {
		status   string
		expected string
	}{
		{"DRAFT", "status-draft"},
		{"CONFIRMED", "status-confirmed"},
		{"IN_TRANSIT", "status-in-transit"},
		{"DELIVERED", "status-delivered"},
		{"CANCELLED", "status-cancelled"},
	}

	for _, tc := range statuses {
		t.Run(tc.status, func(t *testing.T) {
			payload := createTestPayload()
			payload.Status = tc.status
			html, err := exporter.buildPackingListHTML(payload)
			require.NoError(t, err)

			assert.Contains(t, html, tc.expected)
			assert.Contains(t, html, "status-badge")
		})
	}
}

func TestBuildPackingListHTML_WithShippingNotes(t *testing.T) {
	exporter, err := NewPDFExporter("http://localhost", nil)
	require.NoError(t, err)

	payload := createTestPayload()
	notes := "Handle with care - fragile items"
	payload.ShippingNotes = &notes

	html, err := exporter.buildPackingListHTML(payload)
	require.NoError(t, err)

	assert.Contains(t, html, "Shipping Notes")
	assert.Contains(t, html, "Handle with care - fragile items")
}

func TestBuildPackingListHTML_WithDeliveryNotes(t *testing.T) {
	exporter, err := NewPDFExporter("http://localhost", nil)
	require.NoError(t, err)

	payload := createTestPayload()
	notes := "Left with receptionist"
	payload.DeliveryNotes = &notes

	html, err := exporter.buildPackingListHTML(payload)
	require.NoError(t, err)

	assert.Contains(t, html, "Delivery Notes")
	assert.Contains(t, html, "Left with receptionist")
}

func TestBuildPackingListHTML_SignatureArea(t *testing.T) {
	exporter, err := NewPDFExporter("http://localhost", nil)
	require.NoError(t, err)

	payload := createTestPayload()
	html, err := exporter.buildPackingListHTML(payload)
	require.NoError(t, err)

	assert.Contains(t, html, "Prepared By")
	assert.Contains(t, html, "John Doe")
	assert.Contains(t, html, "Received By")
}

func TestBuildPackingListHTML_WithReceivedBy(t *testing.T) {
	exporter, err := NewPDFExporter("http://localhost", nil)
	require.NoError(t, err)

	payload := createTestPayload()
	receivedBy := "Jane Smith"
	payload.ReceivedBy = &receivedBy

	html, err := exporter.buildPackingListHTML(payload)
	require.NoError(t, err)

	assert.Contains(t, html, "Jane Smith")
}

func TestBuildPackingListHTML_Footer(t *testing.T) {
	exporter, err := NewPDFExporter("http://localhost", nil)
	require.NoError(t, err)

	payload := createTestPayload()
	html, err := exporter.buildPackingListHTML(payload)
	require.NoError(t, err)

	assert.Contains(t, html, "computer-generated packing list")
	assert.Contains(t, html, "Generated:")
}

func TestBuildPackingListHTML_HTMLAutoEscaping(t *testing.T) {
	exporter, err := NewPDFExporter("http://localhost", nil)
	require.NoError(t, err)

	payload := createTestPayload()
	payload.CustomerName = "<script>alert('xss')</script>"
	payload.ShippingAddress = "Address & Co. <tag>"

	html, err := exporter.buildPackingListHTML(payload)
	require.NoError(t, err)

	assert.NotContains(t, html, "<script>alert")
	assert.Contains(t, html, "&lt;script&gt;")
	assert.Contains(t, html, "&amp;")
	assert.NotContains(t, html, "<tag>")
}

func TestBuildPackingListHTML_EmptyLines(t *testing.T) {
	exporter, err := NewPDFExporter("http://localhost", nil)
	require.NoError(t, err)

	payload := createTestPayload()
	payload.Lines = []PackingListLine{}

	html, err := exporter.buildPackingListHTML(payload)
	require.NoError(t, err)

	assert.Contains(t, html, "<table class=\"items-table\">")
	assert.Contains(t, html, "<thead>")
	assert.Contains(t, html, "<tbody>")
	assert.Contains(t, html, "</tbody>")
	assert.Contains(t, html, "No items.")
}

func TestBuildPackingListHTML_NilOptionalFields(t *testing.T) {
	exporter, err := NewPDFExporter("http://localhost", nil)
	require.NoError(t, err)

	payload := PackingListPayload{
		DocNumber:        "DO-TEST",
		SalesOrderNumber: "SO-TEST",
		CustomerName:     "Test Customer",
		PlannedDate:      time.Now(),
		Status:           "DRAFT",
		WarehouseName:    "Test Warehouse",
		ShippingAddress:  "Test Address",
		CreatedBy:        "Test User",
		CreatedAt:        time.Now(),
		Lines:            []PackingListLine{},
		ActualShipDate:   nil,
		TrackingNumber:   nil,
		Carrier:          nil,
		ShippingNotes:    nil,
		DeliveryNotes:    nil,
		ReceivedBy:       nil,
	}

	html, err := exporter.buildPackingListHTML(payload)
	require.NoError(t, err)

	assert.Contains(t, html, "DO-TEST")
	assert.Contains(t, html, "SO-TEST")
}

func TestBuildPackingListHTML_LineWithAllOptionalFields(t *testing.T) {
	exporter, err := NewPDFExporter("http://localhost", nil)
	require.NoError(t, err)

	payload := createTestPayload()
	batch := "BATCH-XYZ"
	serial := "SERIAL-123"
	notes := "Special handling required"

	payload.Lines[0].BatchNumber = &batch
	payload.Lines[0].SerialNumber = &serial
	payload.Lines[0].Notes = &notes

	html, err := exporter.buildPackingListHTML(payload)
	require.NoError(t, err)

	assert.Contains(t, html, "BATCH-XYZ")
	assert.Contains(t, html, "SERIAL-123")
}

func TestBuildPackingListHTML_LongContent(t *testing.T) {
	exporter, err := NewPDFExporter("http://localhost", nil)
	require.NoError(t, err)

	payload := createTestPayload()
	payload.CustomerName = strings.Repeat("Very Long Company Name ", 10)
	payload.ShippingAddress = strings.Repeat("123 Very Long Street Address Line ", 5)
	longNotes := strings.Repeat("This is a very long note. ", 20)
	payload.ShippingNotes = &longNotes

	html, err := exporter.buildPackingListHTML(payload)
	require.NoError(t, err)

	assert.Contains(t, html, "</html>")
	assert.True(t, len(html) > 1000)
}

func createTestPayload() PackingListPayload {
	plannedDate := time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC)
	shipDate := time.Date(2025, 1, 16, 10, 30, 0, 0, time.UTC)
	createdAt := time.Date(2025, 1, 10, 14, 30, 0, 0, time.UTC)

	trackingNumber := "TRACK-12345"
	carrier := "FedEx"
	shippingNotes := "Handle with care"
	batch := "BATCH-2025-001"

	return PackingListPayload{
		DocNumber:        "DO-202501-0001",
		SalesOrderNumber: "SO-202501-0100",
		CustomerName:     "Acme Corporation",
		PlannedDate:      plannedDate,
		ActualShipDate:   &shipDate,
		Status:           "IN_TRANSIT",
		WarehouseName:    "Main Warehouse",
		ShippingAddress:  "123 Business St\nSuite 100\nNew York, NY 10001\nUSA",
		TrackingNumber:   &trackingNumber,
		Carrier:          &carrier,
		ShippingNotes:    &shippingNotes,
		Lines: []PackingListLine{
			{
				LineNumber:   1,
				ProductCode:  "WIDGET-A",
				ProductName:  "Premium Widget Model A",
				Description:  "High-quality widget with extended warranty",
				Quantity:     50.0,
				UOM:          "PCS",
				BatchNumber:  &batch,
				SerialNumber: nil,
				Notes:        nil,
			},
			{
				LineNumber:   2,
				ProductCode:  "WIDGET-B",
				ProductName:  "Deluxe Widget Model B",
				Description:  "",
				Quantity:     25.0,
				UOM:          "PCS",
				BatchNumber:  nil,
				SerialNumber: nil,
				Notes:        nil,
			},
		},
		ReceivedBy:    nil,
		DeliveryNotes: nil,
		CreatedBy:     "John Doe",
		CreatedAt:     createdAt,
	}
}
