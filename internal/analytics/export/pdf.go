package export

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"sort"
	"strings"

	"github.com/odyssey-erp/odyssey-erp/internal/analytics"
)

// DashboardPayload aggregates analytics data destined for PDF rendering.
type DashboardPayload struct {
	Period   string
	Summary  analytics.KPISummary
	PL       []analytics.PLTrendPoint
	Cashflow []analytics.CashflowTrendPoint
	ARAging  []analytics.AgingBucket
	APAging  []analytics.AgingBucket
}

// PDFExporter wraps Gotenberg interactions for dashboard exports.
type PDFExporter struct {
	Endpoint string
	Client   *http.Client
}

// RenderDashboard sends HTML content to Gotenberg and returns the PDF bytes.
func (p *PDFExporter) RenderDashboard(ctx context.Context, payload DashboardPayload) ([]byte, error) {
	if p == nil {
		return nil, fmt.Errorf("pdf exporter not initialised")
	}
	endpoint := strings.TrimRight(p.Endpoint, "/")
	if endpoint == "" {
		return nil, fmt.Errorf("gotenberg endpoint required")
	}
	client := p.Client
	if client == nil {
		client = http.DefaultClient
	}

	html := buildHTML(payload)
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("files", "dashboard.html")
	if err != nil {
		return nil, err
	}
	if _, err := io.WriteString(part, html); err != nil {
		return nil, err
	}
	if err := writer.WriteField("waitDelay", "500"); err != nil {
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

func buildHTML(payload DashboardPayload) string {
	var b strings.Builder
	b.WriteString("<html><head><meta charset=\"utf-8\"><style>")
	b.WriteString("body{font-family:sans-serif;margin:24px;}h1{font-size:20px;}table{width:100%;border-collapse:collapse;margin-bottom:16px;}th,td{border:1px solid #ddd;padding:6px;text-align:right;}th{text-align:left;background:#f5f5f5;}section{margin-bottom:24px;} .metric-label{text-align:left;}")
	b.WriteString("</style></head><body>")
	b.WriteString(fmt.Sprintf("<h1>Finance Analytics – %s</h1>", templateEscape(payload.Period)))

	b.WriteString("<section><h2>KPI Summary</h2><table><tbody>")
	writeMetricRow(&b, "Net Profit", payload.Summary.NetProfit)
	writeMetricRow(&b, "Revenue", payload.Summary.Revenue)
	writeMetricRow(&b, "Operating Expense", payload.Summary.Opex)
	writeMetricRow(&b, "Cost of Goods Sold", payload.Summary.COGS)
	writeMetricRow(&b, "Cash In", payload.Summary.CashIn)
	writeMetricRow(&b, "Cash Out", payload.Summary.CashOut)
	writeMetricRow(&b, "AR Outstanding", payload.Summary.AROutstanding)
	writeMetricRow(&b, "AP Outstanding", payload.Summary.APOutstanding)
	b.WriteString("</tbody></table></section>")

	if len(payload.PL) > 0 {
		b.WriteString("<section><h2>P&amp;L Trend</h2><table><thead><tr><th>Period</th><th>Revenue</th><th>COGS</th><th>Opex</th><th>Net</th></tr></thead><tbody>")
		for _, point := range payload.PL {
			b.WriteString("<tr><td class=\"metric-label\">")
			b.WriteString(templateEscape(point.Period))
			b.WriteString("</td><td>")
			b.WriteString(formatFloat(point.Revenue))
			b.WriteString("</td><td>")
			b.WriteString(formatFloat(point.COGS))
			b.WriteString("</td><td>")
			b.WriteString(formatFloat(point.Opex))
			b.WriteString("</td><td>")
			b.WriteString(formatFloat(point.Net))
			b.WriteString("</td></tr>")
		}
		b.WriteString("</tbody></table></section>")
	}

	if len(payload.Cashflow) > 0 {
		b.WriteString("<section><h2>Cashflow Trend</h2><table><thead><tr><th>Period</th><th>Cash In</th><th>Cash Out</th></tr></thead><tbody>")
		for _, point := range payload.Cashflow {
			b.WriteString("<tr><td class=\"metric-label\">")
			b.WriteString(templateEscape(point.Period))
			b.WriteString("</td><td>")
			b.WriteString(formatFloat(point.In))
			b.WriteString("</td><td>")
			b.WriteString(formatFloat(point.Out))
			b.WriteString("</td></tr>")
		}
		b.WriteString("</tbody></table></section>")
	}

	if len(payload.ARAging)+len(payload.APAging) > 0 {
		b.WriteString("<section><h2>Aging Summary</h2><table><thead><tr><th>Bucket</th><th>AR</th><th>AP</th></tr></thead><tbody>")
		buckets := mergeBuckets(payload.ARAging, payload.APAging)
		for _, bucket := range buckets {
			b.WriteString("<tr><td class=\"metric-label\">")
			b.WriteString(templateEscape(bucket.name))
			b.WriteString("</td><td>")
			b.WriteString(formatFloat(bucket.ar))
			b.WriteString("</td><td>")
			b.WriteString(formatFloat(bucket.ap))
			b.WriteString("</td></tr>")
		}
		b.WriteString("</tbody></table></section>")
	}

	b.WriteString("</body></html>")
	return b.String()
}

func writeMetricRow(b *strings.Builder, label string, value float64) {
	b.WriteString("<tr><td class=\"metric-label\">")
	b.WriteString(templateEscape(label))
	b.WriteString("</td><td>")
	b.WriteString(formatFloat(value))
	b.WriteString("</td></tr>")
}

type mergedBucket struct {
	name string
	ar   float64
	ap   float64
}

func mergeBuckets(ar, ap []analytics.AgingBucket) []mergedBucket {
	buckets := make(map[string]*mergedBucket)
	for _, bucket := range ar {
		entry := buckets[bucket.Bucket]
		if entry == nil {
			entry = &mergedBucket{name: bucket.Bucket}
			buckets[bucket.Bucket] = entry
		}
		entry.ar = bucket.Amount
	}
	for _, bucket := range ap {
		entry := buckets[bucket.Bucket]
		if entry == nil {
			entry = &mergedBucket{name: bucket.Bucket}
			buckets[bucket.Bucket] = entry
		}
		entry.ap = bucket.Amount
	}
	keys := make([]string, 0, len(buckets))
	for key := range buckets {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	merged := make([]mergedBucket, 0, len(keys))
	for _, key := range keys {
		merged = append(merged, *buckets[key])
	}
	return merged
}

func templateEscape(v string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		"\"", "&quot;",
		"'", "&#39;",
	)
	return replacer.Replace(v)
}
