-- Phase 10: Accounts Receivable (AR)
-- Invoices and Payments

CREATE TABLE ar_invoices (
    id BIGSERIAL PRIMARY KEY,
    number TEXT NOT NULL UNIQUE,
    customer_id BIGINT NOT NULL REFERENCES customers(id) ON DELETE RESTRICT,
    so_id BIGINT NULL REFERENCES sales_orders(id) ON DELETE SET NULL,
    currency TEXT NOT NULL DEFAULT 'IDR',
    total NUMERIC(14,2) NOT NULL DEFAULT 0 CHECK (total >= 0),
    status TEXT NOT NULL CHECK (status IN ('DRAFT','POSTED','PAID','VOID')),
    due_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_ar_invoices_customer ON ar_invoices(customer_id);
CREATE INDEX idx_ar_invoices_status ON ar_invoices(status);
CREATE INDEX idx_ar_invoices_due_at ON ar_invoices(due_at);
CREATE INDEX idx_ar_invoices_so ON ar_invoices(so_id);

CREATE TABLE ar_payments (
    id BIGSERIAL PRIMARY KEY,
    number TEXT NOT NULL UNIQUE,
    ar_invoice_id BIGINT NOT NULL REFERENCES ar_invoices(id) ON DELETE CASCADE,
    amount NUMERIC(14,2) NOT NULL CHECK (amount > 0),
    paid_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    method TEXT NOT NULL DEFAULT 'TRANSFER',
    note TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_ar_payments_invoice ON ar_payments(ar_invoice_id);
CREATE INDEX idx_ar_payments_date ON ar_payments(paid_at);
