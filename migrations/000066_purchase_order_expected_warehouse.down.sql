DROP INDEX IF EXISTS idx_pos_expected_warehouse;
ALTER TABLE pos DROP COLUMN IF EXISTS expected_warehouse_id;
