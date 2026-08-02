-- Sales demand must be attributed to a warehouse before it can be planned.
-- The column remains nullable for historical orders whose source warehouse is
-- unknown; application writes require it for every new or replaced line.
ALTER TABLE sales_order_lines
    ADD COLUMN IF NOT EXISTS fulfillment_warehouse_id BIGINT REFERENCES warehouses(id) ON DELETE RESTRICT;

-- Safely backfill only companies that have exactly one warehouse. Multi-site
-- historical orders remain unassigned for an explicit operational decision.
WITH single_company_warehouse AS (
    SELECT b.company_id, MIN(w.id) AS warehouse_id
    FROM warehouses w
    JOIN branches b ON b.id = w.branch_id
    GROUP BY b.company_id
    HAVING COUNT(*) = 1
)
UPDATE sales_order_lines line
SET fulfillment_warehouse_id = scoped.warehouse_id
FROM sales_orders so
JOIN single_company_warehouse scoped ON scoped.company_id = so.company_id
WHERE line.sales_order_id = so.id
  AND line.fulfillment_warehouse_id IS NULL;

CREATE INDEX IF NOT EXISTS idx_sales_order_lines_fulfillment_warehouse
    ON sales_order_lines(fulfillment_warehouse_id);
