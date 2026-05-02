package orders

import (
	"context"

	"github.com/odyssey-erp/odyssey-erp/internal/inventory"
)

// InventoryAdapter adapts inventory.Service to the InventoryClient interface.
type InventoryAdapter struct {
	svc *inventory.Service
}

// NewInventoryAdapter creates a new adapter.
func NewInventoryAdapter(svc *inventory.Service) *InventoryAdapter {
	return &InventoryAdapter{svc: svc}
}

// Reduce implements the InventoryClient interface.
func (a *InventoryAdapter) Reduce(ctx context.Context, items []InventoryItem) error {
	for _, item := range items {
		input := inventory.AdjustmentInput{
			WarehouseID: item.WarehouseID,
			ProductID:   item.ProductID,
			Qty:         -item.Quantity, // Negative for reduction
			UnitCost:    item.UnitCost,
			Code:        item.Code,
			Note:        item.Note,
			ActorID:     item.ActorID,
			RefModule:   item.RefModule,
			RefID:       item.RefID,
		}
		if _, err := a.svc.PostAdjustment(ctx, input); err != nil {
			return err
		}
	}
	return nil
}
