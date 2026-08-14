-- Seed data: E2E test fixture for AR Credit Notes (CN-SEED-0001)
-- Purpose: Provides test data for E2E visual and PDF testing of AR Credit Notes
-- Linked to seeded AR invoice INV-SEED-0001 and customer CUST-000001 (PT Maju Bersama)

BEGIN;

-- 1. Insert AR Credit Note Header
INSERT INTO ar_credit_notes (
    number,
    customer_id,
    ar_invoice_id,
    currency,
    reason,
    subtotal,
    tax_amount,
    total,
    status,
    posted_at,
    posted_by,
    created_by,
    created_at,
    updated_at
)
SELECT
    'CN-SEED-0001',
    c.id,
    i.id,
    'IDR',
    'Return of damaged goods',
    750000.0000,
    0.0000,
    750000.0000,
    'POSTED'::ar_credit_note_status,
    CURRENT_TIMESTAMP,
    1,
    1,
    CURRENT_TIMESTAMP,
    CURRENT_TIMESTAMP
FROM customers c
CROSS JOIN ar_invoices i
WHERE c.code = 'CUST-000001'
  AND i.number = 'INV-SEED-0001'
ON CONFLICT (number) DO UPDATE SET
    customer_id = EXCLUDED.customer_id,
    ar_invoice_id = EXCLUDED.ar_invoice_id,
    currency = EXCLUDED.currency,
    reason = EXCLUDED.reason,
    subtotal = EXCLUDED.subtotal,
    tax_amount = EXCLUDED.tax_amount,
    total = EXCLUDED.total,
    status = EXCLUDED.status,
    posted_at = EXCLUDED.posted_at,
    posted_by = EXCLUDED.posted_by,
    updated_at = CURRENT_TIMESTAMP;

-- 2. Insert AR Credit Note Lines (2 line items totaling 750,000.00)
-- Line 1: 500,000.00 (PRD-001)
INSERT INTO ar_credit_note_lines (
    ar_credit_note_id,
    product_id,
    description,
    quantity,
    unit_price,
    discount_pct,
    tax_pct,
    subtotal,
    tax_amount,
    total,
    created_at
)
SELECT
    cn.id,
    p.id,
    'Damaged item return - PRD-001',
    1.0000,
    500000.0000,
    0.00,
    0.00,
    500000.0000,
    0.0000,
    500000.0000,
    CURRENT_TIMESTAMP
FROM ar_credit_notes cn
CROSS JOIN LATERAL (
    SELECT id FROM products WHERE sku = 'PRD-001' UNION ALL SELECT id FROM products ORDER BY id LIMIT 1
) p
WHERE cn.number = 'CN-SEED-0001'
  AND NOT EXISTS (
      SELECT 1 FROM ar_credit_note_lines l
      WHERE l.ar_credit_note_id = cn.id AND l.description = 'Damaged item return - PRD-001'
  )
LIMIT 1;

-- Line 2: 250,000.00 (PRD-002)
INSERT INTO ar_credit_note_lines (
    ar_credit_note_id,
    product_id,
    description,
    quantity,
    unit_price,
    discount_pct,
    tax_pct,
    subtotal,
    tax_amount,
    total,
    created_at
)
SELECT
    cn.id,
    p.id,
    'Restocking allowance - PRD-002',
    1.0000,
    250000.0000,
    0.00,
    0.00,
    250000.0000,
    0.0000,
    250000.0000,
    CURRENT_TIMESTAMP
FROM ar_credit_notes cn
CROSS JOIN LATERAL (
    SELECT id FROM products WHERE sku = 'PRD-002' UNION ALL SELECT id FROM products ORDER BY id LIMIT 1
) p
WHERE cn.number = 'CN-SEED-0001'
  AND NOT EXISTS (
      SELECT 1 FROM ar_credit_note_lines l
      WHERE l.ar_credit_note_id = cn.id AND l.description = 'Restocking allowance - PRD-002'
  )
LIMIT 1;

-- 3. Insert AR Credit Note Allocation against the invoice
INSERT INTO ar_credit_note_allocations (
    ar_credit_note_id,
    ar_invoice_id,
    amount,
    created_at
)
SELECT
    cn.id,
    cn.ar_invoice_id,
    cn.total,
    CURRENT_TIMESTAMP
FROM ar_credit_notes cn
WHERE cn.number = 'CN-SEED-0001'
ON CONFLICT (ar_credit_note_id, ar_invoice_id) DO UPDATE SET
    amount = EXCLUDED.amount;

COMMIT;
