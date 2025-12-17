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
	ID         int64
	Number     string
	CustomerID int64
	SOID       int64
	Currency   string
	Total      float64
	Status     ARInvoiceStatus
	DueAt      time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
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
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// ARInvoiceInput for creating AR invoices.
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

// ARPaymentInput for creating AR payments.
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

// ARAgingBucket summarises totals by aging periods.
type ARAgingBucket struct {
	Current   float64
	Bucket30  float64
	Bucket60  float64
	Bucket90  float64
	Bucket120 float64
}
