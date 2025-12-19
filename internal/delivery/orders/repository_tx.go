package orders

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// CreateDeliveryOrder creates a new delivery order.
func (t *txRepository) CreateDeliveryOrder(ctx context.Context, do DeliveryOrder) (int64, error) {
	query := `
		INSERT INTO delivery_orders (
			doc_number, company_id, sales_order_id, warehouse_id, customer_id,
			delivery_date, status, driver_name, vehicle_number, tracking_number,
			notes, created_by
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING id
	`
	var id int64
	err := t.tx.QueryRow(ctx, query,
		do.DocNumber, do.CompanyID, do.SalesOrderID, do.WarehouseID, do.CustomerID,
		do.DeliveryDate, do.Status, do.DriverName, do.VehicleNumber, do.TrackingNumber,
		do.Notes, do.CreatedBy,
	).Scan(&id)
	return id, err
}

// InsertLine inserts a delivery order line.
func (t *txRepository) InsertLine(ctx context.Context, line Line) (int64, error) {
	query := `
		INSERT INTO delivery_order_lines (
			delivery_order_id, sales_order_line_id, product_id,
			quantity_to_deliver, quantity_delivered, uom, unit_price,
			notes, line_order
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id
	`
	var id int64
	err := t.tx.QueryRow(ctx, query,
		line.DeliveryOrderID, line.SalesOrderLineID, line.ProductID,
		line.QuantityToDeliver, line.QuantityDelivered, line.UOM, line.UnitPrice,
		line.Notes, line.LineOrder,
	).Scan(&id)
	return id, err
}

// UpdateDeliveryOrder updates delivery order fields.
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
	query := `DELETE FROM delivery_order_lines WHERE delivery_order_id = $1`
	_, err := t.tx.Exec(ctx, query, deliveryOrderID)
	return err
}

// UpdateLineQuantity updates delivered quantity on a line.
func (t *txRepository) UpdateLineQuantity(ctx context.Context, lineID int64, quantityDelivered float64) error {
	query := `
		UPDATE delivery_order_lines
		SET quantity_delivered = $1, updated_at = $2
		WHERE id = $3
	`
	cmdTag, err := t.tx.Exec(ctx, query, quantityDelivered, time.Now(), lineID)
	if err != nil {
		return err
	}
	if cmdTag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
