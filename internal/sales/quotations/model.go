package quotations

import "time"

type QuotationStatus string

const (
	QuotationStatusDraft     QuotationStatus = "DRAFT"
	QuotationStatusSubmitted QuotationStatus = "SUBMITTED"
	QuotationStatusApproved  QuotationStatus = "APPROVED"
	QuotationStatusRejected  QuotationStatus = "REJECTED"
	QuotationStatusConverted QuotationStatus = "CONVERTED"
)

type Quotation struct {
	ID              int64            `json:"id" db:"id"`
	DocNumber       string           `json:"doc_number" db:"doc_number"`
	CompanyID       int64            `json:"company_id" db:"company_id"`
	CustomerID      int64            `json:"customer_id" db:"customer_id"`
	QuoteDate       time.Time        `json:"quote_date" db:"quote_date"`
	ValidUntil      time.Time        `json:"valid_until" db:"valid_until"`
	Status          QuotationStatus  `json:"status" db:"status"`
	Currency        string           `json:"currency" db:"currency"`
	Subtotal        float64          `json:"subtotal" db:"subtotal"`
	TaxAmount       float64          `json:"tax_amount" db:"tax_amount"`
	TotalAmount     float64          `json:"total_amount" db:"total_amount"`
	Notes           *string          `json:"notes,omitempty" db:"notes"`
	CreatedBy       int64            `json:"created_by" db:"created_by"`
	ApprovedBy      *int64           `json:"approved_by,omitempty" db:"approved_by"`
	ApprovedAt      *time.Time       `json:"approved_at,omitempty" db:"approved_at"`
	RejectedBy      *int64           `json:"rejected_by,omitempty" db:"rejected_by"`
	RejectedAt      *time.Time       `json:"rejected_at,omitempty" db:"rejected_at"`
	RejectionReason *string          `json:"rejection_reason,omitempty" db:"rejection_reason"`
	CreatedAt       time.Time        `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time        `json:"updated_at" db:"updated_at"`
	Lines           []QuotationLine  `json:"lines,omitempty" db:"-"`
}

type QuotationLine struct {
	ID              int64     `json:"id" db:"id"`
	QuotationID     int64     `json:"quotation_id" db:"quotation_id"`
	ProductID       int64     `json:"product_id" db:"product_id"`
	Description     *string   `json:"description,omitempty" db:"description"`
	Quantity        float64   `json:"quantity" db:"quantity"`
	UOM             string    `json:"uom" db:"uom"`
	UnitPrice       float64   `json:"unit_price" db:"unit_price"`
	DiscountPercent float64   `json:"discount_percent" db:"discount_percent"`
	DiscountAmount  float64   `json:"discount_amount" db:"discount_amount"`
	TaxPercent      float64   `json:"tax_percent" db:"tax_percent"`
	TaxAmount       float64   `json:"tax_amount" db:"tax_amount"`
	LineTotal       float64   `json:"line_total" db:"line_total"`
	Notes           *string   `json:"notes,omitempty" db:"notes"`
	LineOrder       int       `json:"line_order" db:"line_order"`
	CreatedAt       time.Time `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time `json:"updated_at" db:"updated_at"`
}

type QuotationWithDetails struct {
	Quotation
	CustomerName  string  `json:"customer_name" db:"customer_name"`
	CreatedByName string  `json:"created_by_name" db:"created_by_name"`
	ApprovedByName *string `json:"approved_by_name,omitempty" db:"approved_by_name"`
	RejectedByName *string `json:"rejected_by_name,omitempty" db:"rejected_by_name"`
}
