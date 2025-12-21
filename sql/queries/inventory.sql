-- name: GetStockCard :many
SELECT tx_code, tx_type, posted_at, qty_in, qty_out, balance_qty, unit_cost, balance_cost, note
FROM inventory_cards
WHERE warehouse_id = $1 
  AND product_id = $2 
  AND posted_at >= COALESCE(sqlc.narg('from_date')::timestamptz, '-infinity') 
  AND posted_at <= COALESCE(sqlc.narg('to_date')::timestamptz, 'infinity')
ORDER BY posted_at ASC, id ASC
LIMIT $3;

-- name: InsertTransaction :one
INSERT INTO inventory_tx (
    code, tx_type, warehouse_id, ref_module, ref_id, note, posted_at, created_by, created_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, NOW()
) RETURNING id;

-- name: InsertTransactionLine :exec
INSERT INTO inventory_tx_lines (
    tx_id, product_id, qty, unit_cost, src_warehouse_id, dst_warehouse_id
) VALUES (
    $1, $2, $3, $4, $5, $6
);

-- name: GetBalanceForUpdate :one
SELECT warehouse_id, product_id, qty, avg_cost, updated_at 
FROM inventory_balances 
WHERE warehouse_id = $1 AND product_id = $2 
FOR UPDATE;

-- name: UpsertBalance :exec
INSERT INTO inventory_balances (
    warehouse_id, product_id, qty, avg_cost, updated_at
) VALUES (
    $1, $2, $3, $4, NOW()
)
ON CONFLICT (warehouse_id, product_id) 
DO UPDATE SET 
    qty = EXCLUDED.qty, 
    avg_cost = EXCLUDED.avg_cost, 
    updated_at = NOW();

-- name: InsertCardEntry :exec
INSERT INTO inventory_cards (
    warehouse_id, product_id, tx_id, tx_code, tx_type, 
    qty_in, qty_out, balance_qty, unit_cost, balance_cost, 
    posted_at, note
) VALUES (
    $1, $2, $3, $4, $5, 
    $6, $7, $8, $9, $10, 
    $11, $12
);
