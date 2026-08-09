-- name: CalculateOTIFScore :one
SELECT COUNT(*) as total_receipts,
       COALESCE(SUM(CASE WHEN g.received_at::date <= p.expected_date THEN 1 ELSE 0 END), 0)::BIGINT as ontime_receipts
FROM grns g
JOIN pos p ON p.id = g.po_id
WHERE g.company_id = $1 AND g.supplier_id = $2
  AND g.status = 'POSTED'
  AND p.expected_date IS NOT NULL
  AND g.received_at >= $3 AND g.received_at <= $4;

-- name: CalculateQualityScore :one
SELECT COALESCE(SUM(gl.qty - COALESCE(returned.quantity_returned, 0)), 0)::BIGINT as accepted_qty,
       COALESCE(SUM(COALESCE(returned.quantity_returned, 0)), 0)::BIGINT as rejected_qty
FROM grn_lines gl
JOIN grns g ON g.id = gl.grn_id
LEFT JOIN (
    SELECT rgl.grn_line_id, SUM(rgl.quantity_returned) AS quantity_returned
    FROM goods_return_grn_lines rgl
    JOIN goods_return_grns r ON r.id = rgl.goods_return_grn_id
    WHERE r.status = 'CONFIRMED'
    GROUP BY rgl.grn_line_id
) returned ON returned.grn_line_id = gl.id
WHERE g.company_id = $1 AND g.supplier_id = $2
  AND g.status = 'POSTED'
  AND g.received_at >= $3 AND g.received_at <= $4;

-- name: CalculatePriceAdherenceScore :one
SELECT COUNT(*) as total_pos,
       COALESCE(SUM(CASE WHEN NOT EXISTS (
           SELECT 1
           FROM po_contract_variances pending
           WHERE pending.po_line_id = pl.id
             AND pending.approval_status = 'PENDING'
       ) THEN 1 ELSE 0 END), 0)::BIGINT as compliant_pos
FROM po_lines pl
JOIN pos p ON p.id = pl.po_id
WHERE p.company_id = $1 AND p.supplier_id = $2
  AND p.status <> 'CANCELLED'
  AND p.created_at >= $3 AND p.created_at <= $4;

-- name: CalculateRFQResponsivenessScore :one
SELECT COUNT(*) as total_rfqs,
       COALESCE(SUM(CASE WHEN rb.id IS NOT NULL THEN 1 ELSE 0 END), 0)::BIGINT as responded_rfqs
FROM rfq_suppliers rs
JOIN rfqs r ON r.id = rs.rfq_id
LEFT JOIN rfq_bids rb ON rb.rfq_id = r.id AND rb.supplier_id = rs.supplier_id AND rb.status = 'SUBMITTED'
WHERE r.company_id = $1 AND rs.supplier_id = $2 AND r.created_at >= $3 AND r.created_at <= $4;
