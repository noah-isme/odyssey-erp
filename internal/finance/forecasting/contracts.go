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

// ScenarioSourceReader is an optional extension for sources whose expected
// values belong to a particular forecast scenario. The base SourceReader
// contract remains company-scoped for shared ledger sources; scenario-owned
// inputs (for example manual or recurring adjustments) must use this method so
// one scenario can never leak into another scenario's snapshot.
type ScenarioSourceReader interface {
	SourceReader
	ReadExpectedFlowsForScenario(ctx context.Context, companyID, scenarioID int64, fromDate, toDate time.Time) ([]ExpectedCashFlow, error)
}

// ForecastSourceLine is the exact, persisted source identity shown alongside a
// forecast run. Amount is serialized as text so an API consumer never has to
// round a NUMERIC(19,4) value through float64.
type ForecastSourceLine struct {
	ID            int64     `json:"id"`
	RunID         int64     `json:"run_id"`
	DailyBucketID int64     `json:"daily_bucket_id"`
	SourceType    string    `json:"source_type"`
	SourceRef     string    `json:"source_ref"`
	Amount        string    `json:"amount"`
	Currency      string    `json:"currency"`
	ExpectedDate  time.Time `json:"expected_date"`
	Certainty     string    `json:"certainty"`
}
