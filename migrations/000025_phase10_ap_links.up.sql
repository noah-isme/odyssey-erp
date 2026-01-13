-- Phase 10 AP: invoice PO link + payment supplier reference

ALTER TABLE ap_invoices
    ADD COLUMN IF NOT EXISTS po_id BIGINT REFERENCES pos(id) ON DELETE SET NULL;

ALTER TABLE ap_payments
    ADD COLUMN IF NOT EXISTS supplier_id BIGINT REFERENCES suppliers(id) ON DELETE SET NULL;

ALTER TABLE ap_payments
    ALTER COLUMN ap_invoice_id DROP NOT NULL;

CREATE INDEX IF NOT EXISTS idx_ap_invoices_po ON ap_invoices(po_id);
CREATE INDEX IF NOT EXISTS idx_ap_payments_supplier ON ap_payments(supplier_id);

UPDATE ap_invoices i
SET po_id = g.po_id
FROM grns g
WHERE i.grn_id = g.id
  AND i.po_id IS NULL;

UPDATE ap_payments p
SET supplier_id = i.supplier_id
FROM ap_invoices i
WHERE p.ap_invoice_id = i.id
  AND p.supplier_id IS NULL;
