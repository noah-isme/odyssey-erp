package orders

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/odyssey-erp/odyssey-erp/internal/ar"
)

// InvoicingAdapter loads delivery data for AR invoicing.
type InvoicingAdapter struct {
	pool *pgxpool.Pool
}

func NewInvoicingAdapter(pool *pgxpool.Pool) *InvoicingAdapter {
	return &InvoicingAdapter{pool: pool}
}

func (a *InvoicingAdapter) GetDeliveryOrderForInvoicing(ctx context.Context, id int64) (*ar.DeliveryOrderInfo, error) {
	const headerSQL = `
		SELECT d.id, d.doc_number, d.customer_id, c.name, d.sales_order_id,
		       d.warehouse_id, d.status, so.currency
		FROM delivery_orders d
		INNER JOIN customers c ON c.id = d.customer_id
		INNER JOIN sales_orders so ON so.id = d.sales_order_id
		WHERE d.id = $1
	`

	var status string
	info := ar.DeliveryOrderInfo{}
	if err := a.pool.QueryRow(ctx, headerSQL, id).Scan(
		&info.ID,
		&info.DocNumber,
		&info.CustomerID,
		&info.CustomerName,
		&info.SalesOrderID,
		&info.WarehouseID,
		&status,
		&info.Currency,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errors.New("delivery order not found")
		}
		return nil, err
	}

	if status != string(StatusDelivered) {
		return nil, fmt.Errorf("delivery order must be DELIVERED, got: %s", status)
	}

	const lineSQL = `
		SELECT dol.id, dol.product_id, p.name, dol.quantity_delivered, dol.unit_price,
		       sol.discount_percent, sol.tax_percent
		FROM delivery_order_lines dol
		INNER JOIN products p ON p.id = dol.product_id
		INNER JOIN sales_order_lines sol ON sol.id = dol.sales_order_line_id
		WHERE dol.delivery_order_id = $1
		ORDER BY dol.line_order, dol.id
	`

	rows, err := a.pool.Query(ctx, lineSQL, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			line                     ar.DeliveryLineInfo
			qty, unitPrice           pgtype.Numeric
			discountPercent, taxPercent pgtype.Numeric
		)
		if err := rows.Scan(
			&line.ID,
			&line.ProductID,
			&line.ProductName,
			&qty,
			&unitPrice,
			&discountPercent,
			&taxPercent,
		); err != nil {
			return nil, err
		}
		line.Quantity = numericToFloat(qty)
		line.UnitPrice = numericToFloat(unitPrice)
		line.DiscountPct = numericToFloat(discountPercent)
		line.TaxPct = numericToFloat(taxPercent)
		info.Lines = append(info.Lines, line)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(info.Lines) == 0 {
		return nil, errors.New("delivery order has no lines")
	}

	return &info, nil
}
