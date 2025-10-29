package fx

import "testing"

func TestConvertProfitLossAverage(t *testing.T) {
	policy := Policy{ReportingCurrency: "USD", ProfitLossMethod: MethodAverage}
	quotes := map[string]Quote{
		"IDRUSD": {Average: 0.00007, Closing: 0.00006},
	}
	converter := NewConverter(policy, quotes)
	lines, delta, err := converter.ConvertProfitLoss([]Line{{
		AccountCode:   "4000",
		LocalCurrency: "idr",
		LocalAmount:   1000000,
		GroupAmount:   65,
	}})
	if err != nil {
		t.Fatalf("ConvertProfitLoss returned error: %v", err)
	}
	if len(lines) != 1 {
		t.Fatalf("expected one line got %d", len(lines))
	}
	want := 70.0
	if lines[0].GroupAmount != want {
		t.Fatalf("expected converted amount %.2f got %.2f", want, lines[0].GroupAmount)
	}
	if delta != want-65 {
		t.Fatalf("unexpected delta %.2f", delta)
	}
}

func TestConvertBalanceSheetClosing(t *testing.T) {
	policy := Policy{ReportingCurrency: "USD", BalanceSheetMethod: MethodClosing}
	quotes := map[string]Quote{
		"JPYUSD": {Average: 0.009, Closing: 0.0095},
	}
	converter := NewConverter(policy, quotes)
	lines, delta, err := converter.ConvertBalanceSheet([]Line{{
		AccountCode:   "1000",
		LocalCurrency: "JPY",
		LocalAmount:   10000,
		GroupAmount:   80,
	}})
	if err != nil {
		t.Fatalf("ConvertBalanceSheet returned error: %v", err)
	}
	if lines[0].GroupAmount != 95 {
		t.Fatalf("expected converted amount 95 got %.2f", lines[0].GroupAmount)
	}
	if delta != 15 {
		t.Fatalf("expected delta 15 got %.2f", delta)
	}
}

func TestConvertMissingRate(t *testing.T) {
	policy := Policy{ReportingCurrency: "USD"}
	converter := NewConverter(policy, map[string]Quote{})
	_, _, err := converter.ConvertProfitLoss([]Line{{
		AccountCode:   "4000",
		LocalCurrency: "EUR",
		LocalAmount:   100,
		GroupAmount:   110,
	}})
	if err == nil {
		t.Fatalf("expected error for missing rate")
	}
	if _, ok := err.(*MissingRateError); !ok {
		t.Fatalf("expected MissingRateError got %T", err)
	}
}

func TestConvertDefaultsToParity(t *testing.T) {
	policy := Policy{ReportingCurrency: "USD"}
	converter := NewConverter(policy, map[string]Quote{})
	lines, delta, err := converter.ConvertProfitLoss([]Line{{
		AccountCode:   "4000",
		LocalCurrency: "USD",
		LocalAmount:   50,
		GroupAmount:   40,
	}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if lines[0].GroupAmount != 50 {
		t.Fatalf("expected amount 50 got %.2f", lines[0].GroupAmount)
	}
	if delta != 10 {
		t.Fatalf("expected delta 10 got %.2f", delta)
	}
}
