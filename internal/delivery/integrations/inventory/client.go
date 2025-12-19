// Package inventory provides integration with the inventory module.
package inventory

import (
	"context"
	"fmt"

	inv "github.com/odyssey-erp/odyssey-erp/internal/inventory"
)

// Item represents an item for inventory operations.
type Item struct {
	WarehouseID int64
	ProductID   int64
	Quantity    float64
	UnitCost    float64
	Code        string
	Note        string
	ActorID     int64
	RefModule   string
	RefID       string
}

// Client provides inventory operations for delivery.
type Client struct {
	service *inv.Service
}

// NewClient creates a new inventory client.
func NewClient(service *inv.Service) *Client {
	return &Client{service: service}
}

// Reduce reduces inventory stock (outbound).
func (c *Client) Reduce(ctx context.Context, items []Item) error {
	if c.service == nil {
		return fmt.Errorf("inventory service not initialized")
	}

	for _, item := range items {
		input := inv.AdjustmentInput{
			Code:        item.Code,
			WarehouseID: item.WarehouseID,
			ProductID:   item.ProductID,
			Qty:         -item.Quantity, // Negative for outbound
			UnitCost:    item.UnitCost,
			Note:        item.Note,
			ActorID:     item.ActorID,
			RefModule:   item.RefModule,
			RefID:       item.RefID,
		}
		if _, err := c.service.PostAdjustment(ctx, input); err != nil {
			return fmt.Errorf("reduce stock for product %d: %w", item.ProductID, err)
		}
	}

	return nil
}

// Reserve reserves inventory (for future use).
func (c *Client) Reserve(ctx context.Context, items []Item) error {
	// TODO: Implement reservation logic when needed
	return nil
}
