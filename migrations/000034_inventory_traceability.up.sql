ALTER TABLE products
    ADD COLUMN IF NOT EXISTS cost_method TEXT NOT NULL DEFAULT 'AVG'
        CHECK (cost_method IN ('AVG', 'FIFO')),
    ADD COLUMN IF NOT EXISTS reorder_target NUMERIC(14,4) NOT NULL DEFAULT 0
        CHECK (reorder_target >= 0),
    ADD COLUMN IF NOT EXISTS preferred_supplier_id BIGINT REFERENCES suppliers(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS track_batch BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS track_serial BOOLEAN NOT NULL DEFAULT FALSE,
    ADD CONSTRAINT products_traceability_mode_check CHECK (NOT (track_batch AND track_serial));

CREATE INDEX IF NOT EXISTS idx_products_preferred_supplier ON products(preferred_supplier_id);

CREATE TABLE IF NOT EXISTS inventory_lots (
    id BIGSERIAL PRIMARY KEY,
    product_id BIGINT NOT NULL REFERENCES products(id) ON DELETE RESTRICT,
    warehouse_id BIGINT NOT NULL REFERENCES warehouses(id) ON DELETE RESTRICT,
    lot_number TEXT NOT NULL,
    expiry_date DATE,
    received_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    qty_on_hand NUMERIC(14,4) NOT NULL DEFAULT 0 CHECK (qty_on_hand >= 0),
    unit_cost NUMERIC(14,4) NOT NULL DEFAULT 0 CHECK (unit_cost >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(product_id, warehouse_id, lot_number)
);

CREATE INDEX IF NOT EXISTS idx_inventory_lots_expiry ON inventory_lots(product_id, warehouse_id, expiry_date);

CREATE TABLE IF NOT EXISTS inventory_serials (
    id BIGSERIAL PRIMARY KEY,
    product_id BIGINT NOT NULL REFERENCES products(id) ON DELETE RESTRICT,
    warehouse_id BIGINT REFERENCES warehouses(id) ON DELETE SET NULL,
    lot_id BIGINT REFERENCES inventory_lots(id) ON DELETE SET NULL,
    serial_number TEXT NOT NULL UNIQUE,
    status TEXT NOT NULL DEFAULT 'IN_STOCK' CHECK (status IN ('IN_STOCK', 'RESERVED', 'SOLD', 'VOID')),
    received_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_inventory_serials_lookup ON inventory_serials(product_id, warehouse_id, status);

ALTER TABLE inventory_tx_lines
    ADD COLUMN IF NOT EXISTS lot_id BIGINT REFERENCES inventory_lots(id) ON DELETE RESTRICT,
    ADD COLUMN IF NOT EXISTS serial_id BIGINT REFERENCES inventory_serials(id) ON DELETE RESTRICT;

ALTER TABLE grn_lines
    ADD COLUMN IF NOT EXISTS lot_number TEXT,
    ADD COLUMN IF NOT EXISTS expiry_date DATE,
    ADD COLUMN IF NOT EXISTS serial_numbers TEXT[] NOT NULL DEFAULT '{}';
