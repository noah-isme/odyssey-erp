-- name: ListTemplates :many
SELECT id, name, COALESCE(description,'') as description, sections, is_default, is_active, created_by, created_at, updated_at
FROM board_pack_templates
WHERE ($1::boolean = true OR is_active)
ORDER BY is_default DESC, name;

-- name: GetTemplate :one
SELECT id, name, COALESCE(description,'') as description, sections, is_default, is_active, created_by, created_at, updated_at
FROM board_pack_templates
WHERE id = $1;

-- name: InsertBoardPack :one
INSERT INTO board_packs (company_id, period_id, template_id, variance_snapshot_id, status, generated_by, metadata)
VALUES ($1,$2,$3,$4,'PENDING',$5,$6)
RETURNING id;

-- name: GetBoardPack :one
SELECT
    bp.id,
    bp.company_id,
    COALESCE(c.name,'') as company_name,
    COALESCE(c.code,'') as company_code,
    bp.period_id,
    ap.name as period_name,
    ap.start_date as period_start,
    ap.end_date as period_end,
    ap.status as period_status,
    bp.template_id,
    tpl.name as template_name,
    tpl.description as template_description,
    tpl.sections as template_sections,
    tpl.is_default as template_is_default,
    tpl.is_active as template_is_active,
    tpl.created_by as template_created_by,
    tpl.created_at as template_created_at,
    tpl.updated_at as template_updated_at,
    bp.variance_snapshot_id,
    bp.status,
    bp.generated_at,
    bp.generated_by,
    COALESCE(bp.file_path,'') as file_path,
    bp.file_size,
    bp.page_count,
    COALESCE(bp.error_message,'') as error_message,
    bp.metadata,
    bp.created_at,
    bp.updated_at
FROM board_packs bp
JOIN companies c ON c.id = bp.company_id
JOIN accounting_periods ap ON ap.id = bp.period_id
JOIN board_pack_templates tpl ON tpl.id = bp.template_id
WHERE bp.id = $1;

-- name: ListBoardPacks :many
SELECT
    bp.id,
    bp.company_id,
    COALESCE(c.name,'') as company_name,
    COALESCE(c.code,'') as company_code,
    bp.period_id,
    ap.name as period_name,
    ap.start_date as period_start,
    ap.end_date as period_end,
    ap.status as period_status,
    bp.template_id,
    tpl.name as template_name,
    tpl.description as template_description,
    tpl.sections as template_sections,
    tpl.is_default as template_is_default,
    tpl.is_active as template_is_active,
    tpl.created_by as template_created_by,
    tpl.created_at as template_created_at,
    tpl.updated_at as template_updated_at,
    bp.variance_snapshot_id,
    bp.status,
    bp.generated_at,
    bp.generated_by,
    COALESCE(bp.file_path,'') as file_path,
    bp.file_size,
    bp.page_count,
    COALESCE(bp.error_message,'') as error_message,
    bp.metadata,
    bp.created_at,
    bp.updated_at
FROM board_packs bp
JOIN companies c ON c.id = bp.company_id
JOIN accounting_periods ap ON ap.id = bp.period_id
JOIN board_pack_templates tpl ON tpl.id = bp.template_id
WHERE ($1::bigint = 0 OR bp.company_id = $1)
  AND ($2::bigint = 0 OR bp.period_id = $2)
  AND ($3::text = '' OR bp.status::TEXT = $3)
ORDER BY bp.created_at DESC
LIMIT $4 OFFSET $5;

-- name: MarkInProgress :exec
UPDATE board_packs
SET status = 'IN_PROGRESS', error_message = NULL, updated_at = NOW()
WHERE id = $1 AND status = 'PENDING';

-- name: MarkReady :exec
UPDATE board_packs
SET status = 'READY', file_path = $2, file_size = $3, page_count = $4, metadata = $5,
    generated_at = $6, updated_at = NOW()
WHERE id = $1;

-- name: MarkFailed :exec
UPDATE board_packs SET status = 'FAILED', error_message = $2, updated_at = NOW() WHERE id = $1;

-- name: ListCompanies :many
SELECT id, code, name FROM companies ORDER BY name;

-- name: GetCompany :one
SELECT id, code, name FROM companies WHERE id = $1;

-- name: GetPeriod :one
SELECT id, name, start_date, end_date, status, COALESCE(company_id, 0) as company_id
FROM accounting_periods WHERE id = $1;

-- name: ListRecentPeriods :many
SELECT id, name, start_date, end_date, status, COALESCE(company_id, 0) as company_id
FROM accounting_periods
WHERE ($1::bigint = 0)
   OR company_id = $1
   OR company_id IS NULL
ORDER BY start_date DESC
LIMIT $2;

-- name: ListVarianceSnapshots :many
SELECT vs.id, COALESCE(vr.name,'') as rule_name, vs.period_id, vr.company_id, vs.status
FROM variance_snapshots vs
JOIN variance_rules vr ON vr.id = vs.rule_id
WHERE vs.status = 'READY' AND ($1::bigint = 0 OR vr.company_id = $1)
ORDER BY vs.updated_at DESC
LIMIT $2;

-- name: GetVarianceSnapshot :one
SELECT vs.id, COALESCE(vr.name,'') as rule_name, vs.period_id, vr.company_id, vs.status
FROM variance_snapshots vs
JOIN variance_rules vr ON vr.id = vs.rule_id
WHERE vs.id = $1;

-- name: AggregateAccountBalances :many
WITH target_period AS (
    SELECT ap.id, ap.start_date, ap.end_date FROM accounting_periods ap WHERE ap.id = $2
)
SELECT acc.code, acc.name, acc.type,
       COALESCE(SUM(CASE WHEN je.date < tp.start_date THEN (jl.debit - jl.credit) ELSE 0 END),0)::float8 AS opening,
       COALESCE(SUM(CASE WHEN je.date BETWEEN tp.start_date AND tp.end_date THEN jl.debit ELSE 0 END),0)::float8 AS debit,
       COALESCE(SUM(CASE WHEN je.date BETWEEN tp.start_date AND tp.end_date THEN jl.credit ELSE 0 END),0)::float8 AS credit
FROM accounts acc
JOIN journal_lines jl ON jl.account_id = acc.id
JOIN journal_entries je ON je.id = jl.je_id AND je.status = 'POSTED'
JOIN target_period tp ON TRUE
WHERE COALESCE(jl.dim_company_id, 0) = $1 AND je.date <= tp.end_date
GROUP BY acc.code, acc.name, acc.type
HAVING COALESCE(SUM(CASE WHEN je.date < tp.start_date THEN (jl.debit - jl.credit) ELSE 0 END),0) <> 0
    OR COALESCE(SUM(CASE WHEN je.date BETWEEN tp.start_date AND tp.end_date THEN (jl.debit - jl.credit) ELSE 0 END),0) <> 0
ORDER BY acc.code;
