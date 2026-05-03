-- name: InsertStockTake :one
INSERT INTO inventory_stock_takes (
    number, warehouse_id, status, note, taken_at, created_by, created_at
) VALUES (
    $1, $2, $3, $4, $5, $6, NOW()
) RETURNING id;

-- name: GetStockTake :one
SELECT st.*, w.name as warehouse_name, u.email as creator_email
FROM inventory_stock_takes st
JOIN warehouses w ON st.warehouse_id = w.id
JOIN users u ON st.created_by = u.id
WHERE st.id = $1;

-- name: ListStockTakes :many
SELECT st.*, w.name as warehouse_name
FROM inventory_stock_takes st
JOIN warehouses w ON st.warehouse_id = w.id
ORDER BY st.created_at DESC;

-- name: InsertStockTakeLine :exec
INSERT INTO inventory_stock_take_lines (
    stock_take_id, product_id, system_qty, physical_qty, note
) VALUES (
    $1, $2, $3, $4, $5
);

-- name: GetStockTakeLines :many
SELECT stl.*, p.name as product_name
FROM inventory_stock_take_lines stl
JOIN products p ON stl.product_id = p.id
WHERE stl.stock_take_id = $1;

-- name: UpdateStockTakeStatus :exec
UPDATE inventory_stock_takes
SET status = $2, posted_by = $3, posted_at = $4
WHERE id = $1;

-- name: GetStockValuation :many
SELECT b.warehouse_id, w.name as warehouse_name, b.product_id, p.name as product_name, p.sku, b.qty, b.avg_cost, (b.qty * b.avg_cost)::numeric(14,2) as total_value
FROM inventory_balances b
JOIN warehouses w ON b.warehouse_id = w.id
JOIN products p ON b.product_id = p.id
WHERE (b.warehouse_id = $1 OR $1 = 0)
ORDER BY w.name, p.name;

-- name: GetReorderAlerts :many
SELECT p.id, p.name, p.sku, p.min_stock, b.warehouse_id, w.name as warehouse_name, b.qty
FROM products p
JOIN inventory_balances b ON p.id = b.product_id
JOIN warehouses w ON b.warehouse_id = w.id
WHERE b.qty < p.min_stock AND p.is_active = true;
