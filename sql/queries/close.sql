-- name: ListPeriods :many
SELECT ap.id, ap.period_id, COALESCE(ap.company_id, 0), ap.name, ap.start_date, ap.end_date, ap.status,
       ap.soft_closed_by, ap.soft_closed_at, ap.closed_by, ap.closed_at, ap.metadata, ap.created_at, ap.updated_at,
       COALESCE(lr.id, 0) AS latest_run_id
FROM accounting_periods ap
LEFT JOIN LATERAL (
    SELECT id
    FROM period_close_runs r
    WHERE r.period_id = ap.id
    ORDER BY r.created_at DESC
    LIMIT 1
) lr ON TRUE
WHERE ($1 = 0 OR company_id = $1)
ORDER BY start_date DESC
LIMIT $2 OFFSET $3;

-- name: LoadPeriod :one
SELECT ap.id, ap.period_id, COALESCE(ap.company_id, 0), ap.name, ap.start_date, ap.end_date, ap.status,
       ap.soft_closed_by, ap.soft_closed_at, ap.closed_by, ap.closed_at, ap.metadata, ap.created_at, ap.updated_at,
       COALESCE(lr.id, 0) AS latest_run_id
FROM accounting_periods ap
LEFT JOIN LATERAL (
    SELECT id
    FROM period_close_runs r
    WHERE r.period_id = ap.id
    ORDER BY r.created_at DESC
    LIMIT 1
) lr ON TRUE
WHERE ap.id = $1;

-- name: LoadPeriodByLedgerID :one
SELECT ap.id, ap.period_id, COALESCE(ap.company_id, 0), ap.name, ap.start_date, ap.end_date, ap.status,
       ap.soft_closed_by, ap.soft_closed_at, ap.closed_by, ap.closed_at, ap.metadata, ap.created_at, ap.updated_at,
       COALESCE(lr.id, 0) AS latest_run_id
FROM accounting_periods ap
LEFT JOIN LATERAL (
    SELECT id
    FROM period_close_runs r
    WHERE r.period_id = ap.id
    ORDER BY r.created_at DESC
    LIMIT 1
) lr ON TRUE
WHERE ap.period_id = $1;

-- name: LoadPeriodForUpdate :one
SELECT ap.id, ap.period_id, COALESCE(ap.company_id, 0), ap.name, ap.start_date, ap.end_date, ap.status,
       ap.soft_closed_by, ap.soft_closed_at, ap.closed_by, ap.closed_at, ap.metadata, ap.created_at, ap.updated_at,
       COALESCE(lr.id, 0) AS latest_run_id
FROM accounting_periods ap
LEFT JOIN LATERAL (
    SELECT id
    FROM period_close_runs r
    WHERE r.period_id = ap.id
    ORDER BY r.created_at DESC
    LIMIT 1
) lr ON TRUE
WHERE ap.id = $1
FOR UPDATE;

-- name: InsertPeriodLegacy :one
INSERT INTO periods (code, start_date, end_date, status)
VALUES ($1, $2, $3, $4)
RETURNING id, created_at, updated_at;

-- name: InsertAccountingPeriod :one
INSERT INTO accounting_periods (period_id, company_id, name, start_date, end_date, status, metadata, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING id;

-- name: UpdateAccountingPeriodStatus :exec
UPDATE accounting_periods
SET status = $2,
    soft_closed_by = CASE
        WHEN $2 = 'SOFT_CLOSED' THEN $3
        WHEN $2 = 'OPEN' THEN NULL
        ELSE soft_closed_by
    END,
    soft_closed_at = CASE
        WHEN $2 = 'SOFT_CLOSED' THEN NOW()
        WHEN $2 = 'OPEN' THEN NULL
        ELSE soft_closed_at
    END,
    closed_by = CASE
        WHEN $2 = 'HARD_CLOSED' THEN $3
        WHEN $2 != 'HARD_CLOSED' THEN NULL
        ELSE closed_by
    END,
    closed_at = CASE
        WHEN $2 = 'HARD_CLOSED' THEN NOW()
        WHEN $2 != 'HARD_CLOSED' THEN NULL
        ELSE closed_at
    END,
    updated_at = NOW()
WHERE id = $1;

-- name: UpdateLegacyPeriodStatus :exec
UPDATE periods 
SET status = $2, updated_at = NOW() 
WHERE id = (SELECT period_id FROM accounting_periods WHERE accounting_periods.id = $1);

-- name: PeriodRangeConflict :one
SELECT 1
FROM accounting_periods
WHERE company_id = $1
  AND daterange(start_date, end_date, '[]') && daterange($2, $3, '[]')
LIMIT 1;

-- name: PeriodHasActiveRun :one
SELECT 1 FROM period_close_runs
WHERE period_id = $1 AND status IN ('DRAFT','IN_PROGRESS')
LIMIT 1;

-- name: InsertCloseRun :one
INSERT INTO period_close_runs (company_id, period_id, status, created_by, notes)
VALUES ($1, $2, 'IN_PROGRESS', $3, $4)
RETURNING id, company_id, period_id, status, created_by, created_at, completed_at, notes;

-- name: InsertChecklistItem :one
INSERT INTO period_close_checklist_items (period_close_run_id, code, label)
VALUES ($1, $2, $3)
RETURNING id, period_close_run_id, code, label, status, assigned_to, completed_at, comment, created_at, updated_at;

-- name: LoadCloseRun :one
SELECT id, company_id, period_id, status, created_by, created_at, completed_at, notes
FROM period_close_runs WHERE id = $1;

-- name: LoadCloseRunForUpdate :one
SELECT id, company_id, period_id, status, created_by, created_at, completed_at, notes
FROM period_close_runs WHERE id = $1 FOR UPDATE;

-- name: ListChecklistItems :many
SELECT id, period_close_run_id, code, label, status, assigned_to, completed_at, comment, created_at, updated_at
FROM period_close_checklist_items
WHERE period_close_run_id = $1
ORDER BY id;

-- name: UpdateChecklistStatus :one
UPDATE period_close_checklist_items
SET status = $2,
    comment = COALESCE(NULLIF($3,''), comment),
    completed_at = CASE WHEN $2 IN ('DONE','SKIPPED') THEN NOW() ELSE NULL END,
    updated_at = NOW()
WHERE id = $1
RETURNING id, period_close_run_id, code, label, status, assigned_to, completed_at, comment, created_at, updated_at;

-- name: LockChecklistItemRun :one
SELECT period_close_run_id FROM period_close_checklist_items WHERE id = $1 FOR UPDATE;

-- name: CountPendingChecklistItems :one
SELECT COUNT(*) 
FROM period_close_checklist_items
WHERE period_close_run_id = $1 AND status NOT IN ('DONE','SKIPPED');

-- name: UpdateRunStatus :exec
UPDATE period_close_runs
SET status = $2,
    completed_at = CASE WHEN $2 = 'COMPLETED' THEN NOW() ELSE completed_at END,
    updated_at = NOW()
WHERE id = $1;
