package reports

import (
	"bytes"
	"testing"
)

func TestWriteProfitAndLossXLSX(t *testing.T) {
	var output bytes.Buffer
	report := BuildProfitAndLoss([]AccountBalance{{Code: "4000", Name: "Revenue", Type: "REVENUE", Credit: 100}})
	if err := WriteProfitAndLossXLSX(&output, report, "2026-07"); err != nil {
		t.Fatal(err)
	}
	if output.Len() < 4 || string(output.Bytes()[:2]) != "PK" {
		t.Fatalf("expected XLSX ZIP payload, got %q", output.Bytes()[:2])
	}
}

func TestWriteBudgetVsActualXLSX(t *testing.T) {
	var output bytes.Buffer
	report := BuildBudgetVsActual([]AccountBalance{{ID: 1, Code: "5000", Name: "Expense", Type: "EXPENSE", Debit: 100}}, BudgetData{1: 120})
	if err := WriteBudgetVsActualXLSX(&output, report, "2026-07"); err != nil {
		t.Fatal(err)
	}
	if output.Len() < 4 || string(output.Bytes()[:2]) != "PK" {
		t.Fatalf("expected XLSX ZIP payload, got %q", output.Bytes()[:2])
	}
}
