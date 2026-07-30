package ap

import "time"

type APDebitNoteStatus string

const (
	APDebitNoteStatusDraft  APDebitNoteStatus = "DRAFT"
	APDebitNoteStatusPosted APDebitNoteStatus = "POSTED"
	APDebitNoteStatusVoid   APDebitNoteStatus = "VOID"
)

type APDebitNote struct {
	ID               int64
	Number           string
	SupplierID       int64
	SupplierName     string
	APInvoiceID      int64
	InvoiceNumber    string
	GoodsReturnGRNID *int64
	Currency         string
	Reason           string
	Subtotal         float64
	TaxAmount        float64
	Total            float64
	Status           APDebitNoteStatus
	PostedAt         *time.Time
	PostedBy         *int64
	VoidedAt         *time.Time
	VoidedBy         *int64
	VoidReason       *string
	CreatedBy        int64
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type APDebitNoteLine struct {
	ID                   int64
	APDebitNoteID        int64
	APInvoiceLineID      *int64
	GoodsReturnGRNLineID *int64
	ProductID            int64
	Description          string
	Quantity             float64
	UnitPrice            float64
	DiscountPct          float64
	TaxPct               float64
	Subtotal             float64
	TaxAmount            float64
	Total                float64
	CreatedAt            time.Time
}

type APDebitNoteWithDetails struct {
	APDebitNote
	Lines []APDebitNoteLine
}

type CreateAPDebitNoteInput struct {
	SupplierID       int64
	APInvoiceID      int64
	GoodsReturnGRNID *int64
	Number           string
	Currency         string
	Reason           string
	CreatedBy        int64
	Lines            []CreateAPDebitNoteLineInput
}

type CreateAPDebitNoteLineInput struct {
	APInvoiceLineID      *int64
	GoodsReturnGRNLineID *int64
	ProductID            int64
	Description          string
	Quantity             float64
	UnitPrice            float64
	DiscountPct          float64
	TaxPct               float64
}

type CreateAPDebitNoteFromReturnInput struct {
	GoodsReturnGRNID int64
	Reason           string
	CreatedBy        int64
}

type PostAPDebitNoteInput struct {
	DebitNoteID int64
	PostedBy    int64
}

type VoidAPDebitNoteInput struct {
	DebitNoteID int64
	VoidedBy    int64
	VoidReason  string
}

type ListAPDebitNotesRequest struct {
	Status     APDebitNoteStatus
	SupplierID int64
	InvoiceID  int64
	Limit      int
	Offset     int
}
