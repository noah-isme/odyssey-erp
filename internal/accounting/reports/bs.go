package reports

import (
	"sort"
	"strings"
)

// BalanceSheetAccount summarises an account for assets, liabilities, or equity.
type BalanceSheetAccount struct {
	Code    string
	Name    string
	Balance float64
}

// BalanceSheetSection contains the accounts and totals for a classification.
type BalanceSheetSection struct {
	Label    string
	Accounts []BalanceSheetAccount
	Total    float64
}

// BalanceSheet is the structured response for the balance sheet report.
type BalanceSheet struct {
	Assets                    BalanceSheetSection
	Liabilities               BalanceSheetSection
	Equity                    BalanceSheetSection
	TotalLiabilitiesAndEquity float64
}

// BuildBalanceSheet aggregates balances into assets, liabilities, and equity sections.
func BuildBalanceSheet(accounts []AccountBalance) BalanceSheet {
	assets := BalanceSheetSection{Label: "Assets"}
	liabilities := BalanceSheetSection{Label: "Liabilities"}
	equity := BalanceSheetSection{Label: "Equity"}

	for _, acc := range accounts {
		balance := acc.Closing()
		row := BalanceSheetAccount{Code: acc.Code, Name: acc.Name, Balance: balance}
		switch strings.ToUpper(acc.Type) {
		case "ASSET":
			assets.Accounts = append(assets.Accounts, row)
			assets.Total += row.Balance
		case "LIABILITY":
			liabilities.Accounts = append(liabilities.Accounts, row)
			liabilities.Total += row.Balance
		case "EQUITY":
			equity.Accounts = append(equity.Accounts, row)
			equity.Total += row.Balance
		}
	}

	sort.Slice(assets.Accounts, func(i, j int) bool { return assets.Accounts[i].Code < assets.Accounts[j].Code })
	sort.Slice(liabilities.Accounts, func(i, j int) bool { return liabilities.Accounts[i].Code < liabilities.Accounts[j].Code })
	sort.Slice(equity.Accounts, func(i, j int) bool { return equity.Accounts[i].Code < equity.Accounts[j].Code })

	return BalanceSheet{
		Assets:                    assets,
		Liabilities:               liabilities,
		Equity:                    equity,
		TotalLiabilitiesAndEquity: liabilities.Total + equity.Total,
	}
}
