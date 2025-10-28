package reports

import (
	"testing"

	_ "github.com/odyssey-erp/odyssey-erp/testing"
)

func TestBuildTrialBalance(t *testing.T) {
	accounts := []AccountBalance{
		{Code: "1000", Name: "Cash", Type: "ASSET", Opening: 1000, Debit: 200, Credit: 150},
		{Code: "1001", Name: "Bank", Type: "ASSET", Opening: 500, Debit: 100, Credit: 50},
		{Code: "2000", Name: "Accounts Payable", Type: "LIABILITY", Opening: 0, Debit: 10, Credit: 400},
	}

	tb := BuildTrialBalance(accounts)
	if len(tb.Groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(tb.Groups))
	}
	if tb.TotalDebit != 310 {
		t.Fatalf("unexpected total debit: %v", tb.TotalDebit)
	}
	if tb.TotalCredit != 600 {
		t.Fatalf("unexpected total credit: %v", tb.TotalCredit)
	}
	if tb.TotalOpening != 1500 {
		t.Fatalf("unexpected total opening: %v", tb.TotalOpening)
	}
	if tb.TotalClosing != 1210 {
		t.Fatalf("unexpected closing total: %v", tb.TotalClosing)
	}
}

func TestBuildProfitAndLoss(t *testing.T) {
	accounts := []AccountBalance{
		{Code: "4000", Name: "Sales", Type: "REVENUE", Debit: 0, Credit: 1200},
		{Code: "5000", Name: "COGS", Type: "EXPENSE", Debit: 300, Credit: 0},
		{Code: "5100", Name: "Marketing", Type: "EXPENSE", Debit: 200, Credit: 0},
	}

	pl := BuildProfitAndLoss(accounts)
	if pl.Revenue.Total != 1200 {
		t.Fatalf("expected revenue total 1200 got %v", pl.Revenue.Total)
	}
	if pl.Expense.Total != 500 {
		t.Fatalf("expected expense total 500 got %v", pl.Expense.Total)
	}
	if pl.NetIncome != 700 {
		t.Fatalf("expected net income 700 got %v", pl.NetIncome)
	}
}

func TestBuildBalanceSheet(t *testing.T) {
	accounts := []AccountBalance{
		{Code: "1000", Name: "Cash", Type: "ASSET", Opening: 0, Debit: 100, Credit: 20},
		{Code: "2000", Name: "AP", Type: "LIABILITY", Opening: 0, Debit: 10, Credit: 40},
		{Code: "3000", Name: "Equity", Type: "EQUITY", Opening: 500, Debit: 0, Credit: 0},
	}

	bs := BuildBalanceSheet(accounts)
	if bs.Assets.Total != 80 {
		t.Fatalf("expected assets 80 got %v", bs.Assets.Total)
	}
	if bs.Liabilities.Total != -30 {
		t.Fatalf("expected liabilities -30 got %v", bs.Liabilities.Total)
	}
	if bs.Equity.Total != 500 {
		t.Fatalf("expected equity 500 got %v", bs.Equity.Total)
	}
	if bs.TotalLiabilitiesAndEquity != 470 {
		t.Fatalf("expected total L+E 470 got %v", bs.TotalLiabilitiesAndEquity)
	}
}
