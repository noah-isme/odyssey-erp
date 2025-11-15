package elimination

import "testing"

func TestComputeElimination(t *testing.T) {
	cases := []struct {
		name   string
		source float64
		target float64
		want   float64
	}{
		{"balanced", 1000, -950, 950},
		{"zero", 0, 0, 0},
		{"negative", -400, -200, 200},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got := ComputeElimination(tt.source, tt.target)
			if got.Eliminated != tt.want {
				t.Fatalf("expected %.2f, got %.2f", tt.want, got.Eliminated)
			}
			if got.SourceBalance != round2(tt.source) || got.TargetBalance != round2(tt.target) {
				t.Fatalf("balances not rounded")
			}
		})
	}
}
