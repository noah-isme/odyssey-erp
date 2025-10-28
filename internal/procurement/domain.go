package procurement

import (
	"errors"
	"time"
)

// Purchase request lifecycle statuses.
type PRStatus string

const (
	PRStatusDraft     PRStatus = "DRAFT"
	PRStatusSubmitted PRStatus = "SUBMITTED"
	PRStatusClosed    PRStatus = "CLOSED"
)

// Purchase order lifecycle statuses.
type POStatus string

const (
	POStatusDraft     POStatus = "DRAFT"
	POStatusApproval  POStatus = "APPROVAL"
	POStatusApproved  POStatus = "APPROVED"
	POStatusClosed    POStatus = "CLOSED"
	POStatusCancelled POStatus = "CANCELLED"
)

// Goods receipt statuses.
type GRNStatus string

const (
	GRNStatusDraft     GRNStatus = "DRAFT"
	GRNStatusPosted    GRNStatus = "POSTED"
	GRNStatusCancelled GRNStatus = "CANCELLED"
)

// AP invoice statuses.
type APInvoiceStatus string

const (
	APStatusDraft  APInvoiceStatus = "DRAFT"
	APStatusPosted APInvoiceStatus = "POSTED"
	APStatusPaid   APInvoiceStatus = "PAID"
	APStatusVoid   APInvoiceStatus = "VOID"
)

// PurchaseRequest domain model.
type PurchaseRequest struct {
	ID         int64
	Number     string
	SupplierID int64
	RequestBy  int64
	Status     PRStatus
	Note       string
}

// PRLine represents requested item.
type PRLine struct {
	ID        int64
	PRID      int64
	ProductID int64
	Qty       float64
	Note      string
}

// PurchaseOrder domain model.
type PurchaseOrder struct {
	ID           int64
	Number       string
	SupplierID   int64
	Status       POStatus
	Currency     string
	ExpectedDate time.Time
	Note         string
}

// POLine represents PO lines.
type POLine struct {
	ID        int64
	POID      int64
	ProductID int64
	Qty       float64
	Price     float64
	TaxID     int64
	Note      string
}

// GoodsReceipt domain model.
type GoodsReceipt struct {
	ID          int64
	Number      string
	POID        int64
	SupplierID  int64
	WarehouseID int64
	Status      GRNStatus
	ReceivedAt  time.Time
	Note        string
}

// GRNLine describes received goods.
type GRNLine struct {
	ID        int64
	GRNID     int64
	ProductID int64
	Qty       float64
	UnitCost  float64
}

// APInvoice model.
type APInvoice struct {
	ID         int64
	Number     string
	SupplierID int64
	GRNID      int64
	Currency   string
	Total      float64
	Status     APInvoiceStatus
	DueAt      time.Time
}

// APPayment model.
type APPayment struct {
	ID          int64
	Number      string
	APInvoiceID int64
	Amount      float64
}

var (
	// ErrInvalidState occurs when action violates status workflow.
	ErrInvalidState = errors.New("procurement: invalid state transition")
	// ErrNotFound indicates record missing.
	ErrNotFound = errors.New("procurement: not found")
	// ErrValidation indicates invalid input.
	ErrValidation = errors.New("procurement: invalid input")
)
