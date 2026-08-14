-- Seed data: E2E test fixture for AP Debit Notes (DN-SEED-0001)
-- Purpose: Provides test data for E2E visual and PDF testing of AP Debit Notes
-- Linked to seeded AP invoice INV-AP-202412-0001 and supplier SUP-001 (PT Elektronik Jaya)

BEGIN;

-- 1. Insert AP Debit Note Header
INSERT INTO ap_debit_notes (
    number,
    supplier_id,
    ap_invoice_id,
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
    'DN-SEED-0001',
    s.id,
    i.id,
    'IDR',
    'Price adjustment for defective goods',
    500000.0000,
    0.0000,
    500000.0000,
    'POSTED'::ap_debit_note_status,
    CURRENT_TIMESTAMP,
    1,
    1,
    CURRENT_TIMESTAMP,
    CURRENT_TIMESTAMP
FROM suppliers s
CROSS JOIN ap_invoices i
WHERE s.code = 'SUP-001'
  AND i.number = 'INV-AP-202412-0001'
ON CONFLICT (number) DO UPDATE SET
    supplier_id = EXCLUDED.supplier_id,
    ap_invoice_id = EXCLUDED.ap_invoice_id,
    currency = EXCLUDED.currency,
    reason = EXCLUDED.reason,
    subtotal = EXCLUDED.subtotal,
    tax_amount = EXCLUDED.tax_amount,
    total = EXCLUDED.total,
    status = EXCLUDED.status,
    posted_at = EXCLUDED.posted_at,
    posted_by = EXCLUDED.posted_by,
    updated_at = CURRENT_TIMESTAMP;

-- 2. Insert AP Debit Note Lines (2 line items totaling 500,000.00)
-- Line 1: 300,000.00 (PRD-001)
INSERT INTO ap_debit_note_lines (
    ap_debit_note_id,
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
    dn.id,
    p.id,
    'Defective RAM Module replacement concession',
    1.0000,
    300000.0000,
    0.00,
    0.00,
    300000.0000,
    0.0000,
    300000.0000,
    CURRENT_TIMESTAMP
FROM ap_debit_notes dn
CROSS JOIN LATERAL (
    SELECT id FROM products WHERE sku = 'PRD-001' UNION ALL SELECT id FROM products ORDER BY id LIMIT 1
) p
WHERE dn.number = 'DN-SEED-0001'
  AND NOT EXISTS (
      SELECT 1 FROM ap_debit_note_lines l
      WHERE l.ap_debit_note_id = dn.id AND l.description = 'Defective RAM Module replacement concession'
  )
LIMIT 1;

-- Line 2: 200,000.00 (PRD-002: 2 x 100,000.00)
INSERT INTO ap_debit_note_lines (
    ap_debit_note_id,
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
    dn.id,
    p.id,
    'Damaged peripheral accessories discount',
    2.0000,
    100000.0000,
    0.00,
    0.00,
    200000.0000,
    0.0000,
    200000.0000,
    CURRENT_TIMESTAMP
FROM ap_debit_notes dn
CROSS JOIN LATERAL (
    SELECT id FROM products WHERE sku = 'PRD-002' UNION ALL SELECT id FROM products ORDER BY id LIMIT 1
) p
WHERE dn.number = 'DN-SEED-0001'
  AND NOT EXISTS (
      SELECT 1 FROM ap_debit_note_lines l
      WHERE l.ap_debit_note_id = dn.id AND l.description = 'Damaged peripheral accessories discount'
  )
LIMIT 1;

-- 3. Insert AP Debit Note Allocation against the invoice
INSERT INTO ap_debit_note_allocations (
    ap_debit_note_id,
    ap_invoice_id,
    amount,
    created_at
)
SELECT
    dn.id,
    dn.ap_invoice_id,
    dn.total,
    CURRENT_TIMESTAMP
FROM ap_debit_notes dn
WHERE dn.number = 'DN-SEED-0001'
ON CONFLICT (ap_debit_note_id, ap_invoice_id) DO UPDATE SET
    amount = EXCLUDED.amount;

COMMIT;
