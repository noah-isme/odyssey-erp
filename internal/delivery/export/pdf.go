package export

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"
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
	Endpoint string
	Client   *http.Client
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

	html := buildPackingListHTML(payload)
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

func buildPackingListHTML(payload PackingListPayload) string {
	var b strings.Builder

	// HTML header with styles
	b.WriteString(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Packing List - `)
	b.WriteString(escapeHTML(payload.DocNumber))
	b.WriteString(`</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body {
            font-family: 'Arial', 'Helvetica', sans-serif;
            font-size: 11pt;
            line-height: 1.4;
            color: #333;
            padding: 20px;
        }
        .header {
            margin-bottom: 24px;
            border-bottom: 3px solid #2c3e50;
            padding-bottom: 16px;
        }
        .header h1 {
            font-size: 24pt;
            color: #2c3e50;
            margin-bottom: 4px;
        }
        .header .subtitle {
            font-size: 10pt;
            color: #7f8c8d;
        }
        .info-section {
            display: flex;
            justify-content: space-between;
            margin-bottom: 20px;
            gap: 20px;
        }
        .info-box {
            flex: 1;
            border: 1px solid #ddd;
            padding: 12px;
            background-color: #f8f9fa;
        }
        .info-box h3 {
            font-size: 12pt;
            color: #2c3e50;
            margin-bottom: 8px;
            border-bottom: 1px solid #ddd;
            padding-bottom: 4px;
        }
        .info-row {
            margin-bottom: 4px;
            font-size: 10pt;
        }
        .info-label {
            font-weight: bold;
            display: inline-block;
            width: 120px;
            color: #555;
        }
        .info-value {
            display: inline;
            color: #333;
        }
        .status-badge {
            display: inline-block;
            padding: 4px 12px;
            border-radius: 4px;
            font-size: 9pt;
            font-weight: bold;
            text-transform: uppercase;
        }
        .status-draft { background-color: #95a5a6; color: white; }
        .status-confirmed { background-color: #3498db; color: white; }
        .status-in-transit { background-color: #f39c12; color: white; }
        .status-delivered { background-color: #27ae60; color: white; }
        .status-cancelled { background-color: #e74c3c; color: white; }
        .items-table {
            width: 100%;
            border-collapse: collapse;
            margin-bottom: 20px;
        }
        .items-table thead {
            background-color: #34495e;
            color: white;
        }
        .items-table th {
            padding: 10px 8px;
            text-align: left;
            font-size: 10pt;
            font-weight: bold;
        }
        .items-table th.text-right,
        .items-table td.text-right {
            text-align: right;
        }
        .items-table th.text-center,
        .items-table td.text-center {
            text-align: center;
        }
        .items-table tbody tr {
            border-bottom: 1px solid #ddd;
        }
        .items-table tbody tr:nth-child(even) {
            background-color: #f8f9fa;
        }
        .items-table td {
            padding: 8px;
            font-size: 10pt;
        }
        .items-table tbody tr:hover {
            background-color: #e8f4f8;
        }
        .notes-section {
            margin-top: 20px;
            padding: 12px;
            border: 1px solid #ddd;
            background-color: #fffef0;
        }
        .notes-section h3 {
            font-size: 11pt;
            margin-bottom: 8px;
            color: #2c3e50;
        }
        .notes-content {
            font-size: 10pt;
            color: #555;
            white-space: pre-wrap;
        }
        .footer {
            margin-top: 30px;
            padding-top: 16px;
            border-top: 2px solid #ddd;
            font-size: 9pt;
            color: #7f8c8d;
        }
        .signature-area {
            margin-top: 40px;
            display: flex;
            justify-content: space-between;
        }
        .signature-box {
            flex: 1;
            text-align: center;
            padding: 20px;
        }
        .signature-line {
            border-top: 1px solid #333;
            margin-top: 50px;
            padding-top: 8px;
            font-size: 10pt;
            color: #555;
        }
        @media print {
            body { padding: 0; }
            .items-table { page-break-inside: avoid; }
        }
    </style>
</head>
<body>
`)

	// Header section
	b.WriteString(`    <div class="header">
        <h1>PACKING LIST</h1>
        <div class="subtitle">Delivery Order Document</div>
    </div>

`)

	// Document information section
	b.WriteString(`    <div class="info-section">
        <div class="info-box">
            <h3>Document Information</h3>
            <div class="info-row">
                <span class="info-label">Document No:</span>
                <span class="info-value">`)
	b.WriteString(escapeHTML(payload.DocNumber))
	b.WriteString(`</span>
            </div>
            <div class="info-row">
                <span class="info-label">Sales Order:</span>
                <span class="info-value">`)
	b.WriteString(escapeHTML(payload.SalesOrderNumber))
	b.WriteString(`</span>
            </div>
            <div class="info-row">
                <span class="info-label">Status:</span>
                <span class="info-value">`)
	writeStatusBadge(&b, payload.Status)
	b.WriteString(`</span>
            </div>
            <div class="info-row">
                <span class="info-label">Planned Date:</span>
                <span class="info-value">`)
	b.WriteString(escapeHTML(payload.PlannedDate.Format("January 2, 2006")))
	b.WriteString(`</span>
            </div>
`)
	if payload.ActualShipDate != nil {
		b.WriteString(`            <div class="info-row">
                <span class="info-label">Ship Date:</span>
                <span class="info-value">`)
		b.WriteString(escapeHTML(payload.ActualShipDate.Format("January 2, 2006")))
		b.WriteString(`</span>
            </div>
`)
	}
	b.WriteString(`        </div>

        <div class="info-box">
            <h3>Customer Information</h3>
            <div class="info-row">
                <span class="info-label">Customer:</span>
                <span class="info-value">`)
	b.WriteString(escapeHTML(payload.CustomerName))
	b.WriteString(`</span>
            </div>
            <div class="info-row">
                <span class="info-label">Ship To:</span>
                <span class="info-value" style="white-space: pre-wrap;">`)
	b.WriteString(escapeHTML(payload.ShippingAddress))
	b.WriteString(`</span>
            </div>
        </div>

        <div class="info-box">
            <h3>Shipping Information</h3>
            <div class="info-row">
                <span class="info-label">Warehouse:</span>
                <span class="info-value">`)
	b.WriteString(escapeHTML(payload.WarehouseName))
	b.WriteString(`</span>
            </div>
`)
	if payload.Carrier != nil && *payload.Carrier != "" {
		b.WriteString(`            <div class="info-row">
                <span class="info-label">Carrier:</span>
                <span class="info-value">`)
		b.WriteString(escapeHTML(*payload.Carrier))
		b.WriteString(`</span>
            </div>
`)
	}
	if payload.TrackingNumber != nil && *payload.TrackingNumber != "" {
		b.WriteString(`            <div class="info-row">
                <span class="info-label">Tracking:</span>
                <span class="info-value">`)
		b.WriteString(escapeHTML(*payload.TrackingNumber))
		b.WriteString(`</span>
            </div>
`)
	}
	b.WriteString(`        </div>
    </div>

`)

	// Items table
	b.WriteString(`    <table class="items-table">
        <thead>
            <tr>
                <th class="text-center" style="width: 40px;">#</th>
                <th style="width: 120px;">Product Code</th>
                <th>Product Name</th>
                <th class="text-right" style="width: 80px;">Quantity</th>
                <th class="text-center" style="width: 60px;">UOM</th>
                <th style="width: 120px;">Batch/Serial</th>
            </tr>
        </thead>
        <tbody>
`)

	for _, line := range payload.Lines {
		b.WriteString(`            <tr>
                <td class="text-center">`)
		b.WriteString(fmt.Sprintf("%d", line.LineNumber))
		b.WriteString(`</td>
                <td>`)
		b.WriteString(escapeHTML(line.ProductCode))
		b.WriteString(`</td>
                <td>
                    <strong>`)
		b.WriteString(escapeHTML(line.ProductName))
		b.WriteString(`</strong>`)
		if line.Description != "" {
			b.WriteString(`<br><span style="font-size: 9pt; color: #666;">`)
			b.WriteString(escapeHTML(line.Description))
			b.WriteString(`</span>`)
		}
		b.WriteString(`</td>
                <td class="text-right"><strong>`)
		b.WriteString(formatQuantity(line.Quantity))
		b.WriteString(`</strong></td>
                <td class="text-center">`)
		b.WriteString(escapeHTML(line.UOM))
		b.WriteString(`</td>
                <td>`)
		if line.BatchNumber != nil && *line.BatchNumber != "" {
			b.WriteString(`<div style="font-size: 9pt;">Batch: `)
			b.WriteString(escapeHTML(*line.BatchNumber))
			b.WriteString(`</div>`)
		}
		if line.SerialNumber != nil && *line.SerialNumber != "" {
			b.WriteString(`<div style="font-size: 9pt;">Serial: `)
			b.WriteString(escapeHTML(*line.SerialNumber))
			b.WriteString(`</div>`)
		}
		b.WriteString(`</td>
            </tr>
`)
	}

	b.WriteString(`        </tbody>
    </table>

`)

	// Notes section
	if payload.ShippingNotes != nil && *payload.ShippingNotes != "" {
		b.WriteString(`    <div class="notes-section">
        <h3>Shipping Notes</h3>
        <div class="notes-content">`)
		b.WriteString(escapeHTML(*payload.ShippingNotes))
		b.WriteString(`</div>
    </div>

`)
	}

	if payload.DeliveryNotes != nil && *payload.DeliveryNotes != "" {
		b.WriteString(`    <div class="notes-section">
        <h3>Delivery Notes</h3>
        <div class="notes-content">`)
		b.WriteString(escapeHTML(*payload.DeliveryNotes))
		b.WriteString(`</div>
    </div>

`)
	}

	// Signature area
	b.WriteString(`    <div class="signature-area">
        <div class="signature-box">
            <div>Prepared By</div>
            <div class="signature-line">`)
	b.WriteString(escapeHTML(payload.CreatedBy))
	b.WriteString(`</div>
            <div style="font-size: 9pt; color: #999; margin-top: 4px;">`)
	b.WriteString(escapeHTML(payload.CreatedAt.Format("Jan 2, 2006 15:04")))
	b.WriteString(`</div>
        </div>
        <div class="signature-box">
            <div>Received By</div>
            <div class="signature-line">`)
	if payload.ReceivedBy != nil && *payload.ReceivedBy != "" {
		b.WriteString(escapeHTML(*payload.ReceivedBy))
	} else {
		b.WriteString("&nbsp;")
	}
	b.WriteString(`</div>
            <div style="font-size: 9pt; color: #999; margin-top: 4px;">Signature & Date</div>
        </div>
    </div>

`)

	// Footer
	b.WriteString(`    <div class="footer">
        <p>This is a computer-generated packing list. Please verify all items upon receipt.</p>
        <p>For questions or discrepancies, contact customer service immediately.</p>
        <p style="margin-top: 8px;">Generated: `)
	b.WriteString(escapeHTML(time.Now().Format("January 2, 2006 at 3:04 PM")))
	b.WriteString(`</p>
    </div>

</body>
</html>`)

	return b.String()
}

func writeStatusBadge(b *strings.Builder, status string) {
	statusLower := strings.ToLower(status)
	var class string
	switch statusLower {
	case "draft":
		class = "status-draft"
	case "confirmed":
		class = "status-confirmed"
	case "in_transit":
		class = "status-in-transit"
	case "delivered":
		class = "status-delivered"
	case "cancelled":
		class = "status-cancelled"
	default:
		class = "status-draft"
	}

	displayStatus := strings.ReplaceAll(status, "_", " ")
	b.WriteString(`<span class="status-badge `)
	b.WriteString(class)
	b.WriteString(`">`)
	b.WriteString(escapeHTML(displayStatus))
	b.WriteString(`</span>`)
}

func formatQuantity(qty float64) string {
	// Remove trailing zeros
	s := fmt.Sprintf("%.4f", qty)
	s = strings.TrimRight(s, "0")
	s = strings.TrimRight(s, ".")
	return s
}

func escapeHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	s = strings.ReplaceAll(s, "'", "&#39;")
	return s
}

func formatFloat(value float64) string {
	return fmt.Sprintf("%.2f", value)
}
