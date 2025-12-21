-- name: ElimListRules :many
SELECT id, group_id, name, source_company_id, target_company_id, account_src, account_tgt, match_criteria,
       is_active, created_by, created_at, updated_at
FROM elimination_rules
ORDER BY created_at DESC
LIMIT $1;

-- name: ElimInsertRule :one
INSERT INTO elimination_rules (group_id, name, source_company_id, target_company_id, account_src, account_tgt, match_criteria, created_by)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
RETURNING id, group_id, name, source_company_id, target_company_id, account_src, account_tgt, match_criteria,
          is_active, created_by, created_at, updated_at;

-- name: ElimGetRule :one
SELECT id, group_id, name, source_company_id, target_company_id, account_src, account_tgt, match_criteria,
       is_active, created_by, created_at, updated_at
FROM elimination_rules WHERE id = $1;

-- name: CountRuns :one
SELECT COUNT(*) FROM elimination_runs;

-- name: ListRuns :many
SELECT er.id, er.period_id, er.rule_id, er.status, er.created_by, er.created_at, er.simulated_at, er.posted_at, er.journal_entry_id, er.summary,
       ru.id, ru.group_id, ru.name, ru.source_company_id, ru.target_company_id, ru.account_src, ru.account_tgt, ru.match_criteria,
       ru.is_active, ru.created_by, ru.created_at, ru.updated_at
FROM elimination_runs er
JOIN elimination_rules ru ON ru.id = er.rule_id
ORDER BY
  CASE WHEN @sort_by::text = 'created_at' AND @sort_dir::text = 'asc' THEN er.created_at END ASC,
  CASE WHEN @sort_by::text = 'created_at' AND @sort_dir::text = 'desc' THEN er.created_at END DESC,
  CASE WHEN @sort_by::text = 'status' AND @sort_dir::text = 'asc' THEN er.status END ASC,
  CASE WHEN @sort_by::text = 'status' AND @sort_dir::text = 'desc' THEN er.status END DESC,
  CASE WHEN @sort_by::text = 'period_id' AND @sort_dir::text = 'asc' THEN er.period_id END ASC,
  CASE WHEN @sort_by::text = 'period_id' AND @sort_dir::text = 'desc' THEN er.period_id END DESC,
  er.created_at DESC -- Default fallback
LIMIT $1 OFFSET $2;

-- name: InsertRun :one
INSERT INTO elimination_runs (period_id, rule_id, status, created_by)
VALUES ($1,$2,'DRAFT',$3)
RETURNING id, period_id, rule_id, status, created_by, created_at, simulated_at, posted_at, journal_entry_id, summary;

-- name: GetRun :one
SELECT er.id, er.period_id, er.rule_id, er.status, er.created_by, er.created_at, er.simulated_at, er.posted_at, er.journal_entry_id, er.summary,
       ru.id, ru.group_id, ru.name, ru.source_company_id, ru.target_company_id, ru.account_src, ru.account_tgt, ru.match_criteria,
       ru.is_active, ru.created_by, ru.created_at, ru.updated_at
FROM elimination_runs er
JOIN elimination_rules ru ON ru.id = er.rule_id
WHERE er.id = $1;

-- name: SaveRunSimulation :exec
UPDATE elimination_runs
SET status = $2,
    simulated_at = $3,
    summary = $4
WHERE id = $1;

-- name: MarkRunPosted :exec
UPDATE elimination_runs
SET status = 'POSTED',
    posted_at = $2,
    journal_entry_id = $3
WHERE id = $1;

-- name: SumAccountBalance :one
SELECT COALESCE(SUM(jl.debit - jl.credit), 0)::float8
FROM journal_lines jl
JOIN journal_entries je ON je.id = jl.je_id AND je.status = 'POSTED'
JOIN accounts acc ON acc.id = jl.account_id
JOIN accounting_periods ap ON ap.id = $1
WHERE je.period_id = ap.period_id
  AND acc.code = $2
  AND COALESCE(jl.dim_company_id, 0) = $3;

-- name: LookupAccountID :one
SELECT id FROM accounts WHERE code = $1;

-- name: ElimLoadAccountingPeriod :one
SELECT ap.id, ap.period_id, ap.name, ap.start_date, ap.end_date
FROM accounting_periods ap
WHERE ap.id = $1;

-- name: ElimListRecentPeriods :many
SELECT ap.id, ap.period_id, ap.name, ap.start_date, ap.end_date
FROM accounting_periods ap
ORDER BY ap.start_date DESC
LIMIT $1;
