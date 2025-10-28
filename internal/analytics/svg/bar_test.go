package svg

import (
	"strings"
	"testing"
)

func TestBarsProducesSVG(t *testing.T) {
	html, err := Bars(420, 220, []float64{500, 600}, []float64{300, 320}, []string{"2025-01", "2025-02"}, BarOpts{
		Title:        "Cashflow",
		Description:  "Monthly cashflow",
		SeriesALabel: "Cash In",
		SeriesBLabel: "Cash Out",
	})
	if err != nil {
		t.Fatalf("bars renderer error: %v", err)
	}
	output := string(html)
	if !strings.HasPrefix(output, "<svg") {
		t.Fatalf("expected svg output, got %s", output)
	}
	if !strings.Contains(output, "<rect") {
		t.Fatalf("expected rect bars in svg")
	}
	if !strings.Contains(output, "Cash In") {
		t.Fatalf("expected legend label")
	}
}
