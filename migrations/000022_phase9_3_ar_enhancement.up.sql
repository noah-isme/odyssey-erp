-- Phase 9.3: AR Invoice Enhancement
-- Add invoice lines, payment allocations, and workflow support

-- AR Invoice Lines table
CREATE TABLE ar_invoice_lines (
    id BIGSERIAL PRIMARY KEY,
    ar_invoice_id BIGINT NOT NULL REFERENCES ar_invoices(id) ON DELETE CASCADE,
    delivery_order_line_id BIGINT NULL,
    product_id BIGINT NOT NULL,
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

CREATE INDEX idx_ar_invoice_lines_invoice ON ar_invoice_lines(ar_invoice_id);
CREATE INDEX idx_ar_invoice_lines_product ON ar_invoice_lines(product_id);

-- Add delivery_order_id and workflow columns to ar_invoices
ALTER TABLE ar_invoices ADD COLUMN IF NOT EXISTS delivery_order_id BIGINT NULL;
ALTER TABLE ar_invoices ADD COLUMN IF NOT EXISTS subtotal NUMERIC(15,4) NOT NULL DEFAULT 0;
ALTER TABLE ar_invoices ADD COLUMN IF NOT EXISTS tax_amount NUMERIC(15,4) NOT NULL DEFAULT 0;
ALTER TABLE ar_invoices ADD COLUMN IF NOT EXISTS posted_at TIMESTAMPTZ NULL;
ALTER TABLE ar_invoices ADD COLUMN IF NOT EXISTS posted_by BIGINT NULL;
ALTER TABLE ar_invoices ADD COLUMN IF NOT EXISTS voided_at TIMESTAMPTZ NULL;
ALTER TABLE ar_invoices ADD COLUMN IF NOT EXISTS voided_by BIGINT NULL;
ALTER TABLE ar_invoices ADD COLUMN IF NOT EXISTS void_reason TEXT NULL;
ALTER TABLE ar_invoices ADD COLUMN IF NOT EXISTS created_by BIGINT NULL;

CREATE INDEX IF NOT EXISTS idx_ar_invoices_delivery ON ar_invoices(delivery_order_id);

-- Payment allocation table (for partial payments across invoices)
CREATE TABLE ar_payment_allocations (
    id BIGSERIAL PRIMARY KEY,
    ar_payment_id BIGINT NOT NULL REFERENCES ar_payments(id) ON DELETE CASCADE,
    ar_invoice_id BIGINT NOT NULL REFERENCES ar_invoices(id) ON DELETE CASCADE,
    amount NUMERIC(15,4) NOT NULL CHECK (amount > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_ar_payment_alloc_payment ON ar_payment_allocations(ar_payment_id);
CREATE INDEX idx_ar_payment_alloc_invoice ON ar_payment_allocations(ar_invoice_id);

-- Add created_by to ar_payments
ALTER TABLE ar_payments ADD COLUMN IF NOT EXISTS created_by BIGINT NULL;

-- View: AR Invoice with balance
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
    COALESCE(SUM(pa.amount), 0) AS paid_amount,
    i.total - COALESCE(SUM(pa.amount), 0) AS balance,
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
LEFT JOIN ar_payment_allocations pa ON pa.ar_invoice_id = i.id
GROUP BY i.id, c.name;

-- Helper function for AR invoice number
CREATE OR REPLACE FUNCTION generate_ar_invoice_number()
RETURNS TEXT AS $$
DECLARE
    prefix TEXT := 'INV-' || TO_CHAR(NOW(), 'YYMM') || '-';
    seq INT;
BEGIN
    SELECT COALESCE(MAX(CAST(SUBSTRING(number FROM LENGTH(prefix)+1) AS INT)), 0) + 1
    INTO seq
    FROM ar_invoices
    WHERE number LIKE prefix || '%';
    RETURN prefix || LPAD(seq::TEXT, 5, '0');
END;
$$ LANGUAGE plpgsql;

-- Helper function for AR payment number
CREATE OR REPLACE FUNCTION generate_ar_payment_number()
RETURNS TEXT AS $$
DECLARE
    prefix TEXT := 'PAY-' || TO_CHAR(NOW(), 'YYMM') || '-';
    seq INT;
BEGIN
    SELECT COALESCE(MAX(CAST(SUBSTRING(number FROM LENGTH(prefix)+1) AS INT)), 0) + 1
    INTO seq
    FROM ar_payments
    WHERE number LIKE prefix || '%';
    RETURN prefix || LPAD(seq::TEXT, 5, '0');
END;
$$ LANGUAGE plpgsql;

-- Trigger to auto-update invoice status when fully paid
CREATE OR REPLACE FUNCTION update_invoice_status_on_payment()
RETURNS TRIGGER AS $$
DECLARE
    invoice_total NUMERIC;
    total_paid NUMERIC;
BEGIN
    SELECT total INTO invoice_total FROM ar_invoices WHERE id = NEW.ar_invoice_id;
    SELECT COALESCE(SUM(amount), 0) INTO total_paid 
    FROM ar_payment_allocations WHERE ar_invoice_id = NEW.ar_invoice_id;
    
    IF total_paid >= invoice_total THEN
        UPDATE ar_invoices SET status = 'PAID', updated_at = NOW() 
        WHERE id = NEW.ar_invoice_id AND status = 'POSTED';
    END IF;
    
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_update_invoice_status
AFTER INSERT ON ar_payment_allocations
FOR EACH ROW EXECUTE FUNCTION update_invoice_status_on_payment();

-- Add AR permissions
INSERT INTO permissions (name, description) VALUES
    ('finance.ar.create', 'Create AR invoices'),
    ('finance.ar.post', 'Post AR invoices'),
    ('finance.ar.void', 'Void AR invoices'),
    ('finance.ar.payment', 'Record AR payments')
ON CONFLICT (name) DO NOTHING;

-- Assign to finance roles
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r, permissions p
WHERE r.name IN ('Admin', 'Finance Manager')
AND p.name IN ('finance.ar.create', 'finance.ar.post', 'finance.ar.void', 'finance.ar.payment')
ON CONFLICT DO NOTHING;
