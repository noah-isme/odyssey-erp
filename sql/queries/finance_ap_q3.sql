-- name: CreateAPException :one
INSERT INTO ap_exceptions (
    ap_invoice_id, ap_matching_run_id, exception_type, severity, status, 
    owner_id, sla_due_at, reason, evidence, comments
) VALUES (
    $1, $2, $3, $4, $5, 
    $6, $7, $8, $9, $10
) RETURNING id;

-- name: UpdateAPExceptionStatus :exec
-- Legacy non-terminal state update. Terminal transitions must use
-- ResolveAPException so immutable resolution evidence cannot be bypassed.
UPDATE ap_exceptions
SET 
    status = $2,
    resolved_at = CASE WHEN $2 IN ('RESOLVED', 'REJECTED') THEN NOW() ELSE resolved_at END,
    resolved_by = CASE WHEN $2 IN ('RESOLVED', 'REJECTED') THEN $3 ELSE resolved_by END,
    updated_at = NOW()
WHERE id = $1
  AND status NOT IN ('RESOLVED', 'REJECTED')
  AND $2 NOT IN ('RESOLVED', 'REJECTED');

-- name: GetAPException :one
SELECT 
    id, ap_invoice_id, ap_matching_run_id, exception_type, severity, status,
    owner_id, sla_due_at, reason, evidence, comments,
    created_at, updated_at, resolved_at, resolved_by
FROM ap_exceptions
WHERE id = $1;

-- name: GetAPExceptionForCompany :one
-- The company is derived from the invoice first, with the supplier fallback
-- retained for invoices created before company_id was populated.
SELECT
    e.id, e.ap_invoice_id, e.ap_matching_run_id, e.exception_type, e.severity, e.status,
    e.owner_id, e.sla_due_at, e.reason, e.evidence, e.comments,
    e.created_at, e.updated_at, e.resolved_at, e.resolved_by
FROM ap_exceptions e
JOIN ap_invoices i ON i.id = e.ap_invoice_id
JOIN suppliers s ON s.id = i.supplier_id
WHERE e.id = sqlc.arg(exception_id)
  AND COALESCE(i.company_id, s.company_id)::BIGINT = sqlc.arg(company_id)::BIGINT;

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

-- name: ListAPExceptionsForCompany :many
-- Scope before applying workbench filters so pagination cannot leak or skip
-- records from another company.
SELECT
    e.id, e.ap_invoice_id, e.ap_matching_run_id, e.exception_type, e.severity, e.status,
    e.owner_id, e.sla_due_at, e.reason, e.evidence, e.comments,
    e.created_at, e.updated_at, e.resolved_at, e.resolved_by
FROM ap_exceptions e
JOIN ap_invoices i ON i.id = e.ap_invoice_id
JOIN suppliers s ON s.id = i.supplier_id
WHERE COALESCE(i.company_id, s.company_id)::BIGINT = sqlc.arg(company_id)::BIGINT
  AND (sqlc.arg(status)::TEXT = '' OR e.status = sqlc.arg(status)::TEXT)
  AND (sqlc.arg(owner_id)::BIGINT = 0 OR e.owner_id = sqlc.arg(owner_id)::BIGINT)
  AND (sqlc.arg(invoice_id)::BIGINT = 0 OR e.ap_invoice_id = sqlc.arg(invoice_id)::BIGINT)
ORDER BY e.created_at DESC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: UpdateAPExceptionStatusForCompany :execrows
-- Legacy non-terminal state update. Terminal transitions must use the
-- company-scoped resolver so immutable resolution evidence cannot be bypassed.
UPDATE ap_exceptions AS e
SET
    status = sqlc.arg(status),
    resolved_at = CASE WHEN sqlc.arg(status)::TEXT IN ('RESOLVED', 'REJECTED') THEN NOW() ELSE resolved_at END,
    resolved_by = CASE WHEN sqlc.arg(status)::TEXT IN ('RESOLVED', 'REJECTED') THEN sqlc.arg(resolved_by) ELSE resolved_by END,
    updated_at = NOW()
