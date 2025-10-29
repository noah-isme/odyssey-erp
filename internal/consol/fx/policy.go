package fx

// Policy describes the FX conversion behaviour for consolidated reports.
type Policy struct {
	ReportingCurrency  string
	ProfitLossMethod   Method
	BalanceSheetMethod Method
}

// Method enumerates supported FX conversion methods.
type Method string

const (
	// MethodAverage represents average rate usage for P&L.
	MethodAverage Method = "AVERAGE"
	// MethodClosing represents closing rate usage for balance sheet.
	MethodClosing Method = "CLOSING"
)

// DefaultPolicy returns a baseline configuration aligned with the consolidation requirements.
func DefaultPolicy() Policy {
	return Policy{
		ProfitLossMethod:   MethodAverage,
		BalanceSheetMethod: MethodClosing,
	}
}
