package procurement

import (
	"context"
	"time"
)


// GRNLineEvent describes individual line values for integration mapping.
type GRNLineEvent struct {
	ProductID int64
	Qty       float64
	UnitCost  float64
}

// GRNPostedEvent captures details required to post a GRN to the ledger.
type GRNPostedEvent struct {
	ID          int64
	Number      string
	SupplierID  int64
	WarehouseID int64
	ReceivedAt  time.Time
	Lines       []GRNLineEvent
}

// APInvoicePostedEvent contains metadata for AP invoice postings.
type APInvoicePostedEvent struct {
	ID         int64
	Number     string
	SupplierID int64
	GRNID      int64
	Total      float64
	PostedAt   time.Time
}

// APPaymentPostedEvent describes AP payment details for integration.
type APPaymentPostedEvent struct {
	ID          int64
	Number      string
	APInvoiceID int64
	Amount      float64
	PaidAt      time.Time
}

// IntegrationHandler receives procurement domain events for ledger integration.
type IntegrationHandler interface {
	HandleGRNPosted(ctx context.Context, evt GRNPostedEvent) error
	HandleAPInvoicePosted(ctx context.Context, evt APInvoicePostedEvent) error
	HandleAPPaymentPosted(ctx context.Context, evt APPaymentPostedEvent) error
}
