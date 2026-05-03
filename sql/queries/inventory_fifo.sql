-- name: GetInboundHistory :many
SELECT 
    tl.qty, 
    tl.unit_cost, 
    t.posted_at
FROM inventory_tx_lines tl
JOIN inventory_tx t ON tl.tx_id = t.id
WHERE tl.product_id = $1 
  AND tl.dst_warehouse_id = $2
  AND tl.qty > 0
ORDER BY t.posted_at DESC;
