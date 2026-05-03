-- 000027_inventory_enhancements.down.sql

DROP TABLE IF EXISTS inventory_stock_take_lines;
DROP TABLE IF EXISTS inventory_stock_takes;
ALTER TABLE products DROP COLUMN IF EXISTS min_stock;
