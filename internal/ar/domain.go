package ar

import (
	"time"

	accountingmoney "github.com/odyssey-erp/odyssey-erp/internal/accounting/money"
	"github.com/odyssey-erp/odyssey-erp/internal/fx"
)

// ARInvoiceStatus enumerates AR invoice statuses.
type ARInvoiceStatus string

const (
	ARStatusDraft  ARInvoiceStatus = "DRAFT"
	ARStatusPosted ARInvoiceStatus = "POSTED"
	ARStatusPaid   ARInvoiceStatus = "PAID"
	ARStatusVoid   ARInvoiceStatus = "VOID"
)

// ARInvoice model.
type ARInvoice struct {
	ID              int64
	Number          string
	CustomerID      int64
	SOID            int64
	DeliveryOrderID int64
	Currency        string
	OriginalAmount  accountingmoney.Money
	BaseCurrency    string
	BaseAmount      accountingmoney.Money
	FXRate          fx.Decimal
	FXRateDate      time.Time
	FXRateSource    string
	FXRateLockedAt  time.Time
	// The float fields below are legacy UI compatibility fields. New
	// accounting calculations must use the exact valuation fields above.
	Subtotal  float64
	TaxAmount float64
	Total     float64
	Status    ARInvoiceStatus
	DueAt     time.Time
	// CustomerName is populated by listing queries that join customers; other
	// reads leave it empty.
	CustomerName string
	PostedAt     *time.Time
	PostedBy     *int64
	VoidedAt     *time.Time
	VoidedBy     *int64
	VoidReason   string
	CreatedBy    int64
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// ARInvoiceLine represents a line item on an AR invoice.
type ARInvoiceLine struct {
	ID                  int64
	ARInvoiceID         int64
	DeliveryOrderLineID int64
	ProductID           int64
	Description         string
	Quantity            float64
	UnitPrice           float64
	DiscountPct         float64
	TaxPct              float64
	Subtotal            float64
	TaxAmount           float64
	Total               float64
	CreatedAt           time.Time
}

// ARInvoiceWithDetails includes invoice with lines and payment allocation.
// CustomerName comes from the embedded ARInvoice.
type ARInvoiceWithDetails struct {
	ARInvoice
	Lines      []ARInvoiceLine
	Payments   []ARPaymentSummary
	PaidAmount float64
	Balance    float64
}

// ARPayment model.
type ARPayment struct {
	ID             int64
	Number         string
	ARInvoiceID    int64
	Amount         float64
	Currency       string
	OriginalAmount accountingmoney.Money
	BaseCurrency   string
	BaseAmount     accountingmoney.Money
	FXRate         fx.Decimal
	FXRateDate     time.Time
	FXRateSource   string
	FXRateLockedAt time.Time
	PaidAt         time.Time
	Method         string
	Note           string
	CreatedBy      int64
	CreatedAt      time.Time
	UpdatedAt      time.Time
	// InvoiceNumber and CustomerName are populated by listing queries that
	// join the invoice and its customer; other reads leave them empty.
	InvoiceNumber string
	CustomerName  string
}

// ARPaymentSummary for display in invoice detail.
type ARPaymentSummary struct {
	ID              int64
	Number          string
	Amount          float64
	AllocatedAmount float64
	PaidAt          time.Time
	Method          string
	Note            string
}

// ARPaymentAllocation tracks how payments are applied to invoices.
type ARPaymentAllocation struct {
	ID             int64
	ARPaymentID    int64
	ARInvoiceID    int64
	Amount         float64
	Currency       string
	OriginalAmount accountingmoney.Money
	BaseCurrency   string
	BaseAmount     accountingmoney.Money
	FXRate         fx.Decimal
	FXRateDate     time.Time
	FXRateSource   string
	FXRateLockedAt time.Time
	CreatedAt      time.Time
}

// ARAgingBucket summarises totals by aging periods.
type ARAgingBucket struct {
	Current   float64
	Bucket30  float64
	Bucket60  float64
	Bucket90  float64
	Bucket120 float64
}

// ARAgingDetail provides customer-level aging breakdown.
type ARAgingDetail struct {
	CustomerID   int64
	CustomerName string
	Current      float64
	Bucket30     float64
	Bucket60     float64
	Bucket90     float64
	Bucket120    float64
	Total        float64
}

// --- Input DTOs ---

// CreateARInvoiceInput for creating AR invoices.
type CreateARInvoiceInput struct {
	CustomerID      int64
	SOID            int64
	DeliveryOrderID int64
	Number          string
	Currency        string
	Subtotal        float64
	TaxAmount       float64
	Total           float64
	DueDate         time.Time
	CreatedBy       int64
	Lines           []CreateARInvoiceLineInput
}

// CreateARInvoiceLineInput for invoice line items.
type CreateARInvoiceLineInput struct {
	DeliveryOrderLineID int64
	ProductID           int64
	Description         string
	Quantity            float64
	UnitPrice           float64
	DiscountPct         float64
	TaxPct              float64
}

// CreateARInvoiceFromDeliveryInput creates invoice from delivery order.
type CreateARInvoiceFromDeliveryInput struct {
	DeliveryOrderID int64
	DueDate         time.Time
	CreatedBy       int64
}

// PostARInvoiceInput for posting an invoice.
type PostARInvoiceInput struct {
	InvoiceID int64
	PostedBy  int64
}

// VoidARInvoiceInput for voiding an invoice.
type VoidARInvoiceInput struct {
	InvoiceID  int64
	VoidedBy   int64
	VoidReason string
}

// CreateARPaymentInput for creating AR payments.
type CreateARPaymentInput struct {
	Number      string
	Currency    string
	Amount      float64
	PaidAt      time.Time
	Method      string
	Note        string
	CreatedBy   int64
	Allocations []PaymentAllocationInput
}

// PaymentAllocationInput for allocating payment to invoices.
type PaymentAllocationInput struct {
	ARInvoiceID int64   `json:"ar_invoice_id"`
	Amount      float64 `json:"amount"`
}

// InitiateOnlinePaymentInput for processing card charges via integrations like Stripe.
type InitiateOnlinePaymentInput struct {
	CompanyID    int64
	InvoiceID    int64
	ConnectionID int64
	SourceToken  string
}

// ListARInvoicesRequest for filtering invoices.
type ListARInvoicesRequest struct {
	Status     ARInvoiceStatus
	CustomerID int64
	FromDate   time.Time
	ToDate     time.Time
	Limit      int
	Offset     int
}

// ARCreditNoteStatus enumerates AR credit note statuses.
type ARCreditNoteStatus string

const (
	ARCreditNoteStatusDraft  ARCreditNoteStatus = "DRAFT"
	ARCreditNoteStatusPosted ARCreditNoteStatus = "POSTED"
	ARCreditNoteStatusVoid   ARCreditNoteStatus = "VOID"
)

// ARCreditNote model.
type ARCreditNote struct {
	ID                    int64
	Number                string
	CustomerID            int64
	ARInvoiceID           int64
	ReturnDeliveryOrderID int64
	Currency              string
	Reason                string
	Subtotal              float64
	TaxAmount             float64
	Total                 float64
	Status                ARCreditNoteStatus
	PostedAt              *time.Time
	PostedBy              *int64
	VoidedAt              *time.Time
	VoidedBy              *int64
	VoidReason            string
	CreatedBy             int64
	CreatedAt             time.Time
	UpdatedAt             time.Time
	CustomerName          string
	InvoiceNumber         string
}

// ARCreditNoteLine represents a line item on a credit note.
type ARCreditNoteLine struct {
	ID                        int64
	ARCreditNoteID            int64
	ARInvoiceLineID           int64
	ReturnDeliveryOrderLineID int64
	ProductID                 int64
	Description               string
	Quantity                  float64
	UnitPrice                 float64
	DiscountPct               float64
	TaxPct                    float64
	Subtotal                  float64
	TaxAmount                 float64
	Total                     float64
	CreatedAt                 time.Time
}

// ARCreditNoteWithDetails includes credit note with lines and allocation.
type ARCreditNoteWithDetails struct {
	ARCreditNote
	Lines []ARCreditNoteLine
}

// ARCreditNoteAllocation tracks how credit notes are applied to invoices.
type ARCreditNoteAllocation struct {
	ID             int64
	ARCreditNoteID int64
	ARInvoiceID    int64
	Amount         float64
	CreatedAt      time.Time
}

// CreateARCreditNoteInput for creating credit notes.
type CreateARCreditNoteInput struct {
	CustomerID            int64
	ARInvoiceID           int64
	ReturnDeliveryOrderID int64
	Number                string
	Currency              string
	Reason                string
	CreatedBy             int64
	Lines                 []CreateARCreditNoteLineInput
}

// CreateARCreditNoteLineInput for credit note line items.
type CreateARCreditNoteLineInput struct {
	ARInvoiceLineID           int64
	ReturnDeliveryOrderLineID int64
	ProductID                 int64
	Description               string
	Quantity                  float64
	UnitPrice                 float64
	DiscountPct               float64
	TaxPct                    float64
}

// CreateARCreditNoteFromReturnInput creates credit note from return delivery order.
type CreateARCreditNoteFromReturnInput struct {
	ReturnDeliveryOrderID int64
	Reason                string
	CreatedBy             int64
}

// PostARCreditNoteInput for posting a credit note.
type PostARCreditNoteInput struct {
	CreditNoteID int64
	PostedBy     int64
}

// VoidARCreditNoteInput for voiding a credit note.
type VoidARCreditNoteInput struct {
	CreditNoteID int64
	VoidedBy     int64
	VoidReason   string
}

// ListARCreditNotesRequest for filtering credit notes.
type ListARCreditNotesRequest struct {
	Status     ARCreditNoteStatus
	CustomerID int64
	InvoiceID  int64
	Limit      int
	Offset     int
}
