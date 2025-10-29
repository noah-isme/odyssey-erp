package http

import "github.com/odyssey-erp/odyssey-erp/internal/consol"

// ConsolPLViewModel represents the data required to render the consolidated profit and loss page.
type ConsolPLViewModel struct {
	Filters       ConsolPLFilters
	Lines         []ConsolPLLine
	Totals        ConsolPLTotals
	Contributions []ConsolPLEntityContribution
	Errors        map[string]string
	Warnings      []string
}

// ConsolPLFilters captures the user provided filter inputs.
type ConsolPLFilters struct {
	GroupID  int64
	Period   string
	Entities []int64
	FxOn     bool
}

// ConsolPLLine represents a row in the P&L statement.
type ConsolPLLine struct {
	AccountCode string
	AccountName string
	LocalAmount float64
	GroupAmount float64
	Section     string
}

// ConsolPLTotals aggregates key sections of the P&L statement.
type ConsolPLTotals struct {
	Revenue     float64
	COGS        float64
	GrossProfit float64
	Opex        float64
	NetIncome   float64
	DeltaFX     float64
}

// ConsolPLEntityContribution represents member contribution to the consolidated P&L.
type ConsolPLEntityContribution struct {
	EntityName  string
	GroupAmount float64
	Percent     float64
}

// ConsolBSViewModel encapsulates the balance sheet data for rendering purposes.
type ConsolBSViewModel struct {
	Filters       ConsolBSFilters
	Assets        []ConsolBSLine
	LiabilitiesEq []ConsolBSLine
	Totals        ConsolBSTotals
	Contributions []ConsolBSEntityContribution
	Errors        map[string]string
	Warnings      []string
}

// ConsolBSFilters describes the balance sheet filter inputs.
type ConsolBSFilters struct {
	GroupID  int64
	Period   string
	Entities []int64
	FxOn     bool
}

// ConsolBSLine represents a single balance sheet row.
type ConsolBSLine struct {
	AccountCode string
	AccountName string
	LocalAmount float64
	GroupAmount float64
	Section     string
}

// ConsolBSTotals stores the aggregated totals for the balance sheet.
type ConsolBSTotals struct {
	Assets     float64
	LiabEquity float64
	Balanced   bool
	DeltaFX    float64
}

// ConsolBSEntityContribution holds entity contribution data for the balance sheet.
type ConsolBSEntityContribution struct {
	EntityName  string
	GroupAmount float64
	Percent     float64
}

// NewConsolPLViewModel transforms the domain report into a template friendly structure.
func NewConsolPLViewModel(report consol.ProfitLossReport, warnings []string) ConsolPLViewModel {
	vm := ConsolPLViewModel{
		Filters: ConsolPLFilters{
			GroupID:  report.Filters.GroupID,
			Period:   report.Filters.Period,
			Entities: append([]int64(nil), report.Filters.Entities...),
			FxOn:     report.Filters.FxOn,
		},
		Errors:   map[string]string{},
		Warnings: append([]string(nil), warnings...),
	}
	vm.Lines = make([]ConsolPLLine, len(report.Lines))
	for i, line := range report.Lines {
		vm.Lines[i] = ConsolPLLine{
			AccountCode: line.AccountCode,
			AccountName: line.AccountName,
			LocalAmount: line.LocalAmount,
			GroupAmount: line.GroupAmount,
			Section:     line.Section,
		}
	}
	vm.Totals = ConsolPLTotals{
		Revenue:     report.Totals.Revenue,
		COGS:        report.Totals.COGS,
		GrossProfit: report.Totals.GrossProfit,
		Opex:        report.Totals.Opex,
		NetIncome:   report.Totals.NetIncome,
		DeltaFX:     report.Totals.DeltaFX,
	}
	vm.Contributions = make([]ConsolPLEntityContribution, len(report.Contributions))
	for i, contrib := range report.Contributions {
		vm.Contributions[i] = ConsolPLEntityContribution{
			EntityName:  contrib.EntityName,
			GroupAmount: contrib.GroupAmount,
			Percent:     contrib.Percent,
		}
	}
	return vm
}

// NewConsolBSViewModel maps the balance sheet report to a view model.
func NewConsolBSViewModel(report consol.BalanceSheetReport, warnings []string) ConsolBSViewModel {
	vm := ConsolBSViewModel{
		Filters: ConsolBSFilters{
			GroupID:  report.Filters.GroupID,
			Period:   report.Filters.Period,
			Entities: append([]int64(nil), report.Filters.Entities...),
			FxOn:     report.Filters.FxOn,
		},
		Errors:   map[string]string{},
		Warnings: append([]string(nil), warnings...),
	}
	vm.Assets = make([]ConsolBSLine, len(report.Assets))
	for i, line := range report.Assets {
		vm.Assets[i] = ConsolBSLine{
			AccountCode: line.AccountCode,
			AccountName: line.AccountName,
			LocalAmount: line.LocalAmount,
			GroupAmount: line.GroupAmount,
			Section:     line.Section,
		}
	}
	vm.LiabilitiesEq = make([]ConsolBSLine, len(report.LiabilitiesEq))
	for i, line := range report.LiabilitiesEq {
		vm.LiabilitiesEq[i] = ConsolBSLine{
			AccountCode: line.AccountCode,
			AccountName: line.AccountName,
			LocalAmount: line.LocalAmount,
			GroupAmount: line.GroupAmount,
			Section:     line.Section,
		}
	}
	vm.Totals = ConsolBSTotals{
		Assets:     report.Totals.Assets,
		LiabEquity: report.Totals.LiabEquity,
		Balanced:   report.Totals.Balanced,
		DeltaFX:    report.Totals.DeltaFX,
	}
	vm.Contributions = make([]ConsolBSEntityContribution, len(report.Contributions))
	for i, contrib := range report.Contributions {
		vm.Contributions[i] = ConsolBSEntityContribution{
			EntityName:  contrib.EntityName,
			GroupAmount: contrib.GroupAmount,
			Percent:     contrib.Percent,
		}
	}
	return vm
}
