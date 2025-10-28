-- Phase 3 schema for Inventory, Procurement, AP, and Controls

-- Inventory transactions
CREATE TABLE IF NOT EXISTS inventory_tx (
    id BIGSERIAL PRIMARY KEY,
    code TEXT NOT NULL UNIQUE,
    tx_type TEXT NOT NULL CHECK (tx_type IN ('IN','OUT','TRANSFER','ADJUST')),
    warehouse_id INTEGER NULL REFERENCES warehouses(id) ON DELETE SET NULL,
    ref_module TEXT NOT NULL DEFAULT '',
    ref_id UUID NULL,
    note TEXT NOT NULL DEFAULT '',
    posted_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by BIGINT NULL REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS inventory_tx_lines (
    id BIGSERIAL PRIMARY KEY,
    tx_id BIGINT NOT NULL REFERENCES inventory_tx(id) ON DELETE CASCADE,
    product_id INTEGER NOT NULL REFERENCES products(id) ON DELETE RESTRICT,
    qty NUMERIC(14,4) NOT NULL,
    unit_cost NUMERIC(14,4) NULL,
    amount NUMERIC(14,2) GENERATED ALWAYS AS (qty * COALESCE(unit_cost, 0)) STORED,
    src_warehouse_id INTEGER NULL REFERENCES warehouses(id) ON DELETE SET NULL,
    dst_warehouse_id INTEGER NULL REFERENCES warehouses(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_inventory_tx_lines_tx ON inventory_tx_lines(tx_id);
CREATE INDEX IF NOT EXISTS idx_inventory_tx_lines_product ON inventory_tx_lines(product_id);
CREATE INDEX IF NOT EXISTS idx_inventory_tx_lines_src_dst ON inventory_tx_lines(src_warehouse_id, dst_warehouse_id);

CREATE TABLE IF NOT EXISTS inventory_balances (
    warehouse_id INTEGER NOT NULL REFERENCES warehouses(id) ON DELETE CASCADE,
    product_id INTEGER NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    qty NUMERIC(14,4) NOT NULL DEFAULT 0,
    avg_cost NUMERIC(14,4) NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (warehouse_id, product_id)
);

CREATE TABLE IF NOT EXISTS inventory_cards (
    id BIGSERIAL PRIMARY KEY,
    warehouse_id INTEGER NOT NULL REFERENCES warehouses(id) ON DELETE CASCADE,
    product_id INTEGER NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    tx_id BIGINT NOT NULL REFERENCES inventory_tx(id) ON DELETE CASCADE,
    tx_code TEXT NOT NULL,
    tx_type TEXT NOT NULL,
    qty_in NUMERIC(14,4) NOT NULL DEFAULT 0,
    qty_out NUMERIC(14,4) NOT NULL DEFAULT 0,
    balance_qty NUMERIC(14,4) NOT NULL DEFAULT 0,
    unit_cost NUMERIC(14,4) NOT NULL DEFAULT 0,
    balance_cost NUMERIC(14,4) NOT NULL DEFAULT 0,
    posted_at TIMESTAMPTZ NOT NULL,
    note TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_inventory_cards_lookup ON inventory_cards(warehouse_id, product_id, posted_at);

-- Procurement tables
CREATE TABLE IF NOT EXISTS prs (
    id BIGSERIAL PRIMARY KEY,
    number TEXT NOT NULL UNIQUE,
    supplier_id INTEGER NULL REFERENCES suppliers(id) ON DELETE SET NULL,
    request_by BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    status TEXT NOT NULL CHECK (status IN ('DRAFT','SUBMITTED','CLOSED')),
    note TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS pr_lines (
    id BIGSERIAL PRIMARY KEY,
    pr_id BIGINT NOT NULL REFERENCES prs(id) ON DELETE CASCADE,
    product_id INTEGER NOT NULL REFERENCES products(id) ON DELETE RESTRICT,
    qty NUMERIC(14,4) NOT NULL CHECK (qty > 0),
    note TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_pr_lines_pr ON pr_lines(pr_id);

CREATE TABLE IF NOT EXISTS pos (
    id BIGSERIAL PRIMARY KEY,
    number TEXT NOT NULL UNIQUE,
    supplier_id INTEGER NOT NULL REFERENCES suppliers(id) ON DELETE RESTRICT,
    status TEXT NOT NULL CHECK (status IN ('DRAFT','APPROVAL','APPROVED','CLOSED','CANCELLED')),
    currency TEXT NOT NULL DEFAULT 'IDR',
    expected_date DATE NULL,
    note TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    approved_by BIGINT NULL REFERENCES users(id) ON DELETE SET NULL,
    approved_at TIMESTAMPTZ NULL
);

CREATE TABLE IF NOT EXISTS po_lines (
    id BIGSERIAL PRIMARY KEY,
    po_id BIGINT NOT NULL REFERENCES pos(id) ON DELETE CASCADE,
    product_id INTEGER NOT NULL REFERENCES products(id) ON DELETE RESTRICT,
    qty NUMERIC(14,4) NOT NULL CHECK (qty > 0),
    price NUMERIC(14,4) NOT NULL CHECK (price >= 0),
    tax_id INTEGER NULL REFERENCES taxes(id) ON DELETE SET NULL,
    note TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_po_lines_po ON po_lines(po_id);

CREATE TABLE IF NOT EXISTS grns (
    id BIGSERIAL PRIMARY KEY,
    number TEXT NOT NULL UNIQUE,
    po_id BIGINT NULL REFERENCES pos(id) ON DELETE SET NULL,
    supplier_id INTEGER NOT NULL REFERENCES suppliers(id) ON DELETE RESTRICT,
    warehouse_id INTEGER NOT NULL REFERENCES warehouses(id) ON DELETE RESTRICT,
    status TEXT NOT NULL CHECK (status IN ('DRAFT','POSTED','CANCELLED')),
    received_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    note TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS grn_lines (
    id BIGSERIAL PRIMARY KEY,
    grn_id BIGINT NOT NULL REFERENCES grns(id) ON DELETE CASCADE,
    product_id INTEGER NOT NULL REFERENCES products(id) ON DELETE RESTRICT,
    qty NUMERIC(14,4) NOT NULL CHECK (qty > 0),
    unit_cost NUMERIC(14,4) NOT NULL CHECK (unit_cost >= 0)
);
CREATE INDEX IF NOT EXISTS idx_grn_lines_grn ON grn_lines(grn_id);

CREATE TABLE IF NOT EXISTS ap_invoices (
    id BIGSERIAL PRIMARY KEY,
    number TEXT NOT NULL UNIQUE,
    supplier_id INTEGER NOT NULL REFERENCES suppliers(id) ON DELETE RESTRICT,
    grn_id BIGINT NULL REFERENCES grns(id) ON DELETE SET NULL,
    currency TEXT NOT NULL DEFAULT 'IDR',
    total NUMERIC(14,2) NOT NULL CHECK (total >= 0),
    status TEXT NOT NULL CHECK (status IN ('DRAFT','POSTED','PAID','VOID')),
    issued_at DATE NOT NULL DEFAULT CURRENT_DATE,
    due_at DATE NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS ap_payments (
    id BIGSERIAL PRIMARY KEY,
    number TEXT NOT NULL UNIQUE,
    ap_invoice_id BIGINT NOT NULL REFERENCES ap_invoices(id) ON DELETE CASCADE,
    amount NUMERIC(14,2) NOT NULL CHECK (amount > 0),
    paid_at DATE NOT NULL DEFAULT CURRENT_DATE,
    method TEXT NOT NULL DEFAULT 'TRANSFER',
    note TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_ap_payments_invoice ON ap_payments(ap_invoice_id);

-- Controls
CREATE TABLE IF NOT EXISTS approvals (
    id BIGSERIAL PRIMARY KEY,
    module TEXT NOT NULL,
    ref_id UUID NOT NULL,
    actor_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    action TEXT NOT NULL CHECK (action IN ('SUBMIT','APPROVE','REJECT')),
    note TEXT NOT NULL DEFAULT '',
    at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_approvals_ref ON approvals(module, ref_id);

CREATE TABLE IF NOT EXISTS idempotency_keys (
    key TEXT PRIMARY KEY,
    module TEXT NOT NULL,
    ref_id UUID NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Audit already exists from previous phases. Ensure helpful index
CREATE INDEX IF NOT EXISTS idx_audit_logs_module ON audit_logs(entity);
