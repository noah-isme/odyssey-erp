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

// ============================================================================
// PDF EXPORTER TESTS
// ============================================================================

func TestPDFExporter_RenderPackingList_Success(t *testing.T) {
	// Mock Gotenberg server
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		assert.Equal(t, "/forms/chromium/convert/html", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Contains(t, r.Header.Get("Content-Type"), "multipart/form-data")

		// Parse multipart form
		err := r.ParseMultipartForm(10 << 20) // 10 MB
		if err != nil {
			t.Fatalf("failed to parse multipart form: %v", err)
		}

		// Verify HTML file exists
		file, _, err := r.FormFile("files")
		if err != nil {
			t.Fatalf("failed to get files: %v", err)
		}
		defer file.Close()

		htmlContent, err := io.ReadAll(file)
		if err != nil {
			t.Fatalf("failed to read file: %v", err)
		}

		// Verify HTML contains expected content
		html := string(htmlContent)
		assert.Contains(t, html, "PACKING LIST")
		assert.Contains(t, html, "DO-202501-0001")
		assert.Contains(t, html, "Acme Corporation")
		assert.Contains(t, html, "WIDGET-A")

		// Return mock PDF
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("MOCK-PDF-CONTENT"))
	}))
	defer srv.Close()

	exporter := &PDFExporter{
		Endpoint: srv.URL,
		Client:   srv.Client(),
	}

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
	exporter := &PDFExporter{
		Endpoint: "",
	}
	payload := createTestPayload()

	_, err := exporter.RenderPackingList(context.Background(), payload)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "endpoint required")
}

func TestPDFExporter_RenderPackingList_GotenbergError(t *testing.T) {
	// Mock server that returns error
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("Invalid HTML"))
	}))
	defer srv.Close()

	exporter := &PDFExporter{
		Endpoint: srv.URL,
		Client:   srv.Client(),
	}

	payload := createTestPayload()

	_, err := exporter.RenderPackingList(context.Background(), payload)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "gotenberg response 400")
	assert.Contains(t, err.Error(), "Invalid HTML")
}

func TestPDFExporter_RenderPackingList_ContextCancelled(t *testing.T) {
	// Mock server with delay
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("PDF"))
	}))
	defer srv.Close()

	exporter := &PDFExporter{
		Endpoint: srv.URL,
		Client:   srv.Client(),
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	payload := createTestPayload()

	_, err := exporter.RenderPackingList(ctx, payload)
	require.Error(t, err)
}

// ============================================================================
// HTML GENERATION TESTS
// ============================================================================

func TestBuildPackingListHTML_BasicStructure(t *testing.T) {
	payload := createTestPayload()
	html := buildPackingListHTML(payload)

	// Verify HTML structure
	assert.Contains(t, html, "<!DOCTYPE html>")
	assert.Contains(t, html, "<html lang=\"en\">")
	assert.Contains(t, html, "</html>")
	assert.Contains(t, html, "<head>")
	assert.Contains(t, html, "</head>")
	assert.Contains(t, html, "<body>")
	assert.Contains(t, html, "</body>")

	// Verify title
	assert.Contains(t, html, "<title>Packing List - DO-202501-0001</title>")

	// Verify styles are included
	assert.Contains(t, html, "<style>")
	assert.Contains(t, html, ".header")
	assert.Contains(t, html, ".items-table")
	assert.Contains(t, html, ".status-badge")
}

func TestBuildPackingListHTML_HeaderInformation(t *testing.T) {
	payload := createTestPayload()
	html := buildPackingListHTML(payload)

	// Verify header content
	assert.Contains(t, html, "PACKING LIST")
	assert.Contains(t, html, "Delivery Order Document")
}

func TestBuildPackingListHTML_DocumentInformation(t *testing.T) {
	payload := createTestPayload()
	html := buildPackingListHTML(payload)

	// Verify document details
	assert.Contains(t, html, "DO-202501-0001")
	assert.Contains(t, html, "SO-202501-0100")
	assert.Contains(t, html, "January 15, 2025")
}

