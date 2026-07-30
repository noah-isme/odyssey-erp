package orders

import (
	"context"
	"errors"
	"fmt"

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

// Restock implements the InventoryClient interface.
func (a *InventoryAdapter) Restock(ctx context.Context, items []InventoryItem) error {
	processed := make([]InventoryItem, 0, len(items))
	for _, item := range items {
		input := inventory.AdjustmentInput{
			WarehouseID: item.WarehouseID,
			ProductID:   item.ProductID,
			Qty:         item.Quantity,
			UnitCost:    item.UnitCost,
			Code:        item.Code,
			Note:        item.Note,
			ActorID:     item.ActorID,
			RefModule:   item.RefModule,
			RefID:       item.RefID,
		}
		if _, err := a.svc.PostAdjustment(ctx, input); err != nil {
			var compensationErr error
			for i := len(processed) - 1; i >= 0; i-- {
				completed := processed[i]
				_, rollbackErr := a.svc.PostAdjustment(ctx, inventory.AdjustmentInput{
					WarehouseID: completed.WarehouseID,
					ProductID:   completed.ProductID,
					Qty:         -completed.Quantity,
					UnitCost:    completed.UnitCost,
					Code:        completed.Code + "-COMP",
					Note:        "Compensate failed return restock",
					ActorID:     completed.ActorID,
					RefModule:   completed.RefModule,
					RefID:       completed.RefID,
				})
				compensationErr = errors.Join(compensationErr, rollbackErr)
			}
			return errors.Join(fmt.Errorf("restock product %d: %w", item.ProductID, err), compensationErr)
		}
		processed = append(processed, item)
	}
	return nil
}

// CheckAvailability implements the InventoryClient interface.
func (a *InventoryAdapter) CheckAvailability(ctx context.Context, warehouseID, productID int64) (float64, error) {
	return a.svc.CheckAvailability(ctx, warehouseID, productID)
}
