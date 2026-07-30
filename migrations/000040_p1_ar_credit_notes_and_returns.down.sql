-- Revert P1 Phase A

DROP TRIGGER IF EXISTS trg_ar_credit_notes_updated_at ON ar_credit_notes;
DROP TRIGGER IF EXISTS trg_return_delivery_orders_updated_at ON return_delivery_orders;
DROP TRIGGER IF EXISTS trg_return_delivery_order_lines_updated_at ON return_delivery_order_lines;
DROP TRIGGER IF EXISTS trg_update_invoice_status_on_credit_note ON ar_credit_note_allocations;

DROP FUNCTION IF EXISTS update_invoice_status_on_credit_note_allocation();

DROP VIEW IF EXISTS v_ar_invoice_balance;
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

DROP TABLE IF EXISTS ar_credit_note_allocations;
DROP TABLE IF EXISTS ar_credit_note_lines;
DROP TABLE IF EXISTS ar_credit_notes;
DROP TYPE IF EXISTS ar_credit_note_status;

DROP TABLE IF EXISTS return_delivery_order_lines;
DROP FUNCTION IF EXISTS enforce_return_delivery_quantity();
DROP TABLE IF EXISTS return_delivery_orders;
DROP TYPE IF EXISTS return_delivery_order_status;

DROP FUNCTION IF EXISTS generate_ar_credit_note_number();
DROP FUNCTION IF EXISTS generate_return_delivery_order_number(BIGINT, DATE);

DELETE FROM role_permissions WHERE permission_id IN (
    SELECT id FROM permissions
    WHERE name IN (
        'finance.ar.credit_note.view', 'finance.ar.credit_note.create',
        'finance.ar.credit_note.post', 'finance.ar.credit_note.void',
        'delivery.return.view', 'delivery.return.create',
        'delivery.return.post', 'delivery.return.void'
    )
);

DELETE FROM permissions WHERE name IN (
    'finance.ar.credit_note.view', 'finance.ar.credit_note.create',
    'finance.ar.credit_note.post', 'finance.ar.credit_note.void',
    'delivery.return.view', 'delivery.return.create',
    'delivery.return.post', 'delivery.return.void'
);

DELETE FROM account_mappings WHERE module = 'AR' AND key IN (
    'ar.invoice.ar', 'ar.invoice.revenue', 'ar.invoice.tax', 'ar.return.cogs'
);

DELETE FROM accounts WHERE code = '4160' AND name = 'Tax Output';
