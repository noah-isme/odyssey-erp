package svg

import (
	"strings"
	"testing"
)

func TestLineProducesSVG(t *testing.T) {
	html, err := Line(400, 200, []float64{100, 200, 150}, []string{"2025-01", "2025-02", "2025-03"}, LineOpts{
		Title:       "Net Profit",
		Description: "Monthly net profit",
		ShowDots:    true,
	})
	if err != nil {
		t.Fatalf("line renderer error: %v", err)
	}
	output := string(html)
	if !strings.HasPrefix(output, "<svg") {
		t.Fatalf("expected svg output, got %s", output)
	}
	if !strings.Contains(output, "<path") {
		t.Fatalf("expected path element in svg")
	}
	if !strings.Contains(output, "aria-labelledby") {
		t.Fatalf("expected accessibility attributes")
	}
}
