-- name: GetActiveMatchingPolicy :one
SELECT 
    id, name, company_id, supplier_id, category_id,
    qty_tolerance_pct, price_tolerance_pct, tax_tolerance_pct,
    freight_tolerance_pct, total_tolerance_amt,
    effective_from, effective_to, created_at, updated_at
FROM ap_matching_policies
WHERE 
    (effective_from <= CURRENT_DATE) AND (effective_to IS NULL OR effective_to >= CURRENT_DATE)
    AND (company_id = $1 OR company_id IS NULL)
    AND (supplier_id = $2 OR supplier_id IS NULL)
ORDER BY 
    company_id DESC NULLS LAST,
    supplier_id DESC NULLS LAST,
    category_id DESC NULLS LAST,
    effective_from DESC
LIMIT 1;

-- name: CreateMatchingPolicy :one
INSERT INTO ap_matching_policies (
    name, company_id, supplier_id, category_id,
    qty_tolerance_pct, price_tolerance_pct, tax_tolerance_pct,
    freight_tolerance_pct, total_tolerance_amt,
    effective_from, effective_to
) VALUES (
    $1, $2, $3, $4, 
    $5, $6, $7, 
    $8, $9, 
    $10, $11
) RETURNING id;

-- name: CreateMatchingRun :one
INSERT INTO ap_matching_runs (
    ap_invoice_id, policy_id, status, 
    invoice_total, po_total, grn_total,
    reasons, action_recommended, run_by
) VALUES (
    $1, $2, $3, 
    $4, $5, $6,
    $7, $8, $9
) RETURNING id;

-- name: CreateMatchingRunLine :one
INSERT INTO ap_matching_run_lines (
    ap_matching_run_id, ap_invoice_line_id, po_line_id, grn_line_id,
    invoice_qty, invoice_price, po_qty, po_price, grn_qty,
    status, reasons
) VALUES (
    $1, $2, $3, $4,
    $5, $6, $7, $8, $9,
    $10, $11
) RETURNING id;

-- name: GetMatchingRun :one
SELECT 
    id, ap_invoice_id, policy_id, status,
    invoice_total, po_total, grn_total,
    reasons, action_recommended, run_at, run_by
FROM ap_matching_runs
WHERE id = $1;

-- name: ListMatchingRunLines :many
SELECT 
    id, ap_matching_run_id, ap_invoice_line_id, po_line_id, grn_line_id,
    invoice_qty, invoice_price, po_qty, po_price, grn_qty,
    status, reasons
FROM ap_matching_run_lines
WHERE ap_matching_run_id = $1;
