package ar

import (
	"time"
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
	Subtotal        float64
	TaxAmount       float64
	Total           float64
	Status          ARInvoiceStatus
	DueAt           time.Time
	PostedAt        *time.Time
	PostedBy        *int64
	VoidedAt        *time.Time
	VoidedBy        *int64
	VoidReason      string
	CreatedBy       int64
	CreatedAt       time.Time
	UpdatedAt       time.Time
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

// ARInvoiceWithDetails includes invoice with lines and customer info.
type ARInvoiceWithDetails struct {
	ARInvoice
	CustomerName string
	Lines        []ARInvoiceLine
	Payments     []ARPaymentSummary
	PaidAmount   float64
	Balance      float64
}

// ARPayment model.
type ARPayment struct {
	ID          int64
	Number      string
	ARInvoiceID int64
	Amount      float64
	PaidAt      time.Time
	Method      string
	Note        string
	CreatedBy   int64
	CreatedAt   time.Time
	UpdatedAt   time.Time
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
	ID          int64
	ARPaymentID int64
	ARInvoiceID int64
	Amount      float64
	CreatedAt   time.Time
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
	Amount      float64
	PaidAt      time.Time
	Method      string
	Note        string
	CreatedBy   int64
	Allocations []PaymentAllocationInput
}

// PaymentAllocationInput for allocating payment to invoices.
type PaymentAllocationInput struct {
	ARInvoiceID int64
	Amount      float64
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

// Deprecated: ARInvoiceInput kept for backward compatibility.
type ARInvoiceInput struct {
	CustomerID int64
	SOID       int64
	Number     string
	Currency   string
	Total      float64
	DueDate    time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// Deprecated: ARPaymentInput kept for backward compatibility.
type ARPaymentInput struct {
	ARInvoiceID int64
	Number      string
	Amount      float64
	PaidAt      time.Time
	Method      string
	Note        string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}
