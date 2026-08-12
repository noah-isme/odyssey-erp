package forecasting

import (
	"context"
	"time"

	"github.com/odyssey-erp/odyssey-erp/internal/finance/automation"
)

type SourceType string
type Certainty string

const (
	SourceTypeBankBalance      SourceType = "BANK_BALANCE"
	SourceTypeOpenAR           SourceType = "OPEN_AR"
	SourceTypePostedAP         SourceType = "POSTED_AP"
	SourceTypeApprovedPayroll  SourceType = "APPROVED_PAYROLL"
	SourceTypeTaxObligation    SourceType = "TAX_OBLIGATION"
	SourceTypeApprovedPO       SourceType = "APPROVED_PO"
	SourceTypeApprovedPayment  SourceType = "APPROVED_PAYMENT"
	SourceTypeManualAdjustment SourceType = "MANUAL_ADJUSTMENT"

	CertaintyCommitted Certainty = "COMMITTED"
	CertaintyProbable  Certainty = "PROBABLE"
)

// ExpectedCashFlow represents a granular expected cash movement or a starting balance.
type ExpectedCashFlow struct {
	SourceType SourceType
	SourceRef  string // Stable source key to prevent double counting
	Amount     automation.ExactAmount
	Currency   string
	Date       time.Time
	Certainty  Certainty
}

// SourceReader is an interface that allows the forecast engine to fetch expected cash flows
// from various domains (e.g., banking, AP, AR, payroll).
type SourceReader interface {
	Name() string
	ReadExpectedFlows(ctx context.Context, companyID int64, fromDate, toDate time.Time) ([]ExpectedCashFlow, error)
}
