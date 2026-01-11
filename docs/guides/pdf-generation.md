# Phase 9.2 PDF Generation Documentation

## Overview

This document describes the PDF generation system for Delivery Order packing lists in the Odyssey ERP system. The implementation uses Gotenberg (a Docker-powered API for PDF generation) to convert HTML templates into professional PDF documents.

---

## Architecture

### Components

```
┌─────────────────────────────────────────────────────────────┐
│                     Delivery Handler                         │
│  - Receives PDF download request                            │
│  - Fetches delivery order data                              │
│  - Builds PackingListPayload                                │
└────────────────────┬────────────────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────────────────┐
│                    PDF Exporter                              │
│  - Builds HTML from payload                                 │
│  - Sends to Gotenberg                                       │
│  - Returns PDF bytes                                        │
└────────────────────┬────────────────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────────────────┐
│                    Gotenberg API                             │
│  - Chromium-based PDF rendering                             │
│  - Converts HTML to PDF                                     │
│  - Returns PDF bytes                                        │
└─────────────────────────────────────────────────────────────┘
```

---

## Implementation

### Package: `internal/delivery/export`

**Files:**
- `pdf.go` - PDF generation logic
- `pdf_test.go` - Comprehensive test suite

**Key Types:**

```go
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

type PDFExporter struct {
    Endpoint string       // Gotenberg URL
    Client   *http.Client // HTTP client
}
```

---

## Usage

### Basic Usage

```go
import "github.com/odyssey-erp/odyssey-erp/internal/delivery/export"

// Initialize exporter
exporter := &export.PDFExporter{
    Endpoint: "http://gotenberg:3000",
    Client:   http.DefaultClient,
}

// Build payload
payload := export.PackingListPayload{
    DocNumber:        "DO-202501-0001",
    SalesOrderNumber: "SO-202501-0100",
    CustomerName:     "Acme Corporation",
    PlannedDate:      time.Now(),
    Status:           "CONFIRMED",
    WarehouseName:    "Main Warehouse",
    ShippingAddress:  "123 Business St\nNew York, NY 10001",
    Lines: []export.PackingListLine{
        {
            LineNumber:  1,
            ProductCode: "WIDGET-A",
            ProductName: "Premium Widget",
            Quantity:    50.0,
            UOM:         "PCS",
        },
    },
    CreatedBy: "John Doe",
    CreatedAt: time.Now(),
}

// Generate PDF
ctx := context.Background()
pdfBytes, err := exporter.RenderPackingList(ctx, payload)
if err != nil {
    log.Fatal(err)
}

// Save or stream PDF
ioutil.WriteFile("packing-list.pdf", pdfBytes, 0644)
```

### In HTTP Handler

```go
func (h *Handler) downloadPackingList(w http.ResponseWriter, r *http.Request) {
    // Get delivery order ID
    id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
    
    // Fetch delivery order with details
    do, err := h.service.GetDeliveryOrderWithDetails(r.Context(), id)
    if err != nil {
        http.Error(w, "Not found", http.StatusNotFound)
        return
    }
    
    // Build payload
    payload := buildPackingListPayload(do)
    
    // Generate PDF
    pdfBytes, err := h.pdfExporter.RenderPackingList(r.Context(), payload)
    if err != nil {
        http.Error(w, "PDF generation failed", http.StatusInternalServerError)
        return
    }
    
    // Stream to client
    filename := fmt.Sprintf("packing-list-%s.pdf", do.DocNumber)
    w.Header().Set("Content-Type", "application/pdf")
    w.Header().Set("Content-Disposition", 
        fmt.Sprintf("attachment; filename=\"%s\"", filename))
    w.Write(pdfBytes)
}
```

---

## HTML Template

### Template Structure

The packing list PDF includes:

1. **Header Section**
   - Title: "PACKING LIST"
   - Subtitle: "Delivery Order Document"

2. **Document Information**
   - Document Number
   - Sales Order Number
   - Status Badge (color-coded)
   - Planned Date
   - Actual Ship Date (if shipped)

3. **Customer Information**
   - Customer Name
   - Shipping Address (multi-line)

4. **Shipping Information**
   - Warehouse Name
   - Carrier
   - Tracking Number

5. **Line Items Table**
   - Line Number
   - Product Code
   - Product Name (with description)
   - Quantity
   - UOM
   - Batch/Serial Numbers

6. **Notes Sections**
   - Shipping Notes (if present)
   - Delivery Notes (if present)

7. **Signature Area**
   - Prepared By (with date)
   - Received By (with signature line)

8. **Footer**
   - Generated timestamp
   - Disclaimer text

