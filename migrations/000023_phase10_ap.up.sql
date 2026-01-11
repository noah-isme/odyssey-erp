-- AP Invoice Enhancements
ALTER TABLE ap_invoices ADD COLUMN IF NOT EXISTS subtotal NUMERIC(15,2) NOT NULL DEFAULT 0;
ALTER TABLE ap_invoices ADD COLUMN IF NOT EXISTS tax_amount NUMERIC(15,2) NOT NULL DEFAULT 0;
ALTER TABLE ap_invoices ADD COLUMN IF NOT EXISTS posted_at TIMESTAMPTZ;
ALTER TABLE ap_invoices ADD COLUMN IF NOT EXISTS posted_by BIGINT REFERENCES users(id);
ALTER TABLE ap_invoices ADD COLUMN IF NOT EXISTS voided_at TIMESTAMPTZ;
ALTER TABLE ap_invoices ADD COLUMN IF NOT EXISTS voided_by BIGINT REFERENCES users(id);
ALTER TABLE ap_invoices ADD COLUMN IF NOT EXISTS void_reason TEXT;
ALTER TABLE ap_invoices ADD COLUMN IF NOT EXISTS created_by BIGINT REFERENCES users(id);
ALTER TABLE ap_invoices ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

-- Create AP Invoice Lines
CREATE TABLE IF NOT EXISTS ap_invoice_lines (
    id BIGSERIAL PRIMARY KEY,
    ap_invoice_id BIGINT NOT NULL REFERENCES ap_invoices(id) ON DELETE CASCADE,
    grn_line_id BIGINT REFERENCES grn_lines(id),
    product_id BIGINT NOT NULL REFERENCES products(id),
    description TEXT NOT NULL,
    quantity NUMERIC(15,2) NOT NULL DEFAULT 0,
    unit_price NUMERIC(15,2) NOT NULL DEFAULT 0,
    discount_pct NUMERIC(5,2) NOT NULL DEFAULT 0,
    tax_pct NUMERIC(5,2) NOT NULL DEFAULT 0,
    subtotal NUMERIC(15,2) NOT NULL DEFAULT 0,
    tax_amount NUMERIC(15,2) NOT NULL DEFAULT 0,
    total NUMERIC(15,2) NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Enhance AP Payments
ALTER TABLE ap_payments ADD COLUMN IF NOT EXISTS created_by BIGINT REFERENCES users(id);
ALTER TABLE ap_payments ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT NOW();
ALTER TABLE ap_payments ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

-- Create AP Payment Allocations
CREATE TABLE IF NOT EXISTS ap_payment_allocations (
    id BIGSERIAL PRIMARY KEY,
    ap_payment_id BIGINT NOT NULL REFERENCES ap_payments(id) ON DELETE CASCADE,
    ap_invoice_id BIGINT NOT NULL REFERENCES ap_invoices(id) ON DELETE CASCADE,
    amount NUMERIC(15,2) NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes
CREATE INDEX IF NOT EXISTS idx_ap_invoices_posted_by ON ap_invoices(posted_by);
CREATE INDEX IF NOT EXISTS idx_ap_payment_allocations_payment ON ap_payment_allocations(ap_payment_id);
CREATE INDEX IF NOT EXISTS idx_ap_payment_allocations_invoice ON ap_payment_allocations(ap_invoice_id);

-- Permissions
INSERT INTO permissions (name, description) VALUES
    ('finance.ap.view', 'View AP invoices'),
    ('finance.ap.create', 'Create AP invoices'),
    ('finance.ap.post', 'Post AP invoices'),
    ('finance.ap.void', 'Void AP invoices'),
    ('finance.ap.payment', 'Record AP payments')
ON CONFLICT (name) DO NOTHING;

-- Assign to finance roles
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r, permissions p
WHERE r.name IN ('Admin', 'Finance Manager')
AND p.name LIKE 'finance.ap.%'
ON CONFLICT DO NOTHING;
