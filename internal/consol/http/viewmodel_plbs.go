package http

// ConsolPLViewModel represents the data required to render the consolidated profit and loss page.
type ConsolPLViewModel struct {
	Filters       ConsolPLFilters
	Lines         []ConsolPLLine
	Totals        ConsolPLTotals
	Contributions []ConsolPLEntityContribution
	Errors        map[string]string
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