func TestBuildPackingListHTML_CustomerInformation(t *testing.T) {
	payload := createTestPayload()
	html := buildPackingListHTML(payload)

	// Verify customer details
	assert.Contains(t, html, "Acme Corporation")
	assert.Contains(t, html, "123 Business St")
	assert.Contains(t, html, "New York, NY 10001")
}

func TestBuildPackingListHTML_ShippingInformation(t *testing.T) {
	payload := createTestPayload()
	html := buildPackingListHTML(payload)

	// Verify shipping details
	assert.Contains(t, html, "Main Warehouse")
	assert.Contains(t, html, "FedEx")
	assert.Contains(t, html, "TRACK-12345")
}

func TestBuildPackingListHTML_LineItems(t *testing.T) {
	payload := createTestPayload()
	html := buildPackingListHTML(payload)

	// Verify table structure
	assert.Contains(t, html, "<table class=\"items-table\">")
	assert.Contains(t, html, "<thead>")
	assert.Contains(t, html, "<tbody>")

	// Verify headers
	assert.Contains(t, html, "Product Code")
	assert.Contains(t, html, "Product Name")
	assert.Contains(t, html, "Quantity")
	assert.Contains(t, html, "UOM")

	// Verify line items
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
			html := buildPackingListHTML(payload)

			assert.Contains(t, html, tc.expected)
			assert.Contains(t, html, "status-badge")
		})
	}
}

func TestBuildPackingListHTML_WithShippingNotes(t *testing.T) {
	payload := createTestPayload()
	notes := "Handle with care - fragile items"
	payload.ShippingNotes = &notes

	html := buildPackingListHTML(payload)

	assert.Contains(t, html, "Shipping Notes")
	assert.Contains(t, html, "Handle with care - fragile items")
}

func TestBuildPackingListHTML_WithDeliveryNotes(t *testing.T) {
	payload := createTestPayload()
	notes := "Left with receptionist"
	payload.DeliveryNotes = &notes

	html := buildPackingListHTML(payload)

	assert.Contains(t, html, "Delivery Notes")
	assert.Contains(t, html, "Left with receptionist")
}

func TestBuildPackingListHTML_SignatureArea(t *testing.T) {
	payload := createTestPayload()
	html := buildPackingListHTML(payload)

	// Verify signature section
	assert.Contains(t, html, "Prepared By")
	assert.Contains(t, html, "John Doe")
	assert.Contains(t, html, "Received By")
}

func TestBuildPackingListHTML_WithReceivedBy(t *testing.T) {
	payload := createTestPayload()
	receivedBy := "Jane Smith"
	payload.ReceivedBy = &receivedBy

	html := buildPackingListHTML(payload)

	assert.Contains(t, html, "Jane Smith")
}

func TestBuildPackingListHTML_Footer(t *testing.T) {
	payload := createTestPayload()
	html := buildPackingListHTML(payload)

	// Verify footer content
	assert.Contains(t, html, "computer-generated packing list")
	assert.Contains(t, html, "Generated:")
}

// ============================================================================
// HTML ESCAPING TESTS
// ============================================================================

