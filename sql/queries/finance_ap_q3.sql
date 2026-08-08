-- name: CreateAPException :one
INSERT INTO ap_exceptions (
    ap_invoice_id, ap_matching_run_id, exception_type, severity, status, 
    owner_id, sla_due_at, reason, evidence, comments
) VALUES (
    $1, $2, $3, $4, $5, 
    $6, $7, $8, $9, $10
) RETURNING id;

-- name: UpdateAPExceptionStatus :exec
UPDATE ap_exceptions
SET 
    status = $2,
    resolved_at = CASE WHEN $2 IN ('RESOLVED', 'REJECTED') THEN NOW() ELSE resolved_at END,
    resolved_by = CASE WHEN $2 IN ('RESOLVED', 'REJECTED') THEN $3 ELSE resolved_by END,
    updated_at = NOW()
WHERE id = $1;

-- name: GetAPException :one
SELECT 
    id, ap_invoice_id, ap_matching_run_id, exception_type, severity, status,
    owner_id, sla_due_at, reason, evidence, comments,
    created_at, updated_at, resolved_at, resolved_by
FROM ap_exceptions
WHERE id = $1;

-- name: ListAPExceptions :many
SELECT 
    id, ap_invoice_id, ap_matching_run_id, exception_type, severity, status,
    owner_id, sla_due_at, reason, evidence, comments,
    created_at, updated_at, resolved_at, resolved_by
FROM ap_exceptions
WHERE 
    ($1::TEXT = '' OR status = $1)
    AND ($2::BIGINT = 0 OR owner_id = $2)
    AND ($3::BIGINT = 0 OR ap_invoice_id = $3)
ORDER BY created_at DESC
LIMIT $4 OFFSET $5;

-- name: GetLatestMatchingRun :one
SELECT 
    id, ap_invoice_id, policy_id, status,
    invoice_total, po_total, grn_total,
    reasons, action_recommended, run_at, run_by
FROM ap_matching_runs
WHERE ap_invoice_id = $1
ORDER BY run_at DESC
LIMIT 1;