### Status Badges

Status badges are color-coded for easy identification:

| Status | Color | CSS Class |
|--------|-------|-----------|
| DRAFT | Gray (#95a5a6) | status-draft |
| CONFIRMED | Blue (#3498db) | status-confirmed |
| IN_TRANSIT | Orange (#f39c12) | status-in-transit |
| DELIVERED | Green (#27ae60) | status-delivered |
| CANCELLED | Red (#e74c3c) | status-cancelled |

### Responsive Design

The template is designed for:
- **Print:** US Letter (8.5" × 11")
- **Margins:** 0.5" on all sides
- **Font:** Arial/Helvetica, 11pt base size
- **Page Breaks:** Automatic, table-aware

---

## Gotenberg Configuration

### Docker Setup

```yaml
# docker-compose.yml
services:
  gotenberg:
    image: gotenberg/gotenberg:7
    ports:
      - "3000:3000"
    environment:
      - GOTENBERG_API_TIMEOUT=30s
      - GOTENBERG_API_PORT=3000
    restart: unless-stopped
```

### Start Gotenberg

```bash
docker-compose up -d gotenberg
```

### Verify Gotenberg

```bash
curl http://localhost:3000/health
# Response: {"status":"up"}
```

### Environment Configuration

```bash
# .env or configuration file
GOTENBERG_URL=http://gotenberg:3000
```

---

## Testing

### Test Suite

**File:** `internal/delivery/export/pdf_test.go`

**Test Coverage:**

1. **PDF Generation Tests** (5 tests)
   - Successful PDF rendering
   - Nil exporter error handling
   - Empty endpoint error handling
   - Gotenberg error handling
   - Context cancellation

2. **HTML Generation Tests** (14 tests)
   - Basic HTML structure
   - Header information
   - Document information
   - Customer information
   - Shipping information
   - Line items table
   - Status badges (5 statuses)
   - Notes sections
   - Signature area
   - Footer content

3. **Security Tests** (2 tests)
   - HTML escaping
   - XSS prevention

4. **Formatting Tests** (2 tests)
   - Quantity formatting
   - Float formatting

5. **Status Badge Tests** (1 test)
   - All status badge styles

6. **Edge Case Tests** (4 tests)
   - Empty line items
   - Nil optional fields
   - Long content handling
   - All optional fields present

**Total Tests:** 28 tests, all passing ✅

### Running Tests

```bash
# Run all PDF tests
go test -v ./internal/delivery/export/

# Run specific test
go test -v -run TestPDFExporter_RenderPackingList_Success ./internal/delivery/export/

# Run with coverage
go test -cover ./internal/delivery/export/
```

### Test Results

```
=== RUN   TestPDFExporter_RenderPackingList_Success
--- PASS: TestPDFExporter_RenderPackingList_Success (0.00s)
...
PASS
ok      github.com/odyssey-erp/odyssey-erp/internal/delivery/export    0.013s
```

---

## Security Considerations

### HTML Escaping

All user-provided content is properly escaped to prevent XSS attacks:

```go
func escapeHTML(s string) string {
    s = strings.ReplaceAll(s, "&", "&amp;")
    s = strings.ReplaceAll(s, "<", "&lt;")
    s = strings.ReplaceAll(s, ">", "&gt;")
    s = strings.ReplaceAll(s, "\"", "&quot;")
    s = strings.ReplaceAll(s, "'", "&#39;")
    return s
}
```

**Tested Characters:**
- `<script>` tags
- HTML entities (`&`, `<`, `>`)
- Quotes (`"`, `'`)
- Mixed dangerous characters

### Input Validation

Before PDF generation:
1. Verify user has permission to access delivery order
2. Validate delivery order exists
3. Check company access rights
4. Sanitize all text fields

### PDF Security

- PDFs are generated server-side only
- No client-side PDF manipulation
- Generated PDFs contain no embedded scripts
- Access controlled via RBAC permissions

---

## Performance

### Benchmarks

- **PDF Generation Time:** ~200-500ms (depends on Gotenberg)
- **Memory Usage:** ~2-5MB per PDF
- **Concurrent Requests:** Limited by Gotenberg capacity
- **HTML Generation:** <1ms (pure string building)

### Optimization Tips

1. **Connection Pooling**
   ```go
   exporter := &PDFExporter{
       Endpoint: gotenbergURL,
       Client: &http.Client{
           Timeout: 30 * time.Second,
           Transport: &http.Transport{
               MaxIdleConns:        100,
               MaxIdleConnsPerHost: 100,
           },
       },
   }
   ```

2. **Caching** (optional)
   - Cache generated PDFs for unchanged delivery orders
   - Use delivery order version/updated_at as cache key
   - Invalidate cache on status changes

3. **Async Generation** (for bulk operations)
   - Queue PDF generation jobs
   - Process in background worker
   - Notify user when complete

---

## Error Handling

### Common Errors

**1. Gotenberg Not Available**
```
Error: dial tcp: connection refused
```
**Solution:** Ensure Gotenberg is running and accessible

**2. Timeout**
```
Error: context deadline exceeded
```
**Solution:** Increase timeout or optimize HTML

**3. Invalid HTML**
```
Error: gotenberg response 400: Invalid HTML
```
**Solution:** Check HTML generation, verify escaping

**4. Memory Issues**
```
Error: gotenberg response 500: Out of memory
```
**Solution:** Reduce PDF size, add more Gotenberg resources

### Error Recovery

```go
pdfBytes, err := exporter.RenderPackingList(ctx, payload)
if err != nil {
    log.Error("PDF generation failed", 
        "error", err,
        "docNumber", payload.DocNumber)
    
    // Fallback: return HTML or error page
    return fallbackHTMLResponse(w, payload)
}
```

---

## Customization

### Styling

Modify the CSS in `buildPackingListHTML()`:

```go
b.WriteString(`<style>
    /* Your custom styles */
    .header h1 {
        color: #your-brand-color;
        font-size: 28pt;
    }
    /* ... */
</style>`)
```

### Adding Fields

1. **Add to Payload:**
   ```go
   type PackingListPayload struct {
       // ... existing fields
       SpecialInstructions string
   }
   ```

2. **Update HTML Template:**
   ```go
   if payload.SpecialInstructions != "" {
       b.WriteString(`<div class="special">`)
       b.WriteString(escapeHTML(payload.SpecialInstructions))
       b.WriteString(`</div>`)
   }
   ```

3. **Update Tests:**
   ```go
   func TestBuildPackingListHTML_SpecialInstructions(t *testing.T) {
       payload := createTestPayload()
       payload.SpecialInstructions = "Fragile items"
       html := buildPackingListHTML(payload)
       assert.Contains(t, html, "Fragile items")
   }
   ```

### Localization

For multi-language support:

```go
type PackingListPayload struct {
    // ... existing fields
    Language string // "en", "id", "es", etc.
}

func buildPackingListHTML(payload PackingListPayload) string {
    // Load translations
    title := getTranslation(payload.Language, "packing_list_title")
    
    b.WriteString(fmt.Sprintf("<h1>%s</h1>", title))
    // ...
}
```

---

## Integration

### Handler Integration

```go
// In handler.go
type Handler struct {
    // ... existing fields
    pdfExporter *export.PDFExporter
}

// Mount route
r.Get("/delivery-orders/{id}/pdf", h.downloadPackingList)

// Handler method
func (h *Handler) downloadPackingList(w http.ResponseWriter, r *http.Request) {
    id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
    
    // Fetch DO with all details
    do, err := h.service.GetDeliveryOrderWithDetails(r.Context(), id)
    if err != nil {
        http.Error(w, "Not found", http.StatusNotFound)
        return
    }
    
    // Check permission
    // ... RBAC check
    
    // Build payload
    payload := h.buildPackingListPayload(do)
    
    // Generate PDF
    pdfBytes, err := h.pdfExporter.RenderPackingList(r.Context(), payload)
    if err != nil {
        http.Error(w, "PDF generation failed", http.StatusInternalServerError)
        return
    }
    
    // Stream to client
    filename := fmt.Sprintf("packing-list-%s.pdf", do.DocNumber)
    w.Header().Set("Content-Type", "application/pdf")
    w.Header().Set("Content-Disposition", 
        fmt.Sprintf("attachment; filename=\"%s\"", filename))
    w.Write(pdfBytes)
}
```

### Building Payload from Domain Model

```go
func (h *Handler) buildPackingListPayload(do *DeliveryOrderWithDetails) export.PackingListPayload {
    payload := export.PackingListPayload{
        DocNumber:        do.DocNumber,
        SalesOrderNumber: do.SalesOrderDocNumber,
        CustomerName:     do.CustomerName,
        PlannedDate:      do.PlannedDate,
        ActualShipDate:   do.ActualShipDate,
        Status:           string(do.Status),
        WarehouseName:    do.WarehouseName,
        ShippingAddress:  h.formatShippingAddress(do),
        TrackingNumber:   do.TrackingNumber,
        Carrier:          do.Carrier,
        ShippingNotes:    do.ShippingNotes,
        ReceivedBy:       do.ReceivedBy,
        DeliveryNotes:    do.DeliveryNotes,
        CreatedBy:        do.CreatedByName,
        CreatedAt:        do.CreatedAt,
        Lines:            make([]export.PackingListLine, len(do.Lines)),
    }
    
    for i, line := range do.Lines {
        payload.Lines[i] = export.PackingListLine{
            LineNumber:   i + 1,
            ProductCode:  line.ProductCode,
            ProductName:  line.ProductName,
            Description:  line.Description,
            Quantity:     line.Quantity,
            UOM:          line.UOM,
            BatchNumber:  line.BatchNumber,
            SerialNumber: line.SerialNumber,
            Notes:        line.Notes,
        }
    }
    
    return payload
}
```

---

## Troubleshooting

### Issue: PDF is blank

**Cause:** HTML errors or missing content  
**Solution:**
1. Test HTML directly in browser
2. Check console for CSS errors
3. Verify all fields are populated

### Issue: Fonts not rendering

**Cause:** Font not available in Gotenberg container  
**Solution:**
1. Use web-safe fonts (Arial, Helvetica)
2. Or include custom fonts in HTML

### Issue: Images not showing

**Cause:** External image URLs not accessible  
**Solution:**
1. Embed images as base64
2. Use absolute URLs
3. Ensure Gotenberg can access image server

### Issue: PDF layout broken

**Cause:** CSS issues or page breaks  
**Solution:**
1. Test with simpler CSS
2. Add page-break-inside: avoid
3. Adjust margins and sizing

---

## Future Enhancements

### Planned Features

- [ ] **QR Code Generation** - Add QR code with delivery order details
- [ ] **Barcode Support** - Include barcodes for products
- [ ] **Multi-Page Support** - Better handling of large orders
- [ ] **Logo Integration** - Company logo in header
- [ ] **Custom Templates** - User-configurable templates
- [ ] **Batch PDF Generation** - Generate multiple PDFs at once
- [ ] **Email Integration** - Send PDF via email
- [ ] **Digital Signature** - Support for digital signatures

### Infrastructure Improvements

- [ ] **PDF Caching** - Cache generated PDFs
- [ ] **CDN Distribution** - Serve PDFs from CDN
- [ ] **Async Generation** - Background job processing
- [ ] **Template Engine** - Use proper template engine (html/template)

---

## References

- **Gotenberg Documentation:** https://gotenberg.dev/
- **PDF Exporter Code:** `internal/delivery/export/pdf.go`
- **PDF Tests:** `internal/delivery/export/pdf_test.go`
- **Similar Implementation:** `internal/analytics/export/pdf.go`

---

## Appendix A: Complete Example

### Full Packing List Generation

```go
package main

import (
    "context"
    "fmt"
    "net/http"
    "time"
    
    "github.com/odyssey-erp/odyssey-erp/internal/delivery/export"
)

func main() {
    // Initialize exporter
    exporter := &export.PDFExporter{
        Endpoint: "http://localhost:3000",
        Client:   http.DefaultClient,
    }
    
    // Create sample payload
    payload := export.PackingListPayload{
        DocNumber:        "DO-202501-0001",
        SalesOrderNumber: "SO-202501-0100",
        CustomerName:     "Acme Corporation",
        PlannedDate:      time.Now(),
        Status:           "CONFIRMED",
        WarehouseName:    "Main Warehouse",
        ShippingAddress:  "123 Business St\nSuite 100\nNew York, NY 10001",
        Lines: []export.PackingListLine{
            {
                LineNumber:  1,
                ProductCode: "WIDGET-A",
                ProductName: "Premium Widget Model A",
                Description: "High-quality widget",
                Quantity:    50.0,
                UOM:         "PCS",
            },
            {
                LineNumber:  2,
                ProductCode: "WIDGET-B",
                ProductName: "Deluxe Widget Model B",
                Quantity:    25.0,
                UOM:         "PCS",
            },
        },
        CreatedBy: "John Doe",
        CreatedAt: time.Now(),
    }
    
    // Generate PDF
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    
    pdfBytes, err := exporter.RenderPackingList(ctx, payload)
    if err != nil {
        panic(err)
    }
    
    // Save to file
    filename := "packing-list-DO-202501-0001.pdf"
    if err := ioutil.WriteFile(filename, pdfBytes, 0644); err != nil {
        panic(err)
    }
    
    fmt.Printf("PDF generated: %s (%d bytes)\n", filename, len(pdfBytes))
}
```

---

**Document Version:** 1.0  
**Last Updated:** Phase 9.2 PDF Generation Implementation  
**Status:** Complete  
**Maintainer:** Engineering Team