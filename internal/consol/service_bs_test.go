package consol

import (
	"context"
	"encoding/json"
	"testing"
)

type fakeBSRepo struct {
	rows []ConsolBalanceByTypeQueryRow
	err  error
}

func (f *fakeBSRepo) ConsolBalancesByType(ctx context.Context, groupID int64, periodCode string, entities []int64) ([]ConsolBalanceByTypeQueryRow, error) {
	if f.err != nil {
		return nil, f.err
	}
	return append([]ConsolBalanceByTypeQueryRow(nil), f.rows...), nil
}

func TestBalanceSheetServiceBuildAggregates(t *testing.T) {
	members := func(values ...map[string]interface{}) []byte {
		data, _ := json.Marshal(values)
		return data
	}
	repo := &fakeBSRepo{rows: []ConsolBalanceByTypeQueryRow{
		{
			GroupAccountID:   10,
			GroupAccountCode: "1000",
			GroupAccountName: "Assets",
			AccountType:      "ASSET",
			LocalAmount:      150,
			GroupAmount:      150,
			MembersJSON: members(
				map[string]interface{}{"company_id": 1, "company_name": "Alpha", "local_ccy_amt": 100},
				map[string]interface{}{"company_id": 2, "company_name": "Beta", "local_ccy_amt": 50},
			),
		},
		{
			GroupAccountID:   20,
			GroupAccountCode: "2000",
			GroupAccountName: "Liabilities",
			AccountType:      "LIABILITY",
			LocalAmount:      90,
			GroupAmount:      90,
			MembersJSON: members(
				map[string]interface{}{"company_id": 1, "company_name": "Alpha", "local_ccy_amt": 60},
				map[string]interface{}{"company_id": 2, "company_name": "Beta", "local_ccy_amt": 30},
			),
		},
		{
			GroupAccountID:   30,
			GroupAccountCode: "3000",
			GroupAccountName: "Equity",
			AccountType:      "EQUITY",
			LocalAmount:      60,
			GroupAmount:      60,
			MembersJSON: members(
				map[string]interface{}{"company_id": 1, "company_name": "Alpha", "local_ccy_amt": 40},
				map[string]interface{}{"company_id": 2, "company_name": "Beta", "local_ccy_amt": 20},
			),
		},
	}}

	svc := NewBalanceSheetService(repo)
	report, err := svc.Build(context.Background(), BalanceSheetFilters{GroupID: 10, Period: "2024-01"})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if len(report.Assets) != 1 {
		t.Fatalf("expected one asset line got %d", len(report.Assets))
	}
	if len(report.LiabilitiesEq) != 2 {
		t.Fatalf("expected two liability/equity lines got %d", len(report.LiabilitiesEq))
	}
	if !report.Totals.Balanced {
		t.Fatalf("expected balanced totals")
	}
	if report.Totals.Assets != 150 {
		t.Fatalf("expected assets 150 got %v", report.Totals.Assets)
	}
	if report.Totals.LiabEquity != 150 {
		t.Fatalf("expected liabilities+equity 150 got %v", report.Totals.LiabEquity)
	}
	if len(report.Contributions) != 2 {
		t.Fatalf("expected two contributions got %d", len(report.Contributions))
	}
}

func TestBalanceSheetServiceFiltersEntities(t *testing.T) {
	members := func(values ...map[string]interface{}) []byte {
		data, _ := json.Marshal(values)
		return data
	}
	repo := &fakeBSRepo{rows: []ConsolBalanceByTypeQueryRow{
		{
			GroupAccountID:   10,
			GroupAccountCode: "1000",
			GroupAccountName: "Assets",
			AccountType:      "ASSET",
			LocalAmount:      150,
			GroupAmount:      150,
			MembersJSON: members(
				map[string]interface{}{"company_id": 1, "company_name": "Alpha", "local_ccy_amt": 100},
				map[string]interface{}{"company_id": 2, "company_name": "Beta", "local_ccy_amt": 50},
			),
		},
	}}
	svc := NewBalanceSheetService(repo)
	report, err := svc.Build(context.Background(), BalanceSheetFilters{GroupID: 10, Period: "2024-01", Entities: []int64{2}})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if len(report.Assets) != 1 {
		t.Fatalf("expected one asset line got %d", len(report.Assets))
	}
	if report.Assets[0].GroupAmount != 50 {
		t.Fatalf("expected filtered amount 50 got %v", report.Assets[0].GroupAmount)
	}
	if len(report.Contributions) != 1 {
		t.Fatalf("expected one contribution got %d", len(report.Contributions))
	}
	if report.Contributions[0].EntityName != "Beta" {
		t.Fatalf("expected contribution for Beta got %s", report.Contributions[0].EntityName)
	}
}
