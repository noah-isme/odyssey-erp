ALTER TABLE inventory_tx_lines
    DROP COLUMN IF EXISTS serial_id,
    DROP COLUMN IF EXISTS lot_id;

ALTER TABLE grn_lines
    DROP COLUMN IF EXISTS serial_numbers,
    DROP COLUMN IF EXISTS expiry_date,
    DROP COLUMN IF EXISTS lot_number;

DROP TABLE IF EXISTS inventory_serials;
DROP TABLE IF EXISTS inventory_lots;

ALTER TABLE products
    DROP CONSTRAINT IF EXISTS products_traceability_mode_check,
    DROP COLUMN IF EXISTS track_serial,
    DROP COLUMN IF EXISTS track_batch,
    DROP COLUMN IF EXISTS preferred_supplier_id,
    DROP COLUMN IF EXISTS reorder_target,
    DROP COLUMN IF EXISTS cost_method;
