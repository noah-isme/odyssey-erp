package money

import "testing"

func TestMoneyArithmeticDoesNotRoundThroughFloat(t *testing.T) {
	a := Must("100.10", 2)
	b := Must("0.20", 2)
	if got := a.Add(b).String(); got != "100.30" {
		t.Fatalf("got %s, want 100.30", got)
	}
}
