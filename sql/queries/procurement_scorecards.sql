-- name: CalculateOTIFScore :one
SELECT COUNT(*) as total_receipts,
       SUM(CASE WHEN received_at <= expected_date THEN 1 ELSE 0 END) as ontime_receipts
FROM grns
WHERE company_id = $1 AND supplier_id = $2 AND received_at >= $3 AND received_at <= $4;

-- name: CalculateQualityScore :one
SELECT SUM(CASE WHEN gl.status='ACCEPTED' THEN gl.quantity ELSE 0 END) as accepted_qty,
       SUM(CASE WHEN gl.status IN ('REJECTED','RETURNED') THEN gl.quantity ELSE 0 END) as rejected_qty
FROM grn_lines gl
JOIN grns g ON g.id = gl.grn_id
WHERE g.company_id = $1 AND g.supplier_id = $2 AND g.received_at >= $3 AND g.received_at <= $4;

-- name: CalculatePriceAdherenceScore :one
SELECT COUNT(*) as total_pos,
       SUM(CASE WHEN pcv.variance_type IS NULL THEN 1 ELSE 0 END) as compliant_pos
FROM po_lines pl
JOIN pos p ON p.id = pl.po_id
LEFT JOIN po_contract_variances pcv ON pcv.po_line_id = pl.id AND pcv.approval_status='PENDING'
WHERE p.company_id = $1 AND p.supplier_id = $2 AND p.created_at >= $3 AND p.created_at <= $4;

-- name: CalculateRFQResponsivenessScore :one
SELECT COUNT(*) as total_rfqs,
       SUM(CASE WHEN rb.id IS NOT NULL THEN 1 ELSE 0 END) as responded_rfqs
FROM rfq_suppliers rs
JOIN rfqs r ON r.id = rs.rfq_id
LEFT JOIN rfq_bids rb ON rb.rfq_id = r.id AND rb.supplier_id = rs.supplier_id
WHERE r.company_id = $1 AND rs.supplier_id = $2 AND r.created_at >= $3 AND r.created_at <= $4;
