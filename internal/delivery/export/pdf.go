package export

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"github.com/odyssey-erp/odyssey-erp/internal/view"
	"github.com/odyssey-erp/odyssey-erp/web"
)

// PackingListPayload aggregates delivery order data for PDF rendering.
type PackingListPayload struct {
	// Header information
	DocNumber        string
	SalesOrderNumber string
	CustomerName     string
	PlannedDate      time.Time
	ActualShipDate   *time.Time
	Status           string

	// Shipping information
	WarehouseName   string
	ShippingAddress string
	TrackingNumber  *string
	Carrier         *string
	ShippingNotes   *string

	// Line items
	Lines []PackingListLine

	// Footer information
	ReceivedBy    *string
	DeliveryNotes *string
	CreatedBy     string
	CreatedAt     time.Time
}

// PackingListLine represents a single line item in the packing list.
type PackingListLine struct {
	LineNumber   int
	ProductCode  string
	ProductName  string
	Description  string
	Quantity     float64
	UOM          string
	BatchNumber  *string
	SerialNumber *string
	Notes        *string
}

// PDFExporter wraps Gotenberg interactions for delivery order PDF generation.
type PDFExporter struct {
	Endpoint  string
	Client    *http.Client
	templates *template.Template
}

// NewPDFExporter creates a PDFExporter with parsed templates.
func NewPDFExporter(endpoint string, client *http.Client) (*PDFExporter, error) {
	funcMap := template.FuncMap{
		"formatDate": func(t time.Time) string {
			if t.IsZero() {
				return ""
			}
			return t.Format("January 2, 2006")
		},
		"formatDatePtr": func(t *time.Time) string {
			if t == nil || t.IsZero() {
				return ""
			}
			return t.Format("January 2, 2006")
		},
		"formatDateTime": func(t time.Time) string {
			if t.IsZero() {
				return ""
			}
			return t.Format("Jan 2, 2006 15:04")
		},
		"formatQty": func(qty float64) string {
			s := fmt.Sprintf("%.4f", qty)
			s = strings.TrimRight(s, "0")
			s = strings.TrimRight(s, ".")
			return s
		},
		"deref": func(s *string) string {
			if s == nil {
				return ""
			}
			return *s
		},
		"now": func() string {
			return time.Now().Format("January 2, 2006 at 3:04 PM")
		},
		"lower": strings.ToLower,
		"replace": func(s, old, new string) string {
			return strings.ReplaceAll(s, old, new)
		},
	}

	tpl, err := template.New("packing_list_pdf.html").Funcs(funcMap).ParseFS(
		web.Templates, "templates/reports/packing_list_pdf.html",
	)
	if err != nil {
		return nil, fmt.Errorf("parse packing list template: %w", err)
	}

	return &PDFExporter{
		Endpoint:  endpoint,
		Client:    client,
		templates: tpl,
	}, nil
}

// RenderPackingList sends HTML content to Gotenberg and returns the PDF bytes.
func (p *PDFExporter) RenderPackingList(ctx context.Context, payload PackingListPayload) ([]byte, error) {
	if p == nil {
		return nil, fmt.Errorf("pdf exporter not initialized")
	}
	endpoint := strings.TrimRight(p.Endpoint, "/")
	if endpoint == "" {
		return nil, fmt.Errorf("gotenberg endpoint required")
	}
	client := p.Client
	if client == nil {
		client = http.DefaultClient
	}

	// Render HTML from template
	html, err := p.buildPackingListHTML(payload)
	if err != nil {
		return nil, fmt.Errorf("render template: %w", err)
	}

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Add HTML file
	part, err := writer.CreateFormFile("files", "packing-list.html")
	if err != nil {
		return nil, err
	}
	if _, err := io.WriteString(part, html); err != nil {
		return nil, err
	}

	// Add PDF options
	if err := writer.WriteField("paperWidth", "8.5"); err != nil {
		return nil, err
	}
	if err := writer.WriteField("paperHeight", "11"); err != nil {
		return nil, err
	}
	if err := writer.WriteField("marginTop", "0.5"); err != nil {
		return nil, err
	}
	if err := writer.WriteField("marginBottom", "0.5"); err != nil {
		return nil, err
	}
	if err := writer.WriteField("marginLeft", "0.5"); err != nil {
		return nil, err
	}
	if err := writer.WriteField("marginRight", "0.5"); err != nil {
		return nil, err
	}
	if err := writer.WriteField("waitDelay", "100"); err != nil {
		return nil, err
	}

	if err := writer.Close(); err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint+"/forms/chromium/convert/html", body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		return nil, fmt.Errorf("gotenberg response %d: %s", resp.StatusCode, string(data))
	}

	return io.ReadAll(resp.Body)
}

func (p *PDFExporter) buildPackingListHTML(payload PackingListPayload) (string, error) {
	if p.templates == nil {
		return "", fmt.Errorf("templates not initialized")
	}

	buf := &bytes.Buffer{}
	data := view.TemplateData{Data: payload}
	if err := p.templates.ExecuteTemplate(buf, "reports/packing_list_pdf.html", data); err != nil {
		return "", err
	}
	return buf.String(), nil
}
