-- name: InsertAdjustment :one
INSERT INTO inventory_adjustments (
    number, warehouse_id, status, note, adjustment_at, created_by, created_at
) VALUES (
    $1, $2, $3, $4, $5, $6, NOW()
) RETURNING id;

-- name: GetAdjustment :one
SELECT adj.*, w.name as warehouse_name, u.email as creator_email
FROM inventory_adjustments adj
JOIN warehouses w ON adj.warehouse_id = w.id
JOIN users u ON adj.created_by = u.id
WHERE adj.id = $1;

-- name: ListAdjustments :many
SELECT adj.*, w.name as warehouse_name
FROM inventory_adjustments adj
JOIN warehouses w ON adj.warehouse_id = w.id
ORDER BY adj.created_at DESC;

-- name: InsertAdjustmentLine :exec
INSERT INTO inventory_adjustment_lines (
    adjustment_id, product_id, qty, note
) VALUES (
    $1, $2, $3, $4
);

-- name: GetAdjustmentLines :many
SELECT adjl.*, p.name as product_name
FROM inventory_adjustment_lines adjl
JOIN products p ON adjl.product_id = p.id
WHERE adjl.adjustment_id = $1;

-- name: UpdateAdjustmentStatus :exec
UPDATE inventory_adjustments
SET status = $2, posted_by = $3, posted_at = $4
WHERE id = $1;
