package variance

import "testing"

func TestComputeVariance(t *testing.T) {
	base := map[string]AccountBalance{
		"1000": {Name: "Cash", Amount: 1500},
	}
	compare := map[string]AccountBalance{
		"1000": {Name: "Cash", Amount: 1000},
		"2000": {Name: "AP", Amount: -800},
	}
	threshold := 200.0
	rows := ComputeVariance(base, compare, &threshold, nil)
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if !rows[0].Flagged {
		t.Fatalf("expected flagged variance")
	}
}
