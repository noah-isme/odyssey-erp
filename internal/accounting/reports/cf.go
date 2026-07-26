package reports

import "strings"

// CashFlowLine represents a cash flow line item.
type CashFlowLine struct {
	AccountCode string
	AccountName string
	Amount      float64
}

// CashFlowSection represents a section in the cash flow statement.
type CashFlowSection struct {
	Label string
	Lines []CashFlowLine
	Total float64
}

// CashFlow represents the structured output for the cash flow report.
type CashFlow struct {
	Operating   CashFlowSection
	Investing   CashFlowSection
	Financing   CashFlowSection
	NetChange   float64
	OpeningCash float64
	ClosingCash float64
}

// cashFlowClass says how an account takes part in the statement.
type cashFlowClass int

const (
	// classCash marks the accounts whose movement the statement explains.
	classCash cashFlowClass = iota
	classOperating
	classInvesting
	classFinancing
)

// cashAccountPrefix identifies cash and bank accounts, and fixedAssetPrefix
// identifies the investing block. Both follow the chart of accounts convention
// where 11xx is cash and bank and 14xx is fixed assets.
const (
	cashAccountPrefix = "11"
	fixedAssetPrefix  = "14"
)

// classifyCashFlow assigns an account to a section of the statement.
//
// Working capital accounts - receivables, inventory, payables, accruals - are
// operating, capital expenditure is investing, and owner's capital is
// financing. Revenue and expense accounts are operating because the statement
// is built indirectly from movements rather than from cash receipts.
func classifyCashFlow(acc AccountBalance) cashFlowClass {
	switch strings.ToUpper(acc.Type) {
	case "ASSET":
		switch {
		case strings.HasPrefix(acc.Code, cashAccountPrefix):
			return classCash
		case strings.HasPrefix(acc.Code, fixedAssetPrefix):
			return classInvesting
		default:
			return classOperating
		}
	case "EQUITY":
		return classFinancing
	default:
		// Liabilities, revenue and expenses all move operating cash.
		return classOperating
	}
}

// BuildCashFlow produces a cash flow statement that reconciles to the movement
// on the cash accounts.
//
// Under double entry the debits and credits of every account sum to zero, so
// the change in cash is the negative of every other account's movement. Each
// non-cash account therefore contributes credit - debit: a receivable that
// grows consumes cash, a payable that grows releases it. Summing those
// contributions reproduces the cash accounts' own movement exactly, which is
// the property that makes the statement worth reading.
func BuildCashFlow(accounts []AccountBalance) CashFlow {
	operating := CashFlowSection{Label: "Operating Activities"}
	investing := CashFlowSection{Label: "Investing Activities"}
	financing := CashFlowSection{Label: "Financing Activities"}

	var openingCash, closingCash float64

	for _, acc := range accounts {
		class := classifyCashFlow(acc)
		if class == classCash {
			// Cash accounts are the subject of the statement, not a line in it.
			openingCash += acc.Opening
			closingCash += acc.Closing()
			continue
		}

		// The cash effect is the opposite of the account's own movement.
		amount := acc.Credit - acc.Debit
		if amount == 0 {
			continue
		}

		line := CashFlowLine{
			AccountCode: acc.Code,
			AccountName: acc.Name,
			Amount:      amount,
		}
		switch class {
		case classInvesting:
			investing.Lines = append(investing.Lines, line)
			investing.Total += amount
		case classFinancing:
			financing.Lines = append(financing.Lines, line)
			financing.Total += amount
		default:
			operating.Lines = append(operating.Lines, line)
			operating.Total += amount
		}
	}

	return CashFlow{
		Operating:   operating,
		Investing:   investing,
		Financing:   financing,
		NetChange:   operating.Total + investing.Total + financing.Total,
		OpeningCash: openingCash,
		ClosingCash: closingCash,
	}
}
