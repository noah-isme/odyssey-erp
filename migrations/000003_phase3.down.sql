DROP INDEX IF EXISTS idx_audit_logs_module;

DROP TABLE IF EXISTS idempotency_keys;
DROP INDEX IF EXISTS idx_approvals_ref;
DROP TABLE IF EXISTS approvals;

DROP INDEX IF EXISTS idx_ap_payments_invoice;
DROP TABLE IF EXISTS ap_payments;
DROP TABLE IF EXISTS ap_invoices;

DROP INDEX IF EXISTS idx_grn_lines_grn;
DROP TABLE IF EXISTS grn_lines;
DROP TABLE IF EXISTS grns;

DROP INDEX IF EXISTS idx_po_lines_po;
DROP TABLE IF EXISTS po_lines;
DROP TABLE IF EXISTS pos;

DROP INDEX IF EXISTS idx_pr_lines_pr;
DROP TABLE IF EXISTS pr_lines;
DROP TABLE IF EXISTS prs;

DROP INDEX IF EXISTS idx_inventory_cards_lookup;
DROP TABLE IF EXISTS inventory_cards;

DROP TABLE IF EXISTS inventory_balances;

DROP INDEX IF EXISTS idx_inventory_tx_lines_src_dst;
DROP INDEX IF EXISTS idx_inventory_tx_lines_product;
DROP INDEX IF EXISTS idx_inventory_tx_lines_tx;
DROP TABLE IF EXISTS inventory_tx_lines;
DROP TABLE IF EXISTS inventory_tx;
