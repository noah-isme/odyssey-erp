-- P1 Phase A: Sales returns + AR credit notes + return delivery orders

-- ============================================================================
-- RETURN DELIVERY ORDERS
-- ============================================================================

CREATE TYPE return_delivery_order_status AS ENUM ('DRAFT', 'CONFIRMED', 'CANCELLED');

CREATE TABLE return_delivery_orders (
    id BIGSERIAL PRIMARY KEY,
    number TEXT NOT NULL UNIQUE,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    customer_id BIGINT NOT NULL REFERENCES customers(id) ON DELETE RESTRICT,
    original_delivery_order_id BIGINT NOT NULL REFERENCES delivery_orders(id) ON DELETE RESTRICT,
    warehouse_id BIGINT NOT NULL REFERENCES warehouses(id) ON DELETE RESTRICT,

    return_date DATE NOT NULL,
    status return_delivery_order_status NOT NULL DEFAULT 'DRAFT',

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

CREATE INDEX idx_return_delivery_orders_company_status ON return_delivery_orders(company_id, status);
CREATE INDEX idx_return_delivery_orders_original_do ON return_delivery_orders(original_delivery_order_id);
CREATE INDEX idx_return_delivery_orders_customer ON return_delivery_orders(customer_id);
CREATE INDEX idx_return_delivery_orders_warehouse ON return_delivery_orders(warehouse_id);

CREATE TABLE return_delivery_order_lines (
    id BIGSERIAL PRIMARY KEY,
    return_delivery_order_id BIGINT NOT NULL REFERENCES return_delivery_orders(id) ON DELETE CASCADE,
    delivery_order_line_id BIGINT NOT NULL REFERENCES delivery_order_lines(id) ON DELETE RESTRICT,
    product_id BIGINT NOT NULL REFERENCES products(id) ON DELETE RESTRICT,

    quantity_returned NUMERIC(14,4) NOT NULL CHECK (quantity_returned > 0),
    unit_price NUMERIC(18,2) NOT NULL DEFAULT 0,

    -- Restock control: null means return to original warehouse, otherwise put away here
    restock_warehouse_id BIGINT REFERENCES warehouses(id) ON DELETE RESTRICT,
    lot_number TEXT,
    serial_numbers TEXT[],

    notes TEXT,
    line_order INT NOT NULL DEFAULT 0,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_return_delivery_order_lines_rdo ON return_delivery_order_lines(return_delivery_order_id);
CREATE INDEX idx_return_delivery_order_lines_do_line ON return_delivery_order_lines(delivery_order_line_id);
CREATE INDEX idx_return_delivery_order_lines_product ON return_delivery_order_lines(product_id);

CREATE OR REPLACE FUNCTION enforce_return_delivery_quantity()
RETURNS TRIGGER AS $$
DECLARE
    delivered_qty NUMERIC;
    returned_qty NUMERIC;
BEGIN
    SELECT quantity_delivered INTO delivered_qty
    FROM delivery_order_lines
    WHERE id = NEW.delivery_order_line_id
    FOR UPDATE;

    SELECT COALESCE(SUM(l.quantity_returned), 0) INTO returned_qty
    FROM return_delivery_order_lines l
    JOIN return_delivery_orders r ON r.id = l.return_delivery_order_id
    WHERE l.delivery_order_line_id = NEW.delivery_order_line_id
      AND r.status <> 'CANCELLED'
      AND (TG_OP = 'INSERT' OR l.id <> NEW.id);

    IF returned_qty + NEW.quantity_returned > delivered_qty THEN
        RAISE EXCEPTION 'cumulative returned quantity exceeds delivered quantity';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_return_delivery_quantity
BEFORE INSERT OR UPDATE OF quantity_returned ON return_delivery_order_lines
FOR EACH ROW EXECUTE FUNCTION enforce_return_delivery_quantity();

-- ============================================================================
-- AR CREDIT NOTES
-- ============================================================================

CREATE TYPE ar_credit_note_status AS ENUM ('DRAFT', 'POSTED', 'VOID');

CREATE TABLE ar_credit_notes (
    id BIGSERIAL PRIMARY KEY,
    number TEXT NOT NULL UNIQUE,
    customer_id BIGINT NOT NULL REFERENCES customers(id) ON DELETE RESTRICT,
    ar_invoice_id BIGINT NOT NULL REFERENCES ar_invoices(id) ON DELETE RESTRICT,
    return_delivery_order_id BIGINT NULL REFERENCES return_delivery_orders(id) ON DELETE SET NULL,

    currency TEXT NOT NULL DEFAULT 'IDR',
    reason TEXT NOT NULL DEFAULT '',

    subtotal NUMERIC(15,4) NOT NULL DEFAULT 0,
    tax_amount NUMERIC(15,4) NOT NULL DEFAULT 0,
    total NUMERIC(15,4) NOT NULL DEFAULT 0,

    status ar_credit_note_status NOT NULL DEFAULT 'DRAFT',

    posted_at TIMESTAMPTZ,
    posted_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    voided_at TIMESTAMPTZ,
    voided_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    void_reason TEXT,

    created_by BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_ar_credit_notes_customer ON ar_credit_notes(customer_id);
CREATE INDEX idx_ar_credit_notes_invoice ON ar_credit_notes(ar_invoice_id);
CREATE INDEX idx_ar_credit_notes_rdo ON ar_credit_notes(return_delivery_order_id);
CREATE INDEX idx_ar_credit_notes_status ON ar_credit_notes(status);
CREATE UNIQUE INDEX uq_ar_credit_notes_return_delivery ON ar_credit_notes(return_delivery_order_id)
WHERE return_delivery_order_id IS NOT NULL;

CREATE TABLE ar_credit_note_lines (
    id BIGSERIAL PRIMARY KEY,
    ar_credit_note_id BIGINT NOT NULL REFERENCES ar_credit_notes(id) ON DELETE CASCADE,
    ar_invoice_line_id BIGINT NULL REFERENCES ar_invoice_lines(id) ON DELETE SET NULL,
    return_delivery_order_line_id BIGINT NULL REFERENCES return_delivery_order_lines(id) ON DELETE SET NULL,
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

CREATE INDEX idx_ar_credit_note_lines_credit_note ON ar_credit_note_lines(ar_credit_note_id);
CREATE INDEX idx_ar_credit_note_lines_product ON ar_credit_note_lines(product_id);

CREATE TABLE ar_credit_note_allocations (
    id BIGSERIAL PRIMARY KEY,
    ar_credit_note_id BIGINT NOT NULL REFERENCES ar_credit_notes(id) ON DELETE CASCADE,
    ar_invoice_id BIGINT NOT NULL REFERENCES ar_invoices(id) ON DELETE CASCADE,
    amount NUMERIC(15,4) NOT NULL CHECK (amount > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_ar_credit_note_alloc_credit_note ON ar_credit_note_allocations(ar_credit_note_id);
CREATE INDEX idx_ar_credit_note_alloc_invoice ON ar_credit_note_allocations(ar_invoice_id);
CREATE UNIQUE INDEX uq_ar_credit_note_allocation ON ar_credit_note_allocations(ar_credit_note_id, ar_invoice_id);

-- ============================================================================
-- NUMBER GENERATORS
-- ============================================================================

CREATE OR REPLACE FUNCTION generate_ar_credit_note_number()
RETURNS TEXT AS $$
DECLARE
    prefix TEXT := 'CN-' || TO_CHAR(NOW(), 'YYMM') || '-';
    seq INT;
BEGIN
    SELECT COALESCE(MAX(CAST(SUBSTRING(number FROM LENGTH(prefix)+1) AS INT)), 0) + 1
    INTO seq
    FROM ar_credit_notes
    WHERE number LIKE prefix || '%';
    RETURN prefix || LPAD(seq::TEXT, 5, '0');
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION generate_return_delivery_order_number(p_company_id BIGINT, p_date DATE)
RETURNS TEXT AS $$
DECLARE
    v_count INT;
    v_year_month TEXT;
    v_doc_number TEXT;
BEGIN
    v_year_month := TO_CHAR(p_date, 'YYYYMM');

    SELECT COUNT(*) INTO v_count
    FROM return_delivery_orders
    WHERE company_id = p_company_id
      AND DATE_TRUNC('month', return_date) = DATE_TRUNC('month', p_date);

    v_doc_number := 'RDO-' || v_year_month || '-' || LPAD((v_count + 1)::TEXT, 5, '0');

    RETURN v_doc_number;
END;
$$ LANGUAGE plpgsql;

-- ============================================================================
-- UPDATED VIEWS / TRIGGERS
-- ============================================================================

-- Replace the invoice balance view to net credit note allocations as well
CREATE OR REPLACE VIEW v_ar_invoice_balance AS
SELECT
    i.id,
    i.number,
    i.customer_id,
    c.name AS customer_name,
    i.delivery_order_id,
    i.subtotal,
    i.tax_amount,
    i.total,
    COALESCE(pa.paid_amount, 0) + COALESCE(cna.credit_amount, 0) AS paid_amount,
    GREATEST(i.total - COALESCE(pa.paid_amount, 0) - COALESCE(cna.credit_amount, 0), 0) AS balance,
    i.status,
    i.due_at,
    i.created_at,
    CASE
        WHEN i.status = 'PAID' THEN 0
        WHEN i.due_at > NOW() THEN 0
        ELSE EXTRACT(DAY FROM NOW() - i.due_at)::INT
    END AS days_overdue
FROM ar_invoices i
LEFT JOIN customers c ON c.id = i.customer_id
LEFT JOIN (
    SELECT ar_invoice_id, SUM(amount) AS paid_amount
    FROM ar_payment_allocations
    GROUP BY ar_invoice_id
) pa ON pa.ar_invoice_id = i.id
LEFT JOIN (
    SELECT ar_invoice_id, SUM(amount) AS credit_amount
    FROM ar_credit_note_allocations
    GROUP BY ar_invoice_id
) cna ON cna.ar_invoice_id = i.id;

-- Update invoice status when credit notes are allocated too
CREATE OR REPLACE FUNCTION update_invoice_status_on_credit_note_allocation()
RETURNS TRIGGER AS $$
DECLARE
    invoice_total NUMERIC;
    total_paid NUMERIC;
BEGIN
    SELECT total INTO invoice_total FROM ar_invoices WHERE id = NEW.ar_invoice_id;
    SELECT COALESCE(SUM(amount), 0) INTO total_paid
    FROM ar_payment_allocations WHERE ar_invoice_id = NEW.ar_invoice_id;
    SELECT COALESCE(SUM(amount), 0) + total_paid INTO total_paid
    FROM ar_credit_note_allocations WHERE ar_invoice_id = NEW.ar_invoice_id;

    IF total_paid >= invoice_total THEN
        UPDATE ar_invoices SET status = 'PAID', updated_at = NOW()
        WHERE id = NEW.ar_invoice_id AND status = 'POSTED';
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_update_invoice_status_on_credit_note
AFTER INSERT ON ar_credit_note_allocations
FOR EACH ROW EXECUTE FUNCTION update_invoice_status_on_credit_note_allocation();

-- Auto-update timestamps
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_ar_credit_notes_updated_at
    BEFORE UPDATE ON ar_credit_notes
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER trg_return_delivery_orders_updated_at
    BEFORE UPDATE ON return_delivery_orders
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER trg_return_delivery_order_lines_updated_at
    BEFORE UPDATE ON return_delivery_order_lines
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- ============================================================================
-- RBAC PERMISSIONS
-- ============================================================================

INSERT INTO permissions (name, description) VALUES
    ('finance.ar.credit_note.view', 'View AR credit notes'),
    ('finance.ar.credit_note.create', 'Create AR credit notes'),
    ('finance.ar.credit_note.post', 'Post AR credit notes'),
    ('finance.ar.credit_note.void', 'Void AR credit notes'),
    ('delivery.return.view', 'View return delivery orders'),
    ('delivery.return.create', 'Create return delivery orders'),
    ('delivery.return.post', 'Post return delivery orders'),
    ('delivery.return.void', 'Void return delivery orders')
ON CONFLICT (name) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r, permissions p
WHERE r.name IN ('Admin', 'Finance Manager', 'Sales Manager')
AND p.name IN (
    'finance.ar.credit_note.view', 'finance.ar.credit_note.create',
    'finance.ar.credit_note.post', 'finance.ar.credit_note.void'
)
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r, permissions p
WHERE r.name IN ('Admin', 'Sales Manager', 'Warehouse Staff')
AND p.name IN (
    'delivery.return.view', 'delivery.return.create',
    'delivery.return.post', 'delivery.return.void'
)
ON CONFLICT DO NOTHING;

-- ============================================================================
-- ACCOUNT MAPPINGS FOR AR
-- ============================================================================

INSERT INTO accounts (code, name, type, parent_id, is_active, created_at, updated_at)
SELECT '4160', 'Tax Output', 'LIABILITY', a.id, TRUE, NOW(), NOW()
FROM accounts a WHERE a.code = '2000'
ON CONFLICT (code) DO NOTHING;

INSERT INTO account_mappings (module, key, account_id, created_at, updated_at)
SELECT 'AR', 'ar.invoice.ar', a.id, NOW(), NOW()
FROM accounts a WHERE a.code = '1200'
ON CONFLICT (module, key) DO UPDATE SET account_id = EXCLUDED.account_id, updated_at = NOW();

INSERT INTO account_mappings (module, key, account_id, created_at, updated_at)
SELECT 'AR', 'ar.invoice.revenue', a.id, NOW(), NOW()
FROM accounts a WHERE a.code = '4000'
ON CONFLICT (module, key) DO UPDATE SET account_id = EXCLUDED.account_id, updated_at = NOW();

-- Optional tax mapping; rows without a matching account are simply skipped.
INSERT INTO account_mappings (module, key, account_id, created_at, updated_at)
SELECT 'AR', 'ar.invoice.tax', a.id, NOW(), NOW()
FROM accounts a WHERE a.code = '4160'
ON CONFLICT (module, key) DO UPDATE SET account_id = EXCLUDED.account_id, updated_at = NOW();

INSERT INTO account_mappings (module, key, account_id, created_at, updated_at)
SELECT 'AR', 'ar.return.cogs', a.id, NOW(), NOW()
FROM accounts a WHERE a.code = '5100'
ON CONFLICT (module, key) DO UPDATE SET account_id = EXCLUDED.account_id, updated_at = NOW();
