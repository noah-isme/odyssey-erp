DROP INDEX IF EXISTS idx_sales_order_lines_fulfillment_warehouse;
ALTER TABLE sales_order_lines DROP COLUMN IF EXISTS fulfillment_warehouse_id;
