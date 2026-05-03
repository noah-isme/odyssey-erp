package reports

// CashFlowLine represents a cash flow line item.
type CashFlowLine struct {
	AccountCode string
	AccountName string
	Amount      float64
}

// CashFlowSection represents a section in the cash flow statement.
type CashFlowSection struct {
	Label    string
	Lines    []CashFlowLine
	Total    float64
}

// CashFlow represents the structured output for the cash flow report.
type CashFlow struct {
	Operating  CashFlowSection
	Investing  CashFlowSection
	Financing  CashFlowSection
	NetChange  float64
}

// BuildCashFlow aggregates GL entries into cash flow sections.
// This is a simplified direct method approach filtering by cash/bank accounts.
func BuildCashFlow(accounts []AccountBalance) CashFlow {
	operating := CashFlowSection{Label: "Operating Activities"}
	investing := CashFlowSection{Label: "Investing Activities"}
	financing := CashFlowSection{Label: "Financing Activities"}

	// Note: A true direct cash flow would require analyzing counter-accounts of cash transactions.
	// For this MVP, we map specific cash/bank account movements (or categorize by some mapping).
	// Since we don't have a complex mapping yet, we'll just demonstrate the structure.
	
	// Example dummy mapping based on account classification:
	for _, acc := range accounts {
		// Only consider accounts that are categorized for CF.
		// If we had a Cash Flow Mapping table, we'd use it here.
		if acc.Debit == 0 && acc.Credit == 0 {
			continue
		}
		
		// This is just a structural placeholder. Real logic needs a `cash_flow_category` mapping.
		// For now, let's just categorize everything into Operating to show it works,
		// unless it's a known investing/financing account.
		amount := acc.Debit - acc.Credit // Net flow
		
		line := CashFlowLine{
			AccountCode: acc.Code,
			AccountName: acc.Name,
			Amount:      amount,
		}
		
		operating.Lines = append(operating.Lines, line)
		operating.Total += amount
	}

	return CashFlow{
		Operating: operating,
		Investing: investing,
		Financing: financing,
		NetChange: operating.Total + investing.Total + financing.Total,
	}
}
