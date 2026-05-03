-- 000028_inventory_adjustments.up.sql

-- Stock Adjustment Header
CREATE TABLE IF NOT EXISTS inventory_adjustments (
    id BIGSERIAL PRIMARY KEY,
    uuid UUID NOT NULL DEFAULT gen_random_uuid(),
    number TEXT NOT NULL UNIQUE,
    warehouse_id INTEGER NOT NULL REFERENCES warehouses(id),
    status TEXT NOT NULL CHECK (status IN ('DRAFT','POSTED','CANCELLED')),
    note TEXT NOT NULL DEFAULT '',
    adjustment_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by BIGINT NOT NULL REFERENCES users(id),
    posted_by BIGINT REFERENCES users(id),
    posted_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Stock Adjustment Lines
CREATE TABLE IF NOT EXISTS inventory_adjustment_lines (
    id BIGSERIAL PRIMARY KEY,
    adjustment_id BIGINT NOT NULL REFERENCES inventory_adjustments(id) ON DELETE CASCADE,
    product_id INTEGER NOT NULL REFERENCES products(id),
    qty NUMERIC(14,4) NOT NULL DEFAULT 0, -- Positive for addition, negative for subtraction
    note TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_inventory_adjustment_warehouse ON inventory_adjustments(warehouse_id);
CREATE INDEX IF NOT EXISTS idx_inventory_adjustment_lines_header ON inventory_adjustment_lines(adjustment_id);
CREATE INDEX IF NOT EXISTS idx_inventory_adjustment_lines_product ON inventory_adjustment_lines(product_id);
