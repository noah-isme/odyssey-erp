package orders

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/odyssey-erp/odyssey-erp/internal/delivery/orders/db"
)

// CreateDeliveryOrder creates a new delivery order.
func (t *txRepository) CreateDeliveryOrder(ctx context.Context, do DeliveryOrder) (int64, error) {
	return t.queries.CreateDeliveryOrder(ctx, ordersdb.CreateDeliveryOrderParams{
		DocNumber:      do.DocNumber,
		CompanyID:      do.CompanyID,
		SalesOrderID:   do.SalesOrderID,
		WarehouseID:    do.WarehouseID,
		CustomerID:     do.CustomerID,
		DeliveryDate:   pgtype.Date{Time: do.DeliveryDate, Valid: true},
		Status:         ordersdb.DeliveryOrderStatus(do.Status),
		DriverName:     pointerToText(do.DriverName),
		VehicleNumber:  pointerToText(do.VehicleNumber),
		TrackingNumber: pointerToText(do.TrackingNumber),
		Notes:          pointerToText(do.Notes),
		CreatedBy:      do.CreatedBy,
	})
}

// InsertLine inserts a delivery order line.
func (t *txRepository) InsertLine(ctx context.Context, line Line) (int64, error) {
	return t.queries.InsertLine(ctx, ordersdb.InsertLineParams{
		DeliveryOrderID:   line.DeliveryOrderID,
		SalesOrderLineID:  line.SalesOrderLineID,
		ProductID:         line.ProductID,
		QuantityToDeliver: floatToNumeric(line.QuantityToDeliver),
		QuantityDelivered: floatToNumeric(line.QuantityDelivered),
		Uom:               line.UOM,
		UnitPrice:         floatToNumeric(line.UnitPrice),
		Notes:             pointerToText(line.Notes),
		LineOrder:         int32(line.LineOrder),
	})
}

// UpdateDeliveryOrder updates delivery order fields.
// NOTE: Kept as dynamic update to support partial updates via map.
func (t *txRepository) UpdateDeliveryOrder(ctx context.Context, id int64, updates map[string]interface{}) error {
	if len(updates) == 0 {
		return nil
	}

	var setClauses []string
	var args []interface{}
	argPos := 1

	for field, value := range updates {
		setClauses = append(setClauses, fmt.Sprintf("%s = $%d", field, argPos))
		args = append(args, value)
		argPos++
	}

	setClauses = append(setClauses, fmt.Sprintf("updated_at = $%d", argPos))
	args = append(args, time.Now())
	argPos++

	args = append(args, id)

	query := fmt.Sprintf(`
		UPDATE delivery_orders
		SET %s
		WHERE id = $%d
	`, strings.Join(setClauses, ", "), argPos)

	cmdTag, err := t.tx.Exec(ctx, query, args...)
	if err != nil {
		return err
	}
	if cmdTag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdateStatus updates status with additional fields.
func (t *txRepository) UpdateStatus(ctx context.Context, id int64, status Status, updates map[string]interface{}) error {
	if updates == nil {
		updates = make(map[string]interface{})
	}
	updates["status"] = status
	return t.UpdateDeliveryOrder(ctx, id, updates)
}

// DeleteLines removes all lines for a delivery order.
func (t *txRepository) DeleteLines(ctx context.Context, deliveryOrderID int64) error {
	return t.queries.DeleteLines(ctx, deliveryOrderID)
}

// UpdateLineQuantity updates delivered quantity on a line.
func (t *txRepository) UpdateLineQuantity(ctx context.Context, lineID int64, quantityDelivered float64) error {
	return t.queries.UpdateLineQuantity(ctx, ordersdb.UpdateLineQuantityParams{
		QuantityDelivered: floatToNumeric(quantityDelivered),
		UpdatedAt:         pgtype.Timestamptz{Time: time.Now(), Valid: true},
		ID:                lineID,
	})
}
