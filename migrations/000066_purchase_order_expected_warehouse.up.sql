-- Open purchase orders become usable as MRP supply only when their receiving
-- warehouse is explicit. Historical rows are backfilled when unambiguous.
ALTER TABLE pos
    ADD COLUMN IF NOT EXISTS expected_warehouse_id BIGINT REFERENCES warehouses(id) ON DELETE RESTRICT;

WITH single_company_warehouse AS (
    SELECT b.company_id, MIN(w.id) AS warehouse_id
    FROM warehouses w
    JOIN branches b ON b.id=w.branch_id
    GROUP BY b.company_id
    HAVING COUNT(*)=1
)
UPDATE pos
SET expected_warehouse_id=scoped.warehouse_id
FROM single_company_warehouse scoped
WHERE pos.company_id=scoped.company_id
  AND pos.expected_warehouse_id IS NULL;

CREATE INDEX IF NOT EXISTS idx_pos_expected_warehouse
    ON pos(expected_warehouse_id);
