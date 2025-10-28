package inventory

import "context"

// IntegrationHandler receives inventory events for financial integration.
type IntegrationHandler interface {
	HandleInventoryAdjustmentPosted(ctx context.Context, evt AdjustmentPostedEvent) error
}
