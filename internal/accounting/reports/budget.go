package reports

import (
	"math"
)

// BudgetVsActualLine represents a single account's budget comparison.
type BudgetVsActualLine struct {
	AccountID   int64
	AccountCode string
	AccountName string
	Budget      float64
	Actual      float64
	Variance    float64
	VariancePct float64
}

// BudgetVsActual groups comparison lines by account type.
type BudgetVsActual struct {
	Revenue []BudgetVsActualLine
	Expense []BudgetVsActualLine
}

// BudgetData maps account ID to budgeted amount.
type BudgetData map[int64]float64

// BuildBudgetVsActual creates a budget comparison report.
func BuildBudgetVsActual(accounts []AccountBalance, budgets BudgetData) BudgetVsActual {
	var report BudgetVsActual

	for _, acc := range accounts {
		budgetAmt := budgets[acc.ID]
		actualAmt := 0.0

		// Simple actual calculation (Debit/Credit logic depending on type)
		switch acc.Type {
		case "REVENUE", "INCOME":
			actualAmt = acc.Credit - acc.Debit
			if budgetAmt == 0 && actualAmt == 0 {
				continue
			}
			line := buildLine(acc, budgetAmt, actualAmt)
			report.Revenue = append(report.Revenue, line)
		case "EXPENSE", "COGS":
			actualAmt = acc.Debit - acc.Credit
			if budgetAmt == 0 && actualAmt == 0 {
				continue
			}
			line := buildLine(acc, budgetAmt, actualAmt)
			report.Expense = append(report.Expense, line)
		}
	}
	return report
}

func buildLine(acc AccountBalance, budget, actual float64) BudgetVsActualLine {
	variance := actual - budget
	var pct float64
	if budget != 0 {
		pct = (variance / math.Abs(budget)) * 100
	}
	return BudgetVsActualLine{
		AccountID:   acc.ID,
		AccountCode: acc.Code,
		AccountName: acc.Name,
		Budget:      budget,
		Actual:      actual,
		Variance:    variance,
		VariancePct: pct,
	}
}
