package reports

import (
	"sort"
	"strings"
)

// ProfitAndLossAccount represents a revenue or expense account summary.
type ProfitAndLossAccount struct {
	Code   string
	Name   string
	Amount float64
}

// ProfitAndLossSection groups accounts by nature.
type ProfitAndLossSection struct {
	Label    string
	Accounts []ProfitAndLossAccount
	Total    float64
}

// ProfitAndLoss contains the structured output for the report.
type ProfitAndLoss struct {
	Revenue   ProfitAndLossSection
	Expense   ProfitAndLossSection
	NetIncome float64
}

// BuildProfitAndLoss aggregates accounts into revenue and expense sections.
func BuildProfitAndLoss(accounts []AccountBalance) ProfitAndLoss {
	revenue := ProfitAndLossSection{Label: "Revenue"}
	expense := ProfitAndLossSection{Label: "Expense"}

	for _, acc := range accounts {
		amount := acc.Debit - acc.Credit
		row := ProfitAndLossAccount{Code: acc.Code, Name: acc.Name, Amount: amount}
		switch strings.ToUpper(acc.Type) {
		case "REVENUE", "INCOME":
			row.Amount = -amount
			revenue.Accounts = append(revenue.Accounts, row)
			revenue.Total += row.Amount
		case "EXPENSE", "COGS":
			expense.Accounts = append(expense.Accounts, row)
			expense.Total += row.Amount
		}
	}

	sort.Slice(revenue.Accounts, func(i, j int) bool { return revenue.Accounts[i].Code < revenue.Accounts[j].Code })
	sort.Slice(expense.Accounts, func(i, j int) bool { return expense.Accounts[i].Code < expense.Accounts[j].Code })

	return ProfitAndLoss{
		Revenue:   revenue,
		Expense:   expense,
		NetIncome: revenue.Total - expense.Total,
	}
}
