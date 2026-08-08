-- Q1 P2P Foundation Schema

-- 1. Service PO support
ALTER TABLE pos ADD COLUMN IF NOT EXISTS is_service BOOLEAN NOT NULL DEFAULT false;

-- 2. AP Invoice enhancements
ALTER TABLE ap_invoice_lines ADD COLUMN IF NOT EXISTS po_line_id BIGINT REFERENCES po_lines(id) ON DELETE SET NULL;
ALTER TABLE ap_invoices ADD COLUMN IF NOT EXISTS supplier_document_number TEXT;
ALTER TABLE ap_invoices ADD COLUMN IF NOT EXISTS attachment_hash TEXT;
ALTER TABLE ap_invoices ADD COLUMN IF NOT EXISTS duplicate_status TEXT NOT NULL DEFAULT 'OK' CHECK (duplicate_status IN ('OK', 'REVIEW_NEEDED', 'DUPLICATE_REJECTED'));

CREATE UNIQUE INDEX idx_ap_invoices_supplier_doc 
ON ap_invoices(supplier_id, supplier_document_number) 
WHERE supplier_document_number IS NOT NULL AND status != 'VOID';

-- 3. PO Line Progress View
-- Computes the deterministic, real-time aggregate of quantities and amounts across the P2P lifecycle
CREATE OR REPLACE VIEW po_line_progress AS
SELECT 
    pol.id AS po_line_id,
    pol.po_id,
    pol.product_id,
    pol.qty AS ordered_qty,
    pol.price AS unit_price,
    (pol.qty * pol.price) AS ordered_amount,
    
    COALESCE(grn_agg.received_qty, 0) AS received_qty,
    COALESCE(ap_agg.invoiced_qty, 0) AS invoiced_qty,
    COALESCE(ap_agg.invoiced_amount, 0) AS invoiced_amount,
    COALESCE(ap_agg.tax_amount, 0) AS invoiced_tax,
    COALESCE(pay_agg.paid_amount, 0) AS paid_amount
FROM po_lines pol
LEFT JOIN (
    SELECT 
        gl.product_id, -- assuming GRN links back to PO via GRN header, but grn_lines don't have po_line_id
        g.po_id,
        SUM(gl.qty) AS received_qty
    FROM grn_lines gl
    JOIN grns g ON gl.grn_id = g.id
    WHERE g.status = 'POSTED'
    GROUP BY gl.product_id, g.po_id
) grn_agg ON grn_agg.po_id = pol.po_id AND grn_agg.product_id = pol.product_id
LEFT JOIN (
    SELECT 
        COALESCE(al.po_line_id, 
            (SELECT pl2.id FROM po_lines pl2 
             JOIN grns g2 ON g2.po_id = pl2.po_id
             JOIN grn_lines gl2 ON gl2.grn_id = g2.id AND gl2.product_id = pl2.product_id
             WHERE gl2.id = al.grn_line_id LIMIT 1)
        ) AS po_line_id,
        SUM(al.quantity) AS invoiced_qty,
        SUM(al.subtotal) AS invoiced_amount,
        SUM(al.tax_amount) AS tax_amount
    FROM ap_invoice_lines al
    JOIN ap_invoices ai ON al.ap_invoice_id = ai.id
    WHERE ai.status IN ('POSTED', 'PAID')
    GROUP BY 1
) ap_agg ON ap_agg.po_line_id = pol.id
LEFT JOIN (
    -- Payment allocations usually link to invoice, not invoice lines.
    -- To map payment back to PO line, we proportion it or just use the Invoice-to-PO link.
    -- For precise line-level paid_amount without line-level payment allocation, we may approximate 
    -- or require line-level allocations. Here, we'll proportion based on invoiced amount.
    SELECT 
        COALESCE(al2.po_line_id, 
            (SELECT pl2.id FROM po_lines pl2 
             JOIN grns g2 ON g2.po_id = pl2.po_id
             JOIN grn_lines gl2 ON gl2.grn_id = g2.id AND gl2.product_id = pl2.product_id
             WHERE gl2.id = al2.grn_line_id LIMIT 1)
        ) AS po_line_id,
        SUM(pa.amount * (al2.total / NULLIF(ai2.total, 0))) AS paid_amount
    FROM ap_payment_allocations pa
    JOIN ap_invoices ai2 ON pa.ap_invoice_id = ai2.id
    JOIN ap_invoice_lines al2 ON al2.ap_invoice_id = ai2.id
    GROUP BY 1
) pay_agg ON pay_agg.po_line_id = pol.id;