FROM ap_invoices i
JOIN suppliers s ON s.id = i.supplier_id
WHERE e.id = sqlc.arg(exception_id)
  AND i.id = e.ap_invoice_id
  AND e.status NOT IN ('RESOLVED', 'REJECTED')
  AND sqlc.arg(status)::TEXT NOT IN ('RESOLVED', 'REJECTED')
  AND COALESCE(i.company_id, s.company_id)::BIGINT = sqlc.arg(company_id)::BIGINT;

-- name: ResolveAPException :one
-- Lock the current exception, apply one terminal transition, and append its
-- resolution evidence in the same statement/transaction. A terminal row is
-- deliberately not transitioned a second time.
WITH target AS MATERIALIZED (
    SELECT e.id,
           e.status AS from_status,
           COALESCE(i.company_id, s.company_id)::BIGINT AS company_id
    FROM ap_exceptions e
    JOIN ap_invoices i ON i.id = e.ap_invoice_id
    JOIN suppliers s ON s.id = i.supplier_id
    WHERE e.id = sqlc.arg(exception_id)
    -- Lock the ownership sources with the exception so the tenant snapshot
    -- cannot race a concurrent invoice/supplier company reassignment.
    FOR UPDATE OF e, i, s
), transitioned AS (
    UPDATE ap_exceptions AS e
    SET
        status = sqlc.arg(to_status)::TEXT,
        resolved_at = NOW(),
        resolved_by = sqlc.arg(resolved_by)::BIGINT,
        updated_at = NOW()
    FROM target
    WHERE e.id = target.id
      AND target.from_status IN ('OPEN', 'IN_REVIEW')
    RETURNING e.id
)
INSERT INTO ap_exception_resolution_events (
    ap_exception_id, company_id, from_status, to_status, comment, actor_id
)
SELECT target.id,
       target.company_id,
       target.from_status,
       sqlc.arg(to_status)::TEXT,
       sqlc.arg(comment)::TEXT,
       sqlc.arg(resolved_by)::BIGINT
FROM target
JOIN transitioned ON transitioned.id = target.id
RETURNING id;

-- name: ResolveAPExceptionForCompany :one
-- The company predicate is applied while acquiring the row lock; a
-- cross-company ID therefore cannot be updated or generate an event.
WITH target AS MATERIALIZED (
    SELECT e.id,
           e.status AS from_status,
           COALESCE(i.company_id, s.company_id)::BIGINT AS company_id
    FROM ap_exceptions e
    JOIN ap_invoices i ON i.id = e.ap_invoice_id
    JOIN suppliers s ON s.id = i.supplier_id
    WHERE e.id = sqlc.arg(exception_id)
      AND COALESCE(i.company_id, s.company_id)::BIGINT = sqlc.arg(company_id)::BIGINT
    -- Lock the ownership sources with the exception so the tenant snapshot
    -- cannot race a concurrent invoice/supplier company reassignment.
    FOR UPDATE OF e, i, s
), transitioned AS (
    UPDATE ap_exceptions AS e
    SET
        status = sqlc.arg(to_status)::TEXT,
        resolved_at = NOW(),
        resolved_by = sqlc.arg(resolved_by)::BIGINT,
        updated_at = NOW()
    FROM target
    WHERE e.id = target.id
      AND target.from_status IN ('OPEN', 'IN_REVIEW')
    RETURNING e.id
)
INSERT INTO ap_exception_resolution_events (
    ap_exception_id, company_id, from_status, to_status, comment, actor_id
)
SELECT target.id,
       target.company_id,
       target.from_status,
       sqlc.arg(to_status)::TEXT,
       sqlc.arg(comment)::TEXT,
       sqlc.arg(resolved_by)::BIGINT
FROM target
JOIN transitioned ON transitioned.id = target.id
RETURNING id;

-- name: ListAPExceptionResolutionEventsForCompany :many
SELECT
    id, ap_exception_id, company_id, from_status, to_status, comment,
    actor_id, created_at
FROM ap_exception_resolution_events
WHERE company_id = sqlc.arg(company_id)::BIGINT
  AND ap_exception_id = sqlc.arg(exception_id)
ORDER BY created_at ASC, id ASC;

-- name: GetLatestMatchingRun :one
SELECT 
    id, ap_invoice_id, policy_id, status,
    invoice_total, po_total, grn_total,
    reasons, action_recommended, run_at, run_by
FROM ap_matching_runs
WHERE ap_invoice_id = $1
ORDER BY run_at DESC
LIMIT 1;
