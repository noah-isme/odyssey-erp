package ap

import (
	"time"
)

// APInvoiceStatus enumerates AP invoice statuses.
type APInvoiceStatus string

const (
	APStatusDraft  APInvoiceStatus = "DRAFT"
	APStatusPosted APInvoiceStatus = "POSTED"
	APStatusPaid   APInvoiceStatus = "PAID"
	APStatusVoid   APInvoiceStatus = "VOID"
)

// APInvoice model.
type APInvoice struct {
	ID           int64
	Number       string
	SupplierID   int64
	SupplierName string
	GRNID        *int64
	POID         *int64
	Currency     string
	Subtotal     float64
	TaxAmount    float64
	Total        float64
	Status       APInvoiceStatus
	DueAt        time.Time
	PostedAt     *time.Time
	PostedBy     *int64
	VoidedAt     *time.Time
	VoidedBy     *int64
	VoidReason   *string
	CreatedBy    int64
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// APInvoiceLine represents a line item on an AP invoice.
type APInvoiceLine struct {
	ID          int64
	APInvoiceID int64
	GRNLineID   *int64
	ProductID   int64
	Description string
	Quantity    float64
	UnitPrice   float64
	DiscountPct float64
	TaxPct      float64
	Subtotal    float64
	TaxAmount   float64
	Total       float64
	CreatedAt   time.Time
}

// APInvoiceWithDetails includes invoice with lines and supplier info.
type APInvoiceWithDetails struct {
	APInvoice
	SupplierName string
	Lines        []APInvoiceLine
	Payments     []APPaymentSummary
	PaidAmount   float64
	Balance      float64
}

// APPayment model.
type APPayment struct {
	ID           int64
	Number       string
	APInvoiceID  *int64 // Optional direct link
	SupplierID   int64
	SupplierName string
	Amount       float64
	PaidAt       time.Time
	Method       string
	Note         string
	CreatedBy    int64
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// APPaymentSummary for display in invoice detail.
type APPaymentSummary struct {
	ID              int64
	Number          string
	Amount          float64
	AllocatedAmount float64
	PaidAt          time.Time
	Method          string
	Note            string
}

// APPaymentAllocation tracks how payments are applied to invoices.
type APPaymentAllocation struct {
	ID          int64
	APPaymentID int64
	APInvoiceID int64
	Amount      float64
	CreatedAt   time.Time
}

// APPaymentAllocationDetail includes invoice context for a payment allocation.
type APPaymentAllocationDetail struct {
	ID            int64
	APPaymentID   int64
	APInvoiceID   int64
	InvoiceNumber string
	POID          *int64
	InvoiceStatus APInvoiceStatus
	InvoiceTotal  float64
	DueAt         time.Time
	Amount        float64
}

// APPaymentWithDetails includes payment with allocation breakdown and ledger status.
type APPaymentWithDetails struct {
	APPayment
	Allocations    []APPaymentAllocationDetail
	TotalAllocated float64
	Unallocated    float64
	LedgerPosted   bool
}

// APAgingBucket summarises totals by aging periods.
type APAgingBucket struct {
	Current   float64
	Bucket30  float64
	Bucket60  float64
	Bucket90  float64
	Bucket120 float64
}

// APAgingDetail provides supplier-level aging breakdown.
type APAgingDetail struct {
	SupplierID   int64
	SupplierName string
	Current      float64
	Bucket30     float64
	Bucket60     float64
	Bucket90     float64
	Bucket120    float64
	Total        float64
}

// APInvoiceBalance represents an invoice balance for batch aging calculations.
type APInvoiceBalance struct {
	ID         int64
	DueAt      time.Time
	Total      float64
	PaidAmount float64
	Balance    float64
}

// --- Input DTOs ---

// CreateAPInvoiceInput for creating AP invoices.
type CreateAPInvoiceInput struct {
	SupplierID int64
	GRNID      *int64
	POID       *int64
	Number     string
	Currency   string
	Subtotal   float64
	TaxAmount  float64
	Total      float64
	DueDate    time.Time
	CreatedBy  int64
	Lines      []CreateAPInvoiceLineInput
}

// CreateAPInvoiceLineInput for invoice line items.
type CreateAPInvoiceLineInput struct {
	GRNLineID   *int64
	ProductID   int64
	Description string
	Quantity    float64
	UnitPrice   float64
	DiscountPct float64
	TaxPct      float64
}

// CreateAPInvoiceFromGRNInput creates invoice from goods receipt.
type CreateAPInvoiceFromGRNInput struct {
	GRNID     int64
	DueDate   time.Time
	CreatedBy int64
	Number    string
}

// CreateAPInvoiceFromPOInput creates invoice from purchase order.
type CreateAPInvoiceFromPOInput struct {
	POID      int64
	DueDate   time.Time
	CreatedBy int64
	Number    string
}

// PostAPInvoiceInput for posting an invoice.
type PostAPInvoiceInput struct {
	InvoiceID int64
	PostedBy  int64
}

// VoidAPInvoiceInput for voiding an invoice.
type VoidAPInvoiceInput struct {
	InvoiceID  int64
	VoidedBy   int64
	VoidReason string
}

// CreateAPPaymentInput for creating AP payments.
type CreateAPPaymentInput struct {
	Number      string
	SupplierID  int64
	Amount      float64
	PaidAt      time.Time
	Method      string
	Note        string
	CreatedBy   int64
	Allocations []PaymentAllocationInput
}

// PaymentAllocationInput for allocating payment to invoices.
type PaymentAllocationInput struct {
	APInvoiceID int64
	Amount      float64
}

// ListAPInvoicesRequest for filtering invoices.
type ListAPInvoicesRequest struct {
	Status     APInvoiceStatus
	SupplierID int64
	FromDate   time.Time
	ToDate     time.Time
	Limit      int
	Offset     int
}
