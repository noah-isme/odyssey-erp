-- 000027_inventory_enhancements.up.sql

-- Add min_stock to products for reorder point alerts
ALTER TABLE products ADD COLUMN IF NOT EXISTS min_stock NUMERIC(14,4) NOT NULL DEFAULT 0;

-- Stock Take Session Header
CREATE TABLE IF NOT EXISTS inventory_stock_takes (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    number TEXT NOT NULL UNIQUE,
    warehouse_id INTEGER NOT NULL REFERENCES warehouses(id),
    status TEXT NOT NULL CHECK (status IN ('DRAFT','POSTED','CANCELLED')),
    note TEXT NOT NULL DEFAULT '',
    taken_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by BIGINT NOT NULL REFERENCES users(id),
    posted_by BIGINT REFERENCES users(id),
    posted_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Stock Take Items (Counts)
CREATE TABLE IF NOT EXISTS inventory_stock_take_lines (
    id BIGSERIAL PRIMARY KEY,
    stock_take_id BIGINT NOT NULL REFERENCES inventory_stock_takes(id) ON DELETE CASCADE,
    product_id INTEGER NOT NULL REFERENCES products(id),
    system_qty NUMERIC(14,4) NOT NULL DEFAULT 0,
    physical_qty NUMERIC(14,4) NOT NULL DEFAULT 0,
    variance_qty NUMERIC(14,4) GENERATED ALWAYS AS (physical_qty - system_qty) STORED,
    note TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_inventory_stock_take_warehouse ON inventory_stock_takes(warehouse_id);
CREATE INDEX IF NOT EXISTS idx_inventory_stock_take_lines_header ON inventory_stock_take_lines(stock_take_id);
CREATE INDEX IF NOT EXISTS idx_inventory_stock_take_lines_product ON inventory_stock_take_lines(product_id);
