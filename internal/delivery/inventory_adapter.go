package delivery

import (
	"context"
	"fmt"

	"github.com/odyssey-erp/odyssey-erp/internal/inventory"
)

// InventoryAdapter adapts the inventory.Service to the InventoryService interface
// required by the delivery service.
type InventoryAdapter struct {
	service *inventory.Service
}

// NewInventoryAdapter creates a new inventory adapter
func NewInventoryAdapter(service *inventory.Service) *InventoryAdapter {
	return &InventoryAdapter{
		service: service,
	}
}

// PostAdjustment posts an inventory adjustment for delivery order fulfillment
func (a *InventoryAdapter) PostAdjustment(ctx context.Context, input InventoryAdjustmentInput) error {
	if a.service == nil {
		return fmt.Errorf("inventory service not initialized")
	}

	// Convert delivery adjustment input to inventory adjustment input
	invInput := inventory.AdjustmentInput{
		Code:        input.Code,
		WarehouseID: input.WarehouseID,
		ProductID:   input.ProductID,
		Qty:         input.Qty,
		UnitCost:    input.UnitCost,
		Note:        input.Note,
		ActorID:     input.ActorID,
		RefModule:   input.RefModule,
		RefID:       input.RefID,
	}

	// Post the adjustment
	_, err := a.service.PostAdjustment(ctx, invInput)
	if err != nil {
		return fmt.Errorf("post inventory adjustment: %w", err)
	}

	return nil
}