func TestEscapeHTML(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"<script>alert('xss')</script>", "&lt;script&gt;alert(&#39;xss&#39;)&lt;/script&gt;"},
		{"A & B", "A &amp; B"},
		{"Quote: \"Hello\"", "Quote: &quot;Hello&quot;"},
		{"Normal text", "Normal text"},
		{"Multiple & < > \" '", "Multiple &amp; &lt; &gt; &quot; &#39;"},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			result := escapeHTML(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestBuildPackingListHTML_HTMLEscaping(t *testing.T) {
	payload := createTestPayload()
	payload.CustomerName = "<script>alert('xss')</script>"
	payload.ShippingAddress = "Address & Co. <tag>"

	html := buildPackingListHTML(payload)

	// Verify HTML is escaped
	assert.NotContains(t, html, "<script>")
	assert.Contains(t, html, "&lt;script&gt;")
	assert.Contains(t, html, "&amp;")
	assert.NotContains(t, html, "<tag>")
}

// ============================================================================
// FORMATTING TESTS
// ============================================================================

func TestFormatQuantity(t *testing.T) {
	tests := []struct {
		input    float64
		expected string
	}{
		{50.0, "50"},
		{50.5, "50.5"},
		{50.25, "50.25"},
		{50.1234, "50.1234"},
		{50.1000, "50.1"},
		{0.0, "0"},
		{100.9999, "100.9999"},
	}

	for _, tc := range tests {
		t.Run(tc.expected, func(t *testing.T) {
			result := formatQuantity(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestFormatFloat(t *testing.T) {
	tests := []struct {
		input    float64
		expected string
	}{
		{150.0, "150.00"},
		{150.5, "150.50"},
		{150.99, "150.99"},
		{150.999, "151.00"}, // Rounded
		{0.0, "0.00"},
	}

	for _, tc := range tests {
		t.Run(tc.expected, func(t *testing.T) {
			result := formatFloat(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// ============================================================================
// STATUS BADGE TESTS
// ============================================================================

func TestWriteStatusBadge(t *testing.T) {
	tests := []struct {
		status        string
		expectedClass string
		expectedText  string
	}{
		{"DRAFT", "status-draft", "DRAFT"},
		{"CONFIRMED", "status-confirmed", "CONFIRMED"},
		{"IN_TRANSIT", "status-in-transit", "IN TRANSIT"},
		{"DELIVERED", "status-delivered", "DELIVERED"},
		{"CANCELLED", "status-cancelled", "CANCELLED"},
		{"UNKNOWN", "status-draft", "UNKNOWN"}, // Default to draft class
	}

	for _, tc := range tests {
		t.Run(tc.status, func(t *testing.T) {
			var b strings.Builder
			writeStatusBadge(&b, tc.status)
			result := b.String()

			assert.Contains(t, result, tc.expectedClass)
			assert.Contains(t, result, tc.expectedText)
			assert.Contains(t, result, "status-badge")
		})
	}
}

// ============================================================================
// EDGE CASES AND ERROR HANDLING
// ============================================================================

func TestBuildPackingListHTML_EmptyLines(t *testing.T) {
	payload := createTestPayload()
	payload.Lines = []PackingListLine{}

	html := buildPackingListHTML(payload)

	// Should still generate valid HTML
	assert.Contains(t, html, "<table class=\"items-table\">")
	assert.Contains(t, html, "<thead>")
	assert.Contains(t, html, "<tbody>")
	assert.Contains(t, html, "</tbody>")
}

func TestBuildPackingListHTML_NilOptionalFields(t *testing.T) {
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
		// All optional fields are nil
		ActualShipDate: nil,
		TrackingNumber: nil,
		Carrier:        nil,
		ShippingNotes:  nil,
		DeliveryNotes:  nil,
		ReceivedBy:     nil,
	}

	html := buildPackingListHTML(payload)

	// Should generate valid HTML without errors
	assert.Contains(t, html, "DO-TEST")
	assert.Contains(t, html, "SO-TEST")
	assert.NotContains(t, html, "Ship Date:") // Should not show if nil
}

func TestBuildPackingListHTML_LineWithAllOptionalFields(t *testing.T) {
	payload := createTestPayload()
	batch := "BATCH-XYZ"
	serial := "SERIAL-123"
	notes := "Special handling required"

	payload.Lines[0].BatchNumber = &batch
	payload.Lines[0].SerialNumber = &serial
	payload.Lines[0].Notes = &notes

	html := buildPackingListHTML(payload)

	assert.Contains(t, html, "BATCH-XYZ")
	assert.Contains(t, html, "SERIAL-123")
}

func TestBuildPackingListHTML_LongContent(t *testing.T) {
	payload := createTestPayload()

	// Long customer name
	payload.CustomerName = strings.Repeat("Very Long Company Name ", 10)

	// Long shipping address
	payload.ShippingAddress = strings.Repeat("123 Very Long Street Address Line ", 5)

	// Long notes
	longNotes := strings.Repeat("This is a very long note. ", 20)
	payload.ShippingNotes = &longNotes

	html := buildPackingListHTML(payload)

	// Should handle long content without breaking HTML
	assert.Contains(t, html, "</html>")
	assert.True(t, len(html) > 1000)
}

// ============================================================================
// HELPER FUNCTIONS
// ============================================================================

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
