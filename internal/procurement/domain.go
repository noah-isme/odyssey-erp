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
	CompanyID  int64
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
	ID                  int64
	CompanyID           int64
	Number              string
	SupplierID          int64
	ExpectedWarehouseID int64
	Status              POStatus
	Currency            string
	ExpectedDate        time.Time
	Note                string
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
	ID            int64
	GRNID         int64
	ProductID     int64
	Qty           float64
	UnitCost      float64
	LotNumber     string
	ExpiryDate    *time.Time
	SerialNumbers []string
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

// POListItem is a DTO for listing purchase orders with joined data.
type POListItem struct {
	ID           int64
	Number       string
	SupplierID   int64
	SupplierName string
	Status       POStatus
	Currency     string
	ExpectedDate time.Time
	CreatedAt    time.Time
	Total        float64
}

// GRNListItem is a DTO for listing goods receipts with joined data.
type GRNListItem struct {
	ID            int64
	Number        string
	POID          int64
	PONumber      string
	SupplierID    int64
	SupplierName  string
	WarehouseID   int64
	WarehouseName string
	Status        GRNStatus
	ReceivedAt    time.Time
	CreatedAt     time.Time
}

// ListFilters contains filter parameters for list queries.
type ListFilters struct {
	Status     string
	SupplierID int64
	Search     string
	SortBy     string
	SortDir    string // "asc" or "desc"
}

// GoodsReturnGRNStatus enumerates goods return statuses.
type GoodsReturnGRNStatus string

const (
	GoodsReturnStatusDraft     GoodsReturnGRNStatus = "DRAFT"
	GoodsReturnStatusConfirmed GoodsReturnGRNStatus = "CONFIRMED"
	GoodsReturnStatusCancelled GoodsReturnGRNStatus = "CANCELLED"
)

// GoodsReturnGRN domain model for supplier returns from a posted GRN.
type GoodsReturnGRN struct {
	ID          int64
	Number      string
	CompanyID   int64
	SupplierID  int64
	GRNID       int64
	WarehouseID int64
	ReturnDate  time.Time
	Status      GoodsReturnGRNStatus
	Reason      string
	Notes       *string
	CreatedBy   int64
	ConfirmedBy *int64
	ConfirmedAt *time.Time
	VoidedBy    *int64
	VoidedAt    *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Lines       []GoodsReturnGRNLine

	SupplierName  string
	WarehouseName string
	GRNNumber     string
	CreatedByName string
}

// GoodsReturnGRNLine represents a returned item line.
type GoodsReturnGRNLine struct {
	ID               int64
	GoodsReturnGRNID int64
	GRNLineID        int64
	ProductID        int64
	QuantityReturned float64
	UnitCost         float64
	Notes            *string
	LineOrder        int
	CreatedAt        time.Time
	UpdatedAt        time.Time

	ProductCode    string
	ProductName    string
	OriginalGRNQty float64
}

// CreateGoodsReturnGRNInput for creating a goods return from a GRN.
type CreateGoodsReturnGRNInput struct {
	GRNID       int64
	CompanyID   int64
	SupplierID  int64
	WarehouseID int64
	ReturnDate  time.Time
	Reason      string
	Notes       *string
	CreatedBy   int64
	Lines       []GoodsReturnGRNLineInput
}

// GoodsReturnGRNLineInput for a return line.
type GoodsReturnGRNLineInput struct {
	GRNLineID        int64
	ProductID        int64
	QuantityReturned float64
	UnitCost         float64
	Notes            *string
	LineOrder        int
}

var (
	// ErrInvalidState occurs when action violates status workflow.
	ErrInvalidState = errors.New("procurement: invalid state transition")
	// ErrNotFound indicates record missing.
	ErrNotFound = errors.New("procurement: not found")
	// ErrValidation indicates invalid input.
	ErrValidation = errors.New("procurement: invalid input")
	// ErrGoodsReturnNotFound indicates a goods return record was not found.
	ErrGoodsReturnNotFound = errors.New("procurement: goods return not found")
	// ErrGoodsReturnAlreadyConfirmed indicates the return is already confirmed.
	ErrGoodsReturnAlreadyConfirmed = errors.New("procurement: goods return already confirmed")
)
