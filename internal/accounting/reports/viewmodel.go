package reports

// TrialBalanceViewModel holds SSR/PDF data for the trial balance report.
type TrialBalanceViewModel struct {
	CompanyName   string
	PeriodLabel   string
	BranchName    string
	FilterPeriod  string
	FilterBranch  string
	FilterCompany string
	Report        TrialBalance
}

// ProfitAndLossViewModel holds SSR/PDF data for profit & loss.
type ProfitAndLossViewModel struct {
	CompanyName   string
	PeriodLabel   string
	BranchName    string
	FilterPeriod  string
	FilterBranch  string
	FilterCompany string
	Report        ProfitAndLoss
}

// BalanceSheetViewModel contains data for the balance sheet report.
type BalanceSheetViewModel struct {
	CompanyName   string
	PeriodLabel   string
	BranchName    string
	FilterPeriod  string
	FilterBranch  string
	FilterCompany string
	Report        BalanceSheet
}
