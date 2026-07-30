-- P1 Phase B: GRN-based supplier goods returns + AP debit notes
--
-- Flow: POSTED GRN → Goods Return (DRAFT→CONFIRMED/CANCELLED) → negative
--   inventory adjustment → AP Debit Note (DRAFT→POSTED/VOID) on the
--   matching AP invoice → journal Dr AP / Cr inventory|expense + tax

-- ============================================================================
-- GOODS RETURNS (from GRN / supplier returns)
-- ============================================================================

CREATE TYPE goods_return_grn_status AS ENUM ('DRAFT', 'CONFIRMED', 'CANCELLED');

CREATE TABLE goods_return_grns (
    id BIGSERIAL PRIMARY KEY,
    number TEXT NOT NULL UNIQUE,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    supplier_id BIGINT NOT NULL REFERENCES suppliers(id) ON DELETE RESTRICT,
    grn_id BIGINT NOT NULL REFERENCES grns(id) ON DELETE RESTRICT,
    warehouse_id BIGINT NOT NULL REFERENCES warehouses(id) ON DELETE RESTRICT,

    return_date DATE NOT NULL,
    status goods_return_grn_status NOT NULL DEFAULT 'DRAFT',

    reason TEXT NOT NULL DEFAULT '',
    notes TEXT,

    created_by BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    confirmed_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    confirmed_at TIMESTAMPTZ,
    voided_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    voided_at TIMESTAMPTZ,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_goods_return_grns_company_status ON goods_return_grns(company_id, status);
CREATE INDEX idx_goods_return_grns_supplier ON goods_return_grns(supplier_id);
CREATE INDEX idx_goods_return_grns_grn ON goods_return_grns(grn_id);
CREATE INDEX idx_goods_return_grns_warehouse ON goods_return_grns(warehouse_id);

CREATE TABLE goods_return_grn_lines (
    id BIGSERIAL PRIMARY KEY,
    goods_return_grn_id BIGINT NOT NULL REFERENCES goods_return_grns(id) ON DELETE CASCADE,
    grn_line_id BIGINT NOT NULL REFERENCES grn_lines(id) ON DELETE RESTRICT,
    product_id BIGINT NOT NULL REFERENCES products(id) ON DELETE RESTRICT,

    quantity_returned NUMERIC(14,4) NOT NULL CHECK (quantity_returned > 0),
    unit_cost NUMERIC(18,2) NOT NULL DEFAULT 0,

    notes TEXT,
    line_order INT NOT NULL DEFAULT 0,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_goods_return_grn_lines_header ON goods_return_grn_lines(goods_return_grn_id);
CREATE INDEX idx_goods_return_grn_lines_grn_line ON goods_return_grn_lines(grn_line_id);
CREATE INDEX idx_goods_return_grn_lines_product ON goods_return_grn_lines(product_id);

-- Enforce cumulative returned qty does not exceed GRN line qty
CREATE OR REPLACE FUNCTION enforce_goods_return_grn_quantity()
RETURNS TRIGGER AS $$
DECLARE
    grn_qty NUMERIC;
    returned_qty NUMERIC;
BEGIN
    SELECT qty INTO grn_qty
    FROM grn_lines
    WHERE id = NEW.grn_line_id
    FOR UPDATE;

    SELECT COALESCE(SUM(l.quantity_returned), 0) INTO returned_qty
    FROM goods_return_grn_lines l
    JOIN goods_return_grns r ON r.id = l.goods_return_grn_id
    WHERE l.grn_line_id = NEW.grn_line_id
      AND r.status <> 'CANCELLED'
      AND (TG_OP = 'INSERT' OR l.id <> NEW.id);

    IF returned_qty + NEW.quantity_returned > grn_qty THEN
        RAISE EXCEPTION 'cumulative returned quantity exceeds GRN line quantity';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_goods_return_grn_quantity
BEFORE INSERT OR UPDATE OF quantity_returned ON goods_return_grn_lines
FOR EACH ROW EXECUTE FUNCTION enforce_goods_return_grn_quantity();

-- ============================================================================
-- AP DEBIT NOTES
-- ============================================================================

CREATE TYPE ap_debit_note_status AS ENUM ('DRAFT', 'POSTED', 'VOID');

CREATE TABLE ap_debit_notes (
    id BIGSERIAL PRIMARY KEY,
    number TEXT NOT NULL UNIQUE,
    supplier_id BIGINT NOT NULL REFERENCES suppliers(id) ON DELETE RESTRICT,
    ap_invoice_id BIGINT NOT NULL REFERENCES ap_invoices(id) ON DELETE RESTRICT,
    goods_return_grn_id BIGINT NULL REFERENCES goods_return_grns(id) ON DELETE SET NULL,

    currency TEXT NOT NULL DEFAULT 'IDR',
    reason TEXT NOT NULL DEFAULT '',

    subtotal NUMERIC(15,4) NOT NULL DEFAULT 0,
    tax_amount NUMERIC(15,4) NOT NULL DEFAULT 0,
    total NUMERIC(15,4) NOT NULL DEFAULT 0,

    status ap_debit_note_status NOT NULL DEFAULT 'DRAFT',

    posted_at TIMESTAMPTZ,
    posted_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    voided_at TIMESTAMPTZ,
    voided_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    void_reason TEXT,

    created_by BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_ap_debit_notes_supplier ON ap_debit_notes(supplier_id);
CREATE INDEX idx_ap_debit_notes_invoice ON ap_debit_notes(ap_invoice_id);
CREATE INDEX idx_ap_debit_notes_return ON ap_debit_notes(goods_return_grn_id);
CREATE INDEX idx_ap_debit_notes_status ON ap_debit_notes(status);
CREATE UNIQUE INDEX uq_ap_debit_notes_return ON ap_debit_notes(goods_return_grn_id)
    WHERE goods_return_grn_id IS NOT NULL;

CREATE TABLE ap_debit_note_lines (
    id BIGSERIAL PRIMARY KEY,
    ap_debit_note_id BIGINT NOT NULL REFERENCES ap_debit_notes(id) ON DELETE CASCADE,
    ap_invoice_line_id BIGINT NULL REFERENCES ap_invoice_lines(id) ON DELETE SET NULL,
    goods_return_grn_line_id BIGINT NULL REFERENCES goods_return_grn_lines(id) ON DELETE SET NULL,
    product_id BIGINT NOT NULL REFERENCES products(id) ON DELETE RESTRICT,

    description TEXT NOT NULL DEFAULT '',
    quantity NUMERIC(15,4) NOT NULL DEFAULT 0,
    unit_price NUMERIC(15,4) NOT NULL DEFAULT 0,
    discount_pct NUMERIC(5,2) NOT NULL DEFAULT 0,
    tax_pct NUMERIC(5,2) NOT NULL DEFAULT 0,

    subtotal NUMERIC(15,4) NOT NULL DEFAULT 0,
    tax_amount NUMERIC(15,4) NOT NULL DEFAULT 0,
    total NUMERIC(15,4) NOT NULL DEFAULT 0,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_ap_debit_note_lines_debit_note ON ap_debit_note_lines(ap_debit_note_id);
CREATE INDEX idx_ap_debit_note_lines_product ON ap_debit_note_lines(product_id);

CREATE TABLE ap_debit_note_allocations (
    id BIGSERIAL PRIMARY KEY,
    ap_debit_note_id BIGINT NOT NULL REFERENCES ap_debit_notes(id) ON DELETE CASCADE,
    ap_invoice_id BIGINT NOT NULL REFERENCES ap_invoices(id) ON DELETE CASCADE,
    amount NUMERIC(15,4) NOT NULL CHECK (amount > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_ap_dn_alloc_debit_note ON ap_debit_note_allocations(ap_debit_note_id);
CREATE INDEX idx_ap_dn_alloc_invoice ON ap_debit_note_allocations(ap_invoice_id);
CREATE UNIQUE INDEX uq_ap_debit_note_allocation ON ap_debit_note_allocations(ap_debit_note_id, ap_invoice_id);

-- ============================================================================
-- NUMBER GENERATORS
-- ============================================================================

CREATE OR REPLACE FUNCTION generate_ap_debit_note_number()
RETURNS TEXT AS $$
DECLARE
    prefix TEXT := 'DN-' || TO_CHAR(NOW(), 'YYMM') || '-';
    seq INT;
BEGIN
    SELECT COALESCE(MAX(CAST(SUBSTRING(number FROM LENGTH(prefix)+1) AS INT)), 0) + 1
    INTO seq
    FROM ap_debit_notes
    WHERE number LIKE prefix || '%';
    RETURN prefix || LPAD(seq::TEXT, 5, '0');
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION generate_goods_return_grn_number()
RETURNS TEXT AS $$
DECLARE
    prefix TEXT := 'GRN-RET-' || TO_CHAR(NOW(), 'YYMM') || '-';
    seq INT;
BEGIN
    SELECT COALESCE(MAX(CAST(SUBSTRING(number FROM LENGTH(prefix)+1) AS INT)), 0) + 1
    INTO seq
    FROM goods_return_grns
    WHERE number LIKE prefix || '%';
    RETURN prefix || LPAD(seq::TEXT, 5, '0');
END;
$$ LANGUAGE plpgsql;

-- ============================================================================
-- VIEWS / TRIGGERS
-- ============================================================================

-- Replace the AP invoice balance view to net debit note allocations as well
CREATE OR REPLACE VIEW v_ap_invoice_balance AS
SELECT
    i.id,
    i.number,
    i.supplier_id,
    i.grn_id,
    i.subtotal,
    i.tax_amount,
    i.total,
    COALESCE(pa.paid_amount, 0) + COALESCE(dna.debit_amount, 0) AS paid_amount,
    GREATEST(i.total - COALESCE(pa.paid_amount, 0) - COALESCE(dna.debit_amount, 0), 0) AS balance,
    i.status,
    i.due_at,
    i.created_at
FROM ap_invoices i
LEFT JOIN (
    SELECT ap_invoice_id, SUM(amount) AS paid_amount
    FROM ap_payment_allocations
    GROUP BY ap_invoice_id
) pa ON pa.ap_invoice_id = i.id
LEFT JOIN (
    SELECT ap_invoice_id, SUM(amount) AS debit_amount
    FROM ap_debit_note_allocations
    GROUP BY ap_invoice_id
) dna ON dna.ap_invoice_id = i.id;

-- Update AP invoice status when debit note allocated
CREATE OR REPLACE FUNCTION update_ap_invoice_status_on_debit_note()
RETURNS TRIGGER AS $$
DECLARE
    invoice_total NUMERIC;
    total_reduced NUMERIC;
BEGIN
    SELECT total INTO invoice_total FROM ap_invoices WHERE id = NEW.ap_invoice_id;
    SELECT COALESCE(SUM(amount), 0) INTO total_reduced
    FROM ap_payment_allocations WHERE ap_invoice_id = NEW.ap_invoice_id;
    SELECT COALESCE(SUM(amount), 0) + total_reduced INTO total_reduced
    FROM ap_debit_note_allocations WHERE ap_invoice_id = NEW.ap_invoice_id;

    IF total_reduced >= invoice_total THEN
        UPDATE ap_invoices SET status = 'PAID', updated_at = NOW()
        WHERE id = NEW.ap_invoice_id AND status = 'POSTED';
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_update_ap_invoice_status_on_debit_note
AFTER INSERT ON ap_debit_note_allocations
FOR EACH ROW EXECUTE FUNCTION update_ap_invoice_status_on_debit_note();

-- Auto-update timestamps
CREATE TRIGGER trg_goods_return_grns_updated_at
    BEFORE UPDATE ON goods_return_grns
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER trg_goods_return_grn_lines_updated_at
    BEFORE UPDATE ON goods_return_grn_lines
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER trg_ap_debit_notes_updated_at
    BEFORE UPDATE ON ap_debit_notes
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- ============================================================================
-- ACCOUNT MAPPINGS FOR AP DEBIT NOTES
-- ============================================================================

INSERT INTO account_mappings (module, key, account_id, created_at, updated_at)
SELECT 'AP', 'ap.debit_note.ap', a.id, NOW(), NOW()
FROM accounts a WHERE a.code = '2100'
ON CONFLICT (module, key) DO UPDATE SET account_id = EXCLUDED.account_id, updated_at = NOW();

INSERT INTO account_mappings (module, key, account_id, created_at, updated_at)
SELECT 'AP', 'ap.debit_note.inventory', a.id, NOW(), NOW()
FROM accounts a WHERE a.code = '1200'
ON CONFLICT (module, key) DO UPDATE SET account_id = EXCLUDED.account_id, updated_at = NOW();

INSERT INTO account_mappings (module, key, account_id, created_at, updated_at)
SELECT 'AP', 'ap.debit_note.expense', a.id, NOW(), NOW()
FROM accounts a WHERE a.code = '5000'
ON CONFLICT (module, key) DO UPDATE SET account_id = EXCLUDED.account_id, updated_at = NOW();

INSERT INTO account_mappings (module, key, account_id, created_at, updated_at)
SELECT 'AP', 'ap.debit_note.tax', a.id, NOW(), NOW()
FROM accounts a WHERE a.code = '4160'
ON CONFLICT (module, key) DO UPDATE SET account_id = EXCLUDED.account_id, updated_at = NOW();

-- ============================================================================
-- RBAC PERMISSIONS
-- ============================================================================

INSERT INTO permissions (name, description) VALUES
    ('procurement.return.view', 'View GRN-based goods returns'),
    ('procurement.return.create', 'Create goods returns from GRN'),
    ('procurement.return.post', 'Confirm goods returns'),
    ('procurement.return.void', 'Cancel goods returns'),
    ('finance.ap.debit_note.view', 'View AP debit notes'),
    ('finance.ap.debit_note.create', 'Create AP debit notes'),
    ('finance.ap.debit_note.post', 'Post AP debit notes'),
    ('finance.ap.debit_note.void', 'Void AP debit notes')
ON CONFLICT (name) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r, permissions p
WHERE r.name IN ('Admin', 'Finance Manager', 'Purchasing Manager')
AND p.name IN (
    'procurement.return.view', 'procurement.return.create',
    'procurement.return.post', 'procurement.return.void'
)
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r, permissions p
WHERE r.name IN ('Admin', 'Finance Manager')
AND p.name IN (
    'finance.ap.debit_note.view', 'finance.ap.debit_note.create',
    'finance.ap.debit_note.post', 'finance.ap.debit_note.void'
)
ON CONFLICT DO NOTHING;
