package procurement

import (
	"time"

	accountingmoney "github.com/odyssey-erp/odyssey-erp/internal/accounting/money"
)

// ContractStatus represents the lifecycle state of a supplier contract
type ContractStatus string

const (
	ContractStatusDraft    ContractStatus = "DRAFT"
	ContractStatusApproval ContractStatus = "APPROVAL"
	ContractStatusActive   ContractStatus = "ACTIVE"
	ContractStatusExpired  ContractStatus = "EXPIRED"
	ContractStatusTerminated ContractStatus = "TERMINATED"
)

// SupplierContract represents a versioned supplier contract with effective dates
type SupplierContract struct {
	ID                     int64
	CompanyID              int64
	SupplierID             int64
	Version                int
	Status                 ContractStatus
	Currency               string
	EffectiveFrom          time.Time
	EffectiveTo            *time.Time
	PaymentTerms           string
	Incoterms              string
	RenewalNoticeDays      int
	ExpiryNotificationSent bool
	CreatedBy              int64
	ApprovedBy             *int64
	ApprovedAt             *time.Time
	TerminatedAt           *time.Time
	Note                   string
	CreatedAt              time.Time
	UpdatedAt              time.Time
	PriceLines             []ContractPriceLine
}

// ContractPriceLine represents a quantity-tiered price within a contract
type ContractPriceLine struct {
	ID           int64
	ContractID   int64
	ProductID    int64
	MinQuantity  accountingmoney.Money
	UnitPrice    accountingmoney.Money
	TaxRate      accountingmoney.Money
	LeadTimeDays int
	MOQ          accountingmoney.Money
}

// CreateContractInput is used to create a new supplier contract
type CreateContractInput struct {
	CompanyID         int64
	SupplierID        int64
	Currency          string
	EffectiveFrom     time.Time
	EffectiveTo       *time.Time
	PaymentTerms      string
	Incoterms         string
	RenewalNoticeDays int
	CreatedBy         int64
	Note              string
	PriceLines        []ContractPriceLineInput
}

// ContractPriceLineInput is used to input price tiers
type ContractPriceLineInput struct {
	ContractID   int64
	ProductID    int64
	MinQuantity  accountingmoney.Money
	UnitPrice    accountingmoney.Money
	TaxRate      accountingmoney.Money
	LeadTimeDays int
	MOQ          accountingmoney.Money
}

// PriceHistorySource represents the source of a price observation
type PriceHistorySource string

const (
	PriceHistorySourceBid      PriceHistorySource = "BID"
	PriceHistorySourceAward    PriceHistorySource = "AWARD"
	PriceHistorySourceContract PriceHistorySource = "CONTRACT"
	PriceHistorySourcePO       PriceHistorySource = "PO"
)

// PriceHistory is an immutable record of supplier/product pricing
type PriceHistory struct {
	ID                int64
	CompanyID         int64
	SupplierID        int64
	ProductID         int64
	SourceType        PriceHistorySource
	SourceID          int64
	Currency          string
	UnitPrice         accountingmoney.Money
	Quantity          accountingmoney.Money
	TaxRate           accountingmoney.Money
	MOQ               accountingmoney.Money
	LeadTimeDays      int
	FXRate            *accountingmoney.Money
	BaseCurrencyPrice *accountingmoney.Money
	ObservationDate   time.Time
	Note              string
	CreatedAt         time.Time
}

// RecordPriceHistoryInput is used to record a price observation
type RecordPriceHistoryInput struct {
	CompanyID    int64
	SupplierID   int64
	ProductID    int64
	SourceType   PriceHistorySource
	SourceID     int64
	Currency     string
	UnitPrice    accountingmoney.Money
	Quantity     accountingmoney.Money
	TaxRate      accountingmoney.Money
	MOQ          accountingmoney.Money
	LeadTimeDays int
	FXRate       *accountingmoney.Money
	Note         string
}

// ScorecardStatus represents publication state
type ScorecardStatus string

const (
	ScorecardStatusDraft     ScorecardStatus = "DRAFT"
	ScorecardStatusPublished ScorecardStatus = "PUBLISHED"
)

// SupplierScorecard represents a versioned, immutable supplier performance scorecard
type SupplierScorecard struct {
	ID                      int64
	CompanyID               int64
	SupplierID              int64
	Version                 int
	PeriodStart             time.Time
	PeriodEnd               time.Time
	Status                  ScorecardStatus
	DeliveryOTIFScore       accountingmoney.Money
	DeliveryOTIFWeight      int
	DeliveryOTIFSampleSize  int
	QualityScore            accountingmoney.Money
	QualityWeight           int
	QualitySampleSize       int
	PriceAdherenceScore     accountingmoney.Money
	PriceAdherenceWeight    int
	PriceAdherenceSampleSize int
	RFQResponsivenessScore  accountingmoney.Money
	RFQResponsivenessWeight int
	RFQResponsivenessSampleSize int
	ReviewerAssessmentScore accountingmoney.Money
	ReviewerAssessmentWeight int
	OverallScore            accountingmoney.Money
	PublishedBy             *int64
	PublishedAt             *time.Time
	Note                    string
	CreatedBy               int64
	CreatedAt               time.Time
}

// CreateScorecardInput is used to create a draft scorecard
type CreateScorecardInput struct {
	CompanyID              int64
	SupplierID             int64
	PeriodStart            time.Time
	PeriodEnd              time.Time
	CreatedBy              int64
	ReviewerAssessmentScore *accountingmoney.Money
	Note                   string
}

// PublishScorecardInput is used to publish a scorecard
type PublishScorecardInput struct {
	ScorecardID int64
	PublishedBy int64
}

// VarianceType represents the type of PO contract variance
type VarianceType string

const (
	VarianceTypeNoContract       VarianceType = "NO_CONTRACT"
	VarianceTypeExpiredContract  VarianceType = "EXPIRED_CONTRACT"
	VarianceTypePriceVariance    VarianceType = "PRICE_VARIANCE"
	VarianceTypeTermVariance     VarianceType = "TERM_VARIANCE"
)

// ApprovalStatus for variance exceptions
type ApprovalStatus string

const (
	ApprovalStatusPending  ApprovalStatus = "PENDING"
	ApprovalStatusApproved ApprovalStatus = "APPROVED"
	ApprovalStatusRejected ApprovalStatus = "REJECTED"
)

// POContractVariance represents a deviation from contract terms requiring approval
type POContractVariance struct {
	ID                int64
	CompanyID         int64
	POID              int64
	POLineID          int64
	ContractID        *int64
	VarianceType      VarianceType
	VariancePercentage *accountingmoney.Money
	VarianceReason    string
	ApprovalStatus    ApprovalStatus
	ApprovedBy        *int64
	ApprovedAt        *time.Time
	Note              string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// CreatePOVarianceInput is used to record a variance exception
type CreatePOVarianceInput struct {
	CompanyID           int64
	POID                int64
	POLineID            int64
	ContractID          *int64
	VarianceType        VarianceType
	VariancePercentage  *accountingmoney.Money
	VarianceReason      string
	Note                string
}
