package svg

import (
	"fmt"
	"html/template"
	"math"
	"strings"
)

// Bars renders a grouped bar chart comparing two series.
func Bars(width, height int, seriesA, seriesB []float64, labels []string, opts BarOpts) (template.HTML, error) {
	if len(seriesA) == 0 && len(seriesB) == 0 {
		return "", fmt.Errorf("svg: at least one series required")
	}
	if len(labels) == 0 {
		return "", fmt.Errorf("svg: labels required")
	}
	if len(seriesA) > 0 && len(seriesA) != len(labels) {
		return "", fmt.Errorf("svg: seriesA length must match labels")
	}
	if len(seriesB) > 0 && len(seriesB) != len(labels) {
		return "", fmt.Errorf("svg: seriesB length must match labels")
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

	axisColor := fallback(opts.AxisColor, "#475569")
	gridColor := fallback(opts.GridColor, "#cbd5f5")
	colorA := fallback(opts.ColorA, "#0ea5e9")
	colorB := fallback(opts.ColorB, "#f97316")
	labelA := fallback(opts.SeriesALabel, "Series A")
	labelB := fallback(opts.SeriesBLabel, "Series B")

	chartWidth := float64(width) - 2*padding
	chartHeight := float64(height) - 2*padding
	if chartWidth <= 0 || chartHeight <= 0 {
		return "", fmt.Errorf("svg: viewport too small")
	}

	minVal, maxVal := barBounds(seriesA, seriesB)
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
	zeroY := padding + chartHeight - (0-minVal)*scale

	groupWidth := chartWidth / float64(len(labels))
	barWidth := groupWidth / 3

	titleID := makeID(opts.Title, "bar-title")
	descID := makeID(opts.Title, "bar-desc")

	var b strings.Builder
	b.WriteString(fmt.Sprintf("<svg xmlns=\"http://www.w3.org/2000/svg\" viewBox=\"0 0 %d %d\" role=\"img\" aria-labelledby=\"%s %s\">", width, height, titleID, descID))
	b.WriteString(fmt.Sprintf("<title id=\"%s\">%s</title>", titleID, template.HTMLEscapeString(fallback(opts.Title, "Bar chart"))))
	b.WriteString(fmt.Sprintf("<desc id=\"%s\">%s</desc>", descID, template.HTMLEscapeString(fallback(opts.Description, "Grouped bar comparison"))))

	for i := 0; i <= tickCount; i++ {
		ratio := float64(i) / float64(tickCount)
		value := minVal + (maxVal-minVal)*ratio
		y := padding + chartHeight - ratio*chartHeight
		b.WriteString(fmt.Sprintf("<line x1=\"%.2f\" y1=\"%.2f\" x2=\"%.2f\" y2=\"%.2f\" stroke=\"%s\" stroke-width=\"0.5\" stroke-dasharray=\"2,4\" aria-hidden=\"true\"></line>", padding, y, padding+chartWidth, y, gridColor))
		b.WriteString(fmt.Sprintf("<text x=\"%.2f\" y=\"%.2f\" fill=\"%s\" font-size=\"10\" text-anchor=\"end\">%s</text>", padding-6, y+4, axisColor, template.HTMLEscapeString(formatTick(value))))
	}

	// Axes
	b.WriteString(fmt.Sprintf("<g stroke=\"%s\" aria-label=\"Sumbu\">", axisColor))
	b.WriteString(fmt.Sprintf("<line x1=\"%.2f\" y1=\"%.2f\" x2=\"%.2f\" y2=\"%.2f\" stroke-width=\"1\"></line>", padding, padding, padding, padding+chartHeight))
	b.WriteString(fmt.Sprintf("<line x1=\"%.2f\" y1=\"%.2f\" x2=\"%.2f\" y2=\"%.2f\" stroke-width=\"1\"></line>", padding, zeroY, padding+chartWidth, zeroY))
	b.WriteString("</g>")

	chartBottom := padding + chartHeight

	for i, label := range labels {
		baseX := padding + float64(i)*groupWidth
		if len(seriesA) > 0 {
			y, h := barPosition(seriesA[i], scale, zeroY, padding, chartBottom)
			b.WriteString(fmt.Sprintf("<rect x=\"%.2f\" y=\"%.2f\" width=\"%.2f\" height=\"%.2f\" fill=\"%s\" aria-label=\"%s %s\"></rect>", baseX+barWidth*0.3, y, barWidth, h, colorA, template.HTMLEscapeString(labelA), template.HTMLEscapeString(label)))
		}
		if len(seriesB) > 0 {
			y, h := barPosition(seriesB[i], scale, zeroY, padding, chartBottom)
			b.WriteString(fmt.Sprintf("<rect x=\"%.2f\" y=\"%.2f\" width=\"%.2f\" height=\"%.2f\" fill=\"%s\" aria-label=\"%s %s\"></rect>", baseX+barWidth*1.4, y, barWidth, h, colorB, template.HTMLEscapeString(labelB), template.HTMLEscapeString(label)))
		}
		center := baseX + groupWidth/2
		b.WriteString(fmt.Sprintf("<text x=\"%.2f\" y=\"%.2f\" fill=\"%s\" font-size=\"10\" text-anchor=\"middle\">%s</text>", center, padding+chartHeight+14, axisColor, template.HTMLEscapeString(label)))
	}

	// Legend
	legendY := padding - 12
	if legendY < 12 {
		legendY = 12
	}
	legendX := padding
	if len(seriesA) > 0 {
		b.WriteString(fmt.Sprintf("<rect x=\"%.2f\" y=\"%.2f\" width=\"10\" height=\"10\" fill=\"%s\"></rect>", legendX, legendY-8, colorA))
		b.WriteString(fmt.Sprintf("<text x=\"%.2f\" y=\"%.2f\" fill=\"%s\" font-size=\"10\" text-anchor=\"start\">%s</text>", legendX+14, legendY, axisColor, template.HTMLEscapeString(labelA)))
		legendX += 90
	}
	if len(seriesB) > 0 {
		b.WriteString(fmt.Sprintf("<rect x=\"%.2f\" y=\"%.2f\" width=\"10\" height=\"10\" fill=\"%s\"></rect>", legendX, legendY-8, colorB))
		b.WriteString(fmt.Sprintf("<text x=\"%.2f\" y=\"%.2f\" fill=\"%s\" font-size=\"10\" text-anchor=\"start\">%s</text>", legendX+14, legendY, axisColor, template.HTMLEscapeString(labelB)))
	}

	b.WriteString("</svg>")
	return template.HTML(b.String()), nil
}

func barBounds(a, b []float64) (float64, float64) {
	minVal := 0.0
	maxVal := 0.0
	if len(a) > 0 {
		minVal, maxVal = bounds(a)
	}
	if len(b) > 0 {
		minB, maxB := bounds(b)
		if len(a) == 0 || minB < minVal {
			minVal = minB
		}
		if len(a) == 0 || maxB > maxVal {
			maxVal = maxB
		}
	}
	return minVal, maxVal
}

func barPosition(value, scale, zeroY, padding, bottom float64) (float64, float64) {
	if value >= 0 {
		height := value * scale
		y := zeroY - height
		if y < 0 {
			height += y
			y = 0
		}
		if y < padding {
			height -= padding - y
			y = padding
		}
		if height < 0 {
			height = 0
		}
		return y, height
	}
	height := math.Abs(value * scale)
	y := zeroY
	if y+height > bottom {
		height = bottom - y
	}
	if height < 0 {
		height = 0
	}
	return y, height
}
