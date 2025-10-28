package svg

import (
	"fmt"
	"html/template"
	"math"
	"strings"
)

// Line renders a responsive SVG line chart for the given series and labels.
func Line(width, height int, series []float64, labels []string, opts LineOpts) (template.HTML, error) {
	if len(series) == 0 {
		return "", fmt.Errorf("svg: series required")
	}
	if len(series) != len(labels) {
		return "", fmt.Errorf("svg: labels length must match series")
	}
	if width <= 0 {
		width = DefaultWidth
	}
	if height <= 0 {
		height = DefaultHeight
	}
	padding := opts.Padding
	if padding <= 0 {
		padding = DefaultPadding
	}
	tickCount := opts.TickCount
	if tickCount <= 0 {
		tickCount = DefaultTicks
	}
	strokeColor := fallback(opts.StrokeColor, "#2563eb")
	fillColor := fallback(opts.FillColor, "rgba(37,99,235,0.12)")
	axisColor := fallback(opts.AxisColor, "#475569")
	gridColor := fallback(opts.GridColor, "#cbd5f5")

	chartWidth := float64(width) - 2*padding
	chartHeight := float64(height) - 2*padding
	if chartWidth <= 0 || chartHeight <= 0 {
		return "", fmt.Errorf("svg: viewport too small")
	}

	minVal, maxVal := bounds(series)
	if minVal > 0 {
		minVal = 0
	}
	if maxVal < 0 {
		maxVal = 0
	}
	if almostEqual(maxVal, minVal) {
		maxVal = minVal + 1
	}
	scale := chartHeight / (maxVal - minVal)

	step := 0.0
	if len(series) > 1 {
		step = chartWidth / float64(len(series)-1)
	}

	var path strings.Builder
	firstX := 0.0
	lastX := 0.0
	for i, value := range series {
		x := padding
		if len(series) > 1 {
			x += float64(i) * step
		} else {
			x += chartWidth / 2
		}
		normalized := (value - minVal) * scale
		y := padding + chartHeight - normalized
		if i == 0 {
			firstX = x
			path.WriteString(fmt.Sprintf("M%.2f %.2f", x, y))
		} else {
			path.WriteString(fmt.Sprintf(" L%.2f %.2f", x, y))
		}
		lastX = x
	}

	titleID := makeID(opts.Title, "line-title")
	descID := makeID(opts.Title, "line-desc")

	var b strings.Builder
	b.WriteString(fmt.Sprintf("<svg xmlns=\"http://www.w3.org/2000/svg\" viewBox=\"0 0 %d %d\" role=\"img\" aria-labelledby=\"%s %s\">", width, height, titleID, descID))
	b.WriteString(fmt.Sprintf("<title id=\"%s\">%s</title>", titleID, template.HTMLEscapeString(fallback(opts.Title, "Line chart"))))
	b.WriteString(fmt.Sprintf("<desc id=\"%s\">%s</desc>", descID, template.HTMLEscapeString(fallback(opts.Description, "Trend data"))))

	// Grid lines and ticks
	for i := 0; i <= tickCount; i++ {
		ratio := float64(i) / float64(tickCount)
		y := padding + chartHeight - ratio*chartHeight
		value := minVal + (maxVal-minVal)*ratio
		b.WriteString(fmt.Sprintf("<line x1=\"%.2f\" y1=\"%.2f\" x2=\"%.2f\" y2=\"%.2f\" stroke=\"%s\" stroke-width=\"0.5\" stroke-dasharray=\"2,4\" aria-hidden=\"true\"></line>", padding, y, padding+chartWidth, y, gridColor))
		b.WriteString(fmt.Sprintf("<text x=\"%.2f\" y=\"%.2f\" fill=\"%s\" font-size=\"10\" text-anchor=\"end\">%s</text>", padding-6, y+4, axisColor, template.HTMLEscapeString(formatTick(value))))
	}

	// Axes
	b.WriteString(fmt.Sprintf("<g stroke=\"%s\" aria-label=\"Sumbu\">", axisColor))
	b.WriteString(fmt.Sprintf("<line x1=\"%.2f\" y1=\"%.2f\" x2=\"%.2f\" y2=\"%.2f\" stroke-width=\"1\"></line>", padding, padding, padding, padding+chartHeight))
	b.WriteString(fmt.Sprintf("<line x1=\"%.2f\" y1=\"%.2f\" x2=\"%.2f\" y2=\"%.2f\" stroke-width=\"1\"></line>", padding, padding+chartHeight, padding+chartWidth, padding+chartHeight))
	b.WriteString("</g>")

	// Area under line
	if fillColor != "" {
		base := padding + chartHeight
		area := fmt.Sprintf("%s L%.2f %.2f L%.2f %.2f Z", path.String(), lastX, base, firstX, base)
		b.WriteString(fmt.Sprintf("<path d=\"%s\" fill=\"%s\" stroke=\"none\" aria-hidden=\"true\"></path>", area, fillColor))
	}

	b.WriteString(fmt.Sprintf("<path d=\"%s\" fill=\"none\" stroke=\"%s\" stroke-width=\"2\" stroke-linejoin=\"round\" stroke-linecap=\"round\"></path>", path.String(), strokeColor))

	if opts.ShowDots {
		for i, value := range series {
			x := padding
			if len(series) > 1 {
				x += float64(i) * step
			} else {
				x += chartWidth / 2
			}
			normalized := (value - minVal) * scale
			y := padding + chartHeight - normalized
			b.WriteString(fmt.Sprintf("<circle cx=\"%.2f\" cy=\"%.2f\" r=\"3\" fill=\"%s\"></circle>", x, y, strokeColor))
		}
	}

	// X-axis labels
	for i, label := range labels {
		x := padding
		if len(labels) > 1 {
			x += float64(i) * step
		} else {
			x += chartWidth / 2
		}
		b.WriteString(fmt.Sprintf("<text x=\"%.2f\" y=\"%.2f\" fill=\"%s\" font-size=\"10\" text-anchor=\"middle\">%s</text>", x, padding+chartHeight+14, axisColor, template.HTMLEscapeString(label)))
	}

	b.WriteString("</svg>")
	return template.HTML(b.String()), nil
}

func fallback(value, defaultValue string) string {
	if strings.TrimSpace(value) == "" {
		return defaultValue
	}
	return value
}

func bounds(series []float64) (float64, float64) {
	minVal := series[0]
	maxVal := series[0]
	for _, v := range series[1:] {
		if v < minVal {
			minVal = v
		}
		if v > maxVal {
			maxVal = v
		}
	}
	return minVal, maxVal
}

func almostEqual(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}

func makeID(base, suffix string) string {
	cleaned := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			return r
		}
		if r == '-' || r == '_' {
			return r
		}
		return '-'
	}, strings.ToLower(strings.TrimSpace(base)))
	cleaned = strings.Trim(cleaned, "-")
	if cleaned == "" {
		cleaned = "chart"
	}
	return fmt.Sprintf("%s-%s", cleaned, suffix)
}

func formatTick(v float64) string {
	abs := math.Abs(v)
	switch {
	case abs >= 1_000_000_000:
		return fmt.Sprintf("%.1fB", v/1_000_000_000)
	case abs >= 1_000_000:
		return fmt.Sprintf("%.1fM", v/1_000_000)
	case abs >= 1_000:
		return fmt.Sprintf("%.1fk", v/1_000)
	default:
		if almostEqual(v, math.Round(v)) {
			return fmt.Sprintf("%.0f", v)
		}
		return fmt.Sprintf("%.2f", v)
	}
}
