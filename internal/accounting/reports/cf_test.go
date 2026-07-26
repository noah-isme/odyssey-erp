package reports

import (
	"math"
	"testing"
)

// closeEnough tolerates float drift when summing many movements.
func closeEnough(a, b float64) bool { return math.Abs(a-b) < 0.005 }

// balancedLedger mirrors a month of trading: a sale on credit, part of it
// collected, an expense accrued and partly paid. Debits equal credits, as any
// posted ledger must.
func balancedLedger() []AccountBalance {
	return []AccountBalance{
		{Code: "1110", Name: "Operating Bank", Type: "ASSET", Opening: 100_000_000, Debit: 45_000_000, Credit: 90_000_000},
		{Code: "1210", Name: "Piutang Usaha", Type: "ASSET", Debit: 279_000_000, Credit: 0},
		{Code: "2110", Name: "Hutang Usaha", Type: "LIABILITY", Debit: 0, Credit: 180_000_000},
		{Code: "4100", Name: "Pendapatan Penjualan", Type: "REVENUE", Debit: 0, Credit: 279_000_000},
		{Code: "5200", Name: "Operational Expense", Type: "EXPENSE", Debit: 225_000_000, Credit: 0},
	}
}

// The statement is only meaningful if it explains the cash movement. The
// previous implementation put every account into Operating, so the sections
// summed to zero by double entry while real cash had moved.
func TestBuildCashFlowReconcilesToCashMovement(t *testing.T) {
	cf := BuildCashFlow(balancedLedger())

	wantMovement := cf.ClosingCash - cf.OpeningCash
	if !closeEnough(cf.NetChange, wantMovement) {
		t.Fatalf("NetChange = %.2f, want %.2f (closing %.2f - opening %.2f)",
			cf.NetChange, wantMovement, cf.ClosingCash, cf.OpeningCash)
	}
	if closeEnough(cf.NetChange, 0) {
		t.Fatal("NetChange is zero although the bank account moved")
	}
	if !closeEnough(cf.NetChange, -45_000_000) {
		t.Errorf("NetChange = %.2f, want -45000000", cf.NetChange)
	}
	if !closeEnough(cf.OpeningCash, 100_000_000) || !closeEnough(cf.ClosingCash, 55_000_000) {
		t.Errorf("cash opening/closing = %.2f/%.2f, want 100000000/55000000", cf.OpeningCash, cf.ClosingCash)
	}
}

// A receivable that grows consumes cash; a payable that grows releases it.
func TestBuildCashFlowSignsWorkingCapital(t *testing.T) {
	cf := BuildCashFlow(balancedLedger())

	amounts := map[string]float64{}
	for _, line := range cf.Operating.Lines {
		amounts[line.AccountCode] = line.Amount
	}
	for _, tc := range []struct {
		code string
		want float64
	}{
		{"1210", -279_000_000}, // receivable rose, cash did not
		{"2110", 180_000_000},  // payable rose, cash retained
		{"4100", 279_000_000},  // revenue earned
		{"5200", -225_000_000}, // expense incurred
	} {
		if got, ok := amounts[tc.code]; !ok {
			t.Errorf("account %s missing from operating section", tc.code)
		} else if !closeEnough(got, tc.want) {
			t.Errorf("account %s = %.2f, want %.2f", tc.code, got, tc.want)
		}
	}
	// Cash accounts are the subject of the statement, never a line within it.
	for _, section := range []CashFlowSection{cf.Operating, cf.Investing, cf.Financing} {
		for _, line := range section.Lines {
			if line.AccountCode == "1110" {
				t.Errorf("cash account 1110 appears as a %s line", section.Label)
			}
		}
	}
}

func TestBuildCashFlowSeparatesInvestingAndFinancing(t *testing.T) {
	cf := BuildCashFlow([]AccountBalance{
		{Code: "1110", Name: "Operating Bank", Type: "ASSET", Debit: 500_000_000, Credit: 120_000_000},
		{Code: "1420", Name: "Kendaraan", Type: "ASSET", Debit: 120_000_000},
		{Code: "3100", Name: "Modal Disetor", Type: "EQUITY", Credit: 500_000_000},
	})

	if !closeEnough(cf.Investing.Total, -120_000_000) {
		t.Errorf("investing = %.2f, want -120000000 (vehicle purchased)", cf.Investing.Total)
	}
	if !closeEnough(cf.Financing.Total, 500_000_000) {
		t.Errorf("financing = %.2f, want 500000000 (capital injected)", cf.Financing.Total)
	}
	if !closeEnough(cf.Operating.Total, 0) {
		t.Errorf("operating = %.2f, want 0", cf.Operating.Total)
	}
	if !closeEnough(cf.NetChange, cf.ClosingCash-cf.OpeningCash) {
		t.Errorf("NetChange = %.2f does not reconcile to cash movement %.2f",
			cf.NetChange, cf.ClosingCash-cf.OpeningCash)
	}
}

func TestBuildCashFlowWithNoMovement(t *testing.T) {
	cf := BuildCashFlow([]AccountBalance{
		{Code: "1110", Name: "Operating Bank", Type: "ASSET", Opening: 10_000_000},
	})
	if !closeEnough(cf.NetChange, 0) {
		t.Errorf("NetChange = %.2f, want 0", cf.NetChange)
	}
	if !closeEnough(cf.OpeningCash, 10_000_000) || !closeEnough(cf.ClosingCash, 10_000_000) {
		t.Errorf("cash = %.2f/%.2f, want 10000000/10000000", cf.OpeningCash, cf.ClosingCash)
	}
}
