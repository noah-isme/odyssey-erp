package procurement

import (
	"testing"

	accountingmoney "github.com/odyssey-erp/odyssey-erp/internal/accounting/money"
)

func TestCalculateOverallScoreUsesInputsAndWeights(t *testing.T) {
	service := &ScorecardService{}
	overall := service.CalculateOverallScore(
		accountingmoney.Must("85.00", 2), 35,
		accountingmoney.Must("90.00", 2), 25,
		accountingmoney.Must("88.00", 2), 20,
		accountingmoney.Must("80.00", 2), 10,
		accountingmoney.Must("0.00", 2), 10,
	)

	if overall.Amount != "77.85" {
		t.Fatalf("overall score = %s, want 77.85", overall.Amount)
	}
}

func TestScoreFromCountsIsExactAndHandlesEmptyEvidence(t *testing.T) {
	if got := scoreFromCounts(2, 3); got.Amount != "66.67" {
		t.Fatalf("score = %s, want 66.67", got.Amount)
	}
	if got := scoreFromCounts(0, 0); got.Amount != "0.00" {
		t.Fatalf("empty score = %s, want 0.00", got.Amount)
	}
}

func TestPercentageDifferenceUsesExactPriceMath(t *testing.T) {
	got, err := percentageDifference(
		accountingmoney.Must("2.50", 4),
		accountingmoney.Must("10.00", 4),
	)
	if err != nil {
		t.Fatalf("percentageDifference returned error: %v", err)
	}
	if got.Amount != "25.00" {
		t.Fatalf("percentage difference = %s, want 25.00", got.Amount)
	}
}
