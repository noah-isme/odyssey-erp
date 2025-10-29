package consol

import (
	"context"
	"encoding/json"
	"testing"
)

type fakePLRepo struct {
	rows []ConsolBalanceByTypeQueryRow
	err  error
}

func (f *fakePLRepo) ConsolBalancesByType(ctx context.Context, groupID int64, periodCode string, entities []int64) ([]ConsolBalanceByTypeQueryRow, error) {
	if f.err != nil {
		return nil, f.err
	}
	return append([]ConsolBalanceByTypeQueryRow(nil), f.rows...), nil
}

func marshalMembers(members ...map[string]interface{}) []byte {
	data, _ := json.Marshal(members)
	return data
}

func TestProfitLossServiceBuildAggregatesSections(t *testing.T) {
	repo := &fakePLRepo{rows: []ConsolBalanceByTypeQueryRow{
		{
			GroupAccountID:   1,
			GroupAccountCode: "4000",
			GroupAccountName: "Revenue",
			AccountType:      "REVENUE",
			LocalAmount:      -1500,
			GroupAmount:      -1500,
			MembersJSON: marshalMembers(
				map[string]interface{}{"company_id": 1, "company_name": "Alpha", "local_ccy_amt": -1000},
				map[string]interface{}{"company_id": 2, "company_name": "Beta", "local_ccy_amt": -500},
			),
		},
		{
			GroupAccountID:   2,
			GroupAccountCode: "5000",
			GroupAccountName: "COGS",
			AccountType:      "EXPENSE",
			LocalAmount:      1000,
			GroupAmount:      1000,
			MembersJSON: marshalMembers(
				map[string]interface{}{"company_id": 1, "company_name": "Alpha", "local_ccy_amt": 600},
				map[string]interface{}{"company_id": 2, "company_name": "Beta", "local_ccy_amt": 400},
			),
		},
		{
			GroupAccountID:   3,
			GroupAccountCode: "6100",
			GroupAccountName: "Operating Expenses",
			AccountType:      "EXPENSE",
			LocalAmount:      350,
			GroupAmount:      350,
			MembersJSON: marshalMembers(
				map[string]interface{}{"company_id": 1, "company_name": "Alpha", "local_ccy_amt": 200},
				map[string]interface{}{"company_id": 2, "company_name": "Beta", "local_ccy_amt": 150},
			),
		},
	}}

	svc := NewProfitLossService(repo)
	report, err := svc.Build(context.Background(), ProfitLossFilters{GroupID: 10, Period: "2024-01"})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if got, want := len(report.Lines), 3; got != want {
		t.Fatalf("expected %d lines, got %d", want, got)
	}
	totals := report.Totals
	if totals.Revenue != 1500 {
		t.Fatalf("expected revenue 1500 got %v", totals.Revenue)
	}
	if totals.COGS != 1000 {
		t.Fatalf("expected cogs 1000 got %v", totals.COGS)
	}
	if totals.Opex != 350 {
		t.Fatalf("expected opex 350 got %v", totals.Opex)
	}
	if totals.GrossProfit != 500 {
		t.Fatalf("expected gross profit 500 got %v", totals.GrossProfit)
	}
	if totals.NetIncome != 150 {
		t.Fatalf("expected net income 150 got %v", totals.NetIncome)
	}
	if len(report.Contributions) != 2 {
		t.Fatalf("expected 2 contributions got %d", len(report.Contributions))
	}
	if pct := report.Contributions[0].Percent + report.Contributions[1].Percent; pct < 99.9 || pct > 100.1 {
		t.Fatalf("expected contribution percent to sum to ~100 got %v", pct)
	}
}

func TestProfitLossServiceBuildFiltersEntities(t *testing.T) {
	repo := &fakePLRepo{rows: []ConsolBalanceByTypeQueryRow{
		{
			GroupAccountID:   1,
			GroupAccountCode: "4000",
			GroupAccountName: "Revenue",
			AccountType:      "REVENUE",
			LocalAmount:      -1500,
			GroupAmount:      -1500,
			MembersJSON: marshalMembers(
				map[string]interface{}{"company_id": 1, "company_name": "Alpha", "local_ccy_amt": -1000},
				map[string]interface{}{"company_id": 2, "company_name": "Beta", "local_ccy_amt": -500},
			),
		},
	}}
	svc := NewProfitLossService(repo)
	report, err := svc.Build(context.Background(), ProfitLossFilters{GroupID: 10, Period: "2024-01", Entities: []int64{2}})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if len(report.Lines) != 1 {
		t.Fatalf("expected one line got %d", len(report.Lines))
	}
	if report.Lines[0].GroupAmount != 500 {
		t.Fatalf("expected filtered group amount 500 got %v", report.Lines[0].GroupAmount)
	}
	if len(report.Contributions) != 1 {
		t.Fatalf("expected one contribution got %d", len(report.Contributions))
	}
	if report.Contributions[0].EntityName != "Beta" {
		t.Fatalf("expected contribution for Beta got %s", report.Contributions[0].EntityName)
	}
}
