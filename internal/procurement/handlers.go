package procurement

import "context"

// IntegrationHandler receives procurement domain events for ledger integration.
type IntegrationHandler interface {
	HandleGRNPosted(ctx context.Context, evt GRNPostedEvent) error
	HandleAPInvoicePosted(ctx context.Context, evt APInvoicePostedEvent) error
	HandleAPPaymentPosted(ctx context.Context, evt APPaymentPostedEvent) error
}
