-- name: GetByID :one
SELECT id, doc_number, company_id, sales_order_id, warehouse_id, customer_id,
       delivery_date, status, driver_name, vehicle_number, tracking_number,
       notes, created_by, confirmed_by, confirmed_at, delivered_at,
       created_at, updated_at
FROM delivery_orders
WHERE id = $1;

-- name: GetByDocNumber :one
SELECT id, doc_number, company_id, sales_order_id, warehouse_id, customer_id,
       delivery_date, status, driver_name, vehicle_number, tracking_number,
       notes, created_by, confirmed_by, confirmed_at, delivered_at,
       created_at, updated_at
FROM delivery_orders
WHERE company_id = $1 AND doc_number = $2;

-- name: GetLines :many
SELECT id, delivery_order_id, sales_order_line_id, product_id,
       quantity_to_deliver, quantity_delivered, uom, unit_price,
       notes, line_order, created_at, updated_at
FROM delivery_order_lines
WHERE delivery_order_id = $1
ORDER BY line_order, id;

-- name: GetWithDetails :one
SELECT dor.id, dor.doc_number, dor.company_id, dor.sales_order_id, dor.warehouse_id,
       dor.customer_id, dor.delivery_date, dor.status, dor.driver_name,
       dor.vehicle_number, dor.tracking_number, dor.notes, dor.created_by,
       dor.confirmed_by, dor.confirmed_at, dor.delivered_at,
       dor.created_at, dor.updated_at,
       so.doc_number AS sales_order_number,
       w.name AS warehouse_name,
       c.name AS customer_name,
       u_created.email AS created_by_name,
       u_confirmed.email AS confirmed_by_name,
       COUNT(dol.id) AS line_count,
       COALESCE(SUM(dol.quantity_to_deliver), 0)::NUMERIC AS total_quantity
FROM delivery_orders dor
INNER JOIN sales_orders so ON so.id = dor.sales_order_id
INNER JOIN warehouses w ON w.id = dor.warehouse_id
INNER JOIN customers c ON c.id = dor.customer_id
INNER JOIN users u_created ON u_created.id = dor.created_by
LEFT JOIN users u_confirmed ON u_confirmed.id = dor.confirmed_by
LEFT JOIN delivery_order_lines dol ON dol.delivery_order_id = dor.id
WHERE dor.id = $1
GROUP BY dor.id, dor.doc_number, dor.company_id, dor.sales_order_id, dor.warehouse_id,
            dor.customer_id, dor.delivery_date, dor.status, dor.driver_name,
            dor.vehicle_number, dor.tracking_number, dor.notes, dor.created_by,
            dor.confirmed_by, dor.confirmed_at, dor.delivered_at,
            dor.created_at, dor.updated_at, so.doc_number, w.name, c.name,
            u_created.email, u_confirmed.email;

-- name: GetLinesWithDetails :many
SELECT dol.id, dol.delivery_order_id, dol.sales_order_line_id, dol.product_id,
       dol.quantity_to_deliver, dol.quantity_delivered, dol.uom, dol.unit_price,
       dol.notes, dol.line_order, dol.created_at, dol.updated_at,
       p.sku AS product_code,
       p.name AS product_name,
       sol.quantity AS so_line_quantity,
       sol.quantity_delivered AS so_line_delivered,
       (sol.quantity - sol.quantity_delivered)::NUMERIC AS remaining_to_deliver
FROM delivery_order_lines dol
INNER JOIN products p ON p.id = dol.product_id
INNER JOIN sales_order_lines sol ON sol.id = dol.sales_order_line_id
WHERE dol.delivery_order_id = $1
ORDER BY dol.line_order, dol.id;

-- name: CreateDeliveryOrder :one
INSERT INTO delivery_orders (
    doc_number, company_id, sales_order_id, warehouse_id, customer_id,
    delivery_date, status, driver_name, vehicle_number, tracking_number,
    notes, created_by
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12
)
RETURNING id;

-- name: InsertLine :one
INSERT INTO delivery_order_lines (
    delivery_order_id, sales_order_line_id, product_id,
    quantity_to_deliver, quantity_delivered, uom, unit_price,
    notes, line_order
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9
)
RETURNING id;

-- name: UpdateStatus :exec
UPDATE delivery_orders
SET status = $2, updated_at = NOW()
WHERE id = $1;

-- name: UpdateStatusConfirmed :exec
UPDATE delivery_orders
SET status = $2, confirmed_by = $3, confirmed_at = NOW(), updated_at = NOW()
WHERE id = $1;

-- name: DeleteLines :exec
DELETE FROM delivery_order_lines WHERE delivery_order_id = $1;

-- name: UpdateLineQuantity :exec
UPDATE delivery_order_lines
SET quantity_delivered = $1, updated_at = $2
WHERE id = $3;

-- name: GetDeliverableSOLines :many
SELECT sol.id AS sales_order_line_id,
       sol.sales_order_id,
       sol.product_id,
       p.sku AS product_code,
       p.name AS product_name,
       sol.quantity,
       sol.quantity_delivered,
       (sol.quantity - sol.quantity_delivered)::NUMERIC AS remaining_quantity,
       sol.uom,
       sol.unit_price,
       sol.line_order
FROM sales_order_lines sol
INNER JOIN products p ON p.id = sol.product_id
WHERE sol.sales_order_id = $1
  AND sol.quantity > sol.quantity_delivered
ORDER BY sol.line_order, sol.id;

-- name: GenerateDocNumber :one
SELECT generate_delivery_order_number($1, $2);

-- name: GetSalesOrderDetails :one
SELECT id, doc_number, company_id, customer_id, status
FROM sales_orders
WHERE id = $1;

-- name: CheckWarehouseExists :one
SELECT EXISTS(SELECT 1 FROM warehouses WHERE id = $1);
