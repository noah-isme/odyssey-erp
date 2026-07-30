-- P1 Phase B rollback

DROP TRIGGER IF EXISTS trg_update_ap_invoice_status_on_debit_note ON ap_debit_note_allocations;
DROP FUNCTION IF EXISTS update_ap_invoice_status_on_debit_note();

DROP TRIGGER IF EXISTS trg_ap_debit_notes_updated_at ON ap_debit_notes;
DROP TRIGGER IF EXISTS trg_goods_return_grn_lines_updated_at ON goods_return_grn_lines;
DROP TRIGGER IF EXISTS trg_goods_return_grns_updated_at ON goods_return_grns;
DROP TRIGGER IF EXISTS trg_goods_return_grn_quantity ON goods_return_grn_lines;
DROP FUNCTION IF EXISTS enforce_goods_return_grn_quantity();

DROP TABLE IF EXISTS ap_debit_note_allocations CASCADE;
DROP TABLE IF EXISTS ap_debit_note_lines CASCADE;
DROP TABLE IF EXISTS ap_debit_notes CASCADE;
DROP TABLE IF EXISTS goods_return_grn_lines CASCADE;
DROP TABLE IF EXISTS goods_return_grns CASCADE;

DROP TYPE IF EXISTS ap_debit_note_status;
DROP TYPE IF EXISTS goods_return_grn_status;

DROP FUNCTION IF EXISTS generate_ap_debit_note_number();
DROP FUNCTION IF EXISTS generate_goods_return_grn_number();

-- Restore the simple AP invoice balance view (without debit notes)
CREATE OR REPLACE VIEW v_ap_invoice_balance AS
SELECT
    i.id,
    i.number,
    i.supplier_id,
    i.grn_id,
    i.subtotal,
    i.tax_amount,
    i.total,
    COALESCE(pa.paid_amount, 0) AS paid_amount,
    GREATEST(i.total - COALESCE(pa.paid_amount, 0), 0) AS balance,
    i.status,
    i.due_at,
    i.created_at
FROM ap_invoices i
LEFT JOIN (
    SELECT ap_invoice_id, SUM(amount) AS paid_amount
    FROM ap_payment_allocations
    GROUP BY ap_invoice_id
) pa ON pa.ap_invoice_id = i.id;

-- Remove RBAC permissions
DELETE FROM role_permissions WHERE permission_id IN (
    SELECT id FROM permissions WHERE name IN (
        'procurement.return.view', 'procurement.return.create',
        'procurement.return.post', 'procurement.return.void',
        'finance.ap.debit_note.view', 'finance.ap.debit_note.create',
        'finance.ap.debit_note.post', 'finance.ap.debit_note.void'
    )
);
DELETE FROM permissions WHERE name IN (
    'procurement.return.view', 'procurement.return.create',
    'procurement.return.post', 'procurement.return.void',
    'finance.ap.debit_note.view', 'finance.ap.debit_note.create',
    'finance.ap.debit_note.post', 'finance.ap.debit_note.void'
);

-- Remove account mappings
DELETE FROM account_mappings WHERE module = 'AP' AND key IN (
    'ap.debit_note.ap', 'ap.debit_note.inventory',
    'ap.debit_note.expense', 'ap.debit_note.tax'
);
