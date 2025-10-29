package insightssvg

import (
	"fmt"
	"html/template"
	"math"
	"strings"
)

const (
	defaultWidth   = 720
	defaultHeight  = 240
	defaultPadding = 24.0
	defaultTicks   = 6
	axisColor      = "#475569"
	gridColor      = "#cbd5f5"
	seriesAColor   = "#2563eb"
	seriesBColor   = "#f97316"
)

// LineMulti merender grafik garis multi-seri sederhana untuk Net & Revenue.
func LineMulti(width, height int, seriesA, seriesB []float64, labels []string) (template.HTML, error) {
	if len(seriesA) == 0 || len(seriesB) == 0 {
		return "", fmt.Errorf("svg: data series required")
	}
	if len(seriesA) != len(seriesB) || len(seriesA) != len(labels) {
		return "", fmt.Errorf("svg: series length mismatch")
	}
	if width <= 0 {
		width = defaultWidth
	}
	if height <= 0 {
		height = defaultHeight
	}

	padding := defaultPadding
	chartWidth := float64(width) - padding*2
	chartHeight := float64(height) - padding*2
	if chartWidth <= 0 || chartHeight <= 0 {
		return "", fmt.Errorf("svg: viewport too small")
	}

	minVal, maxVal := bounds(seriesA, seriesB)
	if almostEqual(minVal, maxVal) {
		if almostEqual(minVal, 0) {
			maxVal = 1
		} else {
			maxVal = minVal + math.Abs(minVal)*0.1
		}
	}
	if minVal > 0 {
		minVal = 0
	}
	if maxVal < 0 {
		maxVal = 0
	}
	scale := chartHeight / (maxVal - minVal)
	step := 0.0
	if len(seriesA) > 1 {
		step = chartWidth / float64(len(seriesA)-1)
	}

	pathA := buildPath(seriesA, padding, chartWidth, chartHeight, minVal, scale, step)
	pathB := buildPath(seriesB, padding, chartWidth, chartHeight, minVal, scale, step)

	idBase := makeID("finance-insights")
	titleID := idBase + "-title"
	descID := idBase + "-desc"

	var b strings.Builder
	b.WriteString(fmt.Sprintf("<svg xmlns=\"http://www.w3.org/2000/svg\" viewBox=\"0 0 %d %d\" role=\"img\" aria-labelledby=\"%s %s\">", width, height, titleID, descID))
	b.WriteString(fmt.Sprintf("<title id=\"%s\">Net dan Revenue 12 bulan</title>", titleID))
	b.WriteString(fmt.Sprintf("<desc id=\"%s\">Perbandingan performa bulanan</desc>", descID))

	for i := 0; i <= defaultTicks; i++ {
		ratio := float64(i) / float64(defaultTicks)
		y := padding + chartHeight - ratio*chartHeight
		value := minVal + (maxVal-minVal)*ratio
		b.WriteString(fmt.Sprintf("<line x1=\"%.2f\" y1=\"%.2f\" x2=\"%.2f\" y2=\"%.2f\" stroke=\"%s\" stroke-width=\"0.5\" stroke-dasharray=\"2,4\" aria-hidden=\"true\"></line>", padding, y, padding+chartWidth, y, gridColor))
		b.WriteString(fmt.Sprintf("<text x=\"%.2f\" y=\"%.2f\" fill=\"%s\" font-size=\"10\" text-anchor=\"end\">%s</text>", padding-6, y+4, axisColor, formatTick(value)))
	}

	// axes
	b.WriteString(fmt.Sprintf("<line x1=\"%.2f\" y1=\"%.2f\" x2=\"%.2f\" y2=\"%.2f\" stroke=\"%s\" stroke-width=\"1\"></line>", padding, padding, padding, padding+chartHeight, axisColor))
	b.WriteString(fmt.Sprintf("<line x1=\"%.2f\" y1=\"%.2f\" x2=\"%.2f\" y2=\"%.2f\" stroke=\"%s\" stroke-width=\"1\"></line>", padding, padding+chartHeight, padding+chartWidth, padding+chartHeight, axisColor))

	b.WriteString(fmt.Sprintf("<path d=\"%s\" fill=\"none\" stroke=\"%s\" stroke-width=\"2\" stroke-linejoin=\"round\" stroke-linecap=\"round\"></path>", pathA, seriesAColor))
	b.WriteString(fmt.Sprintf("<path d=\"%s\" fill=\"none\" stroke=\"%s\" stroke-width=\"2\" stroke-linejoin=\"round\" stroke-linecap=\"round\"></path>", pathB, seriesBColor))

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

func buildPath(series []float64, padding, chartWidth, chartHeight, minVal, scale, step float64) string {
	var path strings.Builder
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
			path.WriteString(fmt.Sprintf("M%.2f %.2f", x, y))
		} else {
			path.WriteString(fmt.Sprintf(" L%.2f %.2f", x, y))
		}
	}
	return path.String()
}

func bounds(a, b []float64) (float64, float64) {
	minVal := a[0]
	maxVal := a[0]
	for _, v := range a {
		if v < minVal {
			minVal = v
		}
		if v > maxVal {
			maxVal = v
		}
	}
	for _, v := range b {
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

func makeID(base string) string {
	cleaned := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r
		case r >= 'A' && r <= 'Z':
			return r
		case r >= '0' && r <= '9':
			return r
		case r == '-' || r == '_':
			return r
		default:
			return '-'
		}
	}, strings.ToLower(strings.TrimSpace(base)))
	cleaned = strings.Trim(cleaned, "-")
	if cleaned == "" {
		cleaned = "chart"
	}
	return cleaned
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
