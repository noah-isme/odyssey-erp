package shared

import "testing"

func TestCalculateLineTotals(t *testing.T) {
	discount, tax, total := CalculateLineTotals(4, 25, 10, 11)
	if discount != 10 {
		t.Fatalf("discount = %v, want 10", discount)
	}
	if tax != 9.9 {
		t.Fatalf("tax = %v, want 9.9", tax)
	}
	if total != 99.9 {
		t.Fatalf("total = %v, want 99.9", total)
	}
}

func TestCalculateLineTotalsHandlesZeroDiscountAndTax(t *testing.T) {
	discount, tax, total := CalculateLineTotals(2, 12.5, 0, 0)
	if discount != 0 || tax != 0 || total != 25 {
		t.Fatalf("totals = %v, %v, %v", discount, tax, total)
	}
}
