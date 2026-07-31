package fx

import "testing"

func TestDecimalAndPaymentValuation(t *testing.T) {
	allocated := MustDecimal("100.00")
	v, err := CalculatePaymentValuation(allocated, MustDecimal("15000"), MustDecimal("15500"))
	if err != nil {
		t.Fatal(err)
	}
	if got := v.CarryingBaseAmount.String(); got != "1500000.0000000000" {
		t.Fatalf("carrying = %s", got)
	}
	if got := v.SettlementBaseAmount.String(); got != "1550000.0000000000" {
		t.Fatalf("settlement = %s", got)
	}
	if got := v.RealizedDifference.String(); got != "50000.0000000000" {
		t.Fatalf("difference = %s", got)
	}
}

func TestCalculateBaseAmountRoundsAtAccountingBoundary(t *testing.T) {
	got, err := CalculateBaseAmount(MustDecimal("10.00"), MustDecimal("1.23456"))
	if err != nil {
		t.Fatal(err)
	}
	if got.String() != "12.3500000000" {
		t.Fatalf("got %s", got.String())
	}
}

func TestCurrencyNormalizesAndRejectsInvalidCodes(t *testing.T) {
	if got, _ := Currency(" usd "); got != "USD" {
		t.Fatal(got)
	}
	if _, err := Currency("US"); err == nil {
		t.Fatal("expected invalid currency")
	}
}

func TestRevaluationPreservesCarryingAmountAndSupportsLoss(t *testing.T) {
	result, err := CalculateRevaluation(RevaluationInput{DocumentType: ARInvoice, OriginalBalance: MustDecimal("100"), PreviousBaseAmount: MustDecimal("1500000"), ClosingRate: MustDecimal("14500")})
	if err != nil {
		t.Fatal(err)
	}
	if result.ClosingBaseAmount.String() != "1450000.0000000000" || result.Difference.String() != "-50000.0000000000" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestUSDPartialPaymentRealizedFXUsesOnlyAllocatedAmount(t *testing.T) {
	result, err := CalculatePaymentValuation(MustDecimal("40"), MustDecimal("15000"), MustDecimal("15500"))
	if err != nil {
		t.Fatal(err)
	}
	if got := result.CarryingBaseAmount.String(); got != "600000.0000000000" {
		t.Fatalf("carrying amount = %s", got)
	}
	if got := result.SettlementBaseAmount.String(); got != "620000.0000000000" {
		t.Fatalf("settlement amount = %s", got)
	}
	if got := result.RealizedDifference.String(); got != "20000.0000000000" {
		t.Fatalf("realized difference = %s", got)
	}
}
