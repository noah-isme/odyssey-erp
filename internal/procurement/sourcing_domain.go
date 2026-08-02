package procurement

import (
	"time"

	accountingmoney "github.com/odyssey-erp/odyssey-erp/internal/accounting/money"
)

type RFQStatus string

const (
	RFQStatusDraft     RFQStatus = "DRAFT"
	RFQStatusIssued    RFQStatus = "ISSUED"
	RFQStatusClosed    RFQStatus = "CLOSED"
	RFQStatusAwarded   RFQStatus = "AWARDED"
	RFQStatusCancelled RFQStatus = "CANCELLED"
)

type BidStatus string

const (
	BidStatusDraft        BidStatus = "DRAFT"
	BidStatusSubmitted    BidStatus = "SUBMITTED"
	BidStatusWithdrawn    BidStatus = "WITHDRAWN"
	BidStatusDisqualified BidStatus = "DISQUALIFIED"
)

type AwardStatus string

const (
	AwardStatusDraft    AwardStatus = "DRAFT"
	AwardStatusApproval AwardStatus = "APPROVAL"
	AwardStatusApproved AwardStatus = "APPROVED"
	AwardStatusRejected AwardStatus = "REJECTED"
)

type RFQWeights struct {
	Price, LeadTime, Terms, SupplierRating int
}

func DefaultRFQWeights() RFQWeights {
	return RFQWeights{Price: 50, LeadTime: 20, Terms: 10, SupplierRating: 20}
}

func (w RFQWeights) Valid() bool {
	return w.Price >= 0 && w.LeadTime >= 0 && w.Terms >= 0 && w.SupplierRating >= 0 &&
		w.Price+w.LeadTime+w.Terms+w.SupplierRating == 100
}

type RFQ struct {
	ID, CompanyID, CreatedBy int64
	Number, Currency         string
	Status                   RFQStatus
	ResponseDueAt            time.Time
	CommercialTerms          string
	Weights                  RFQWeights
	Version                  int
	Lines                    []RFQLine
	Suppliers                []int64
}

type RFQLine struct {
	ID, PRLineID, ProductID int64
	Quantity                string
	Note                    string
	LineOrder               int
}

type CreateRFQInput struct {
	CompanyID, CreatedBy int64
	Number, Currency     string
	ResponseDueAt        time.Time
	CommercialTerms      string
	Weights              RFQWeights
	Lines                []RFQLineInput
	SupplierIDs          []int64
}

type RFQLineInput struct {
	PRLineID, ProductID int64
	Quantity            string
	Note                string
}

type BidLineInput struct {
	RFQLineID            int64
	Quantity             string
	UnitPrice            accountingmoney.Money
	TaxAmount            accountingmoney.Money
	FreightAmount        accountingmoney.Money
	MinimumOrderQuantity string
	LeadTimeDays         int
	CommercialScore      int
	SupplierRatingScore  int
	Note                 string
}

type CreateBidInput struct {
	RFQID, SupplierID, CompanyID, CreatedBy int64
	Currency                                string
	FXRate                                  string
	FXRateDate                              time.Time
	PaymentTerms, SourceReference           string
	ValidUntil                              *time.Time
	Lines                                   []BidLineInput
}

type AwardLineInput struct {
	RFQLineID, BidLineID int64
	Quantity             string
}

type CreateAwardInput struct {
	RFQID, CompanyID, ExpectedWarehouseID, CreatedBy int64
	Note                                             string
	Lines                                            []AwardLineInput
}

type Award struct {
	ID, RFQID, CompanyID, ExpectedWarehouseID, CreatedBy int64
	Status                                               AwardStatus
	Note                                                 string
	Version                                              int
}

type ComparisonEntry struct {
	RFQLineID       int64  `json:"rfq_line_id"`
	BidID           int64  `json:"bid_id"`
	BidLineID       int64  `json:"bid_line_id"`
	SupplierID      int64  `json:"supplier_id"`
	TotalBaseAmount string `json:"total_base_amount"`
	Score           string `json:"score"`
	Rank            int    `json:"rank"`
}

type ComparisonSnapshot struct {
	RFQID, Version int64
	Entries        []ComparisonEntry
}
