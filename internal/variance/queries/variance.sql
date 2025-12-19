-- name: InsertRule :one
INSERT INTO variance_rules (company_id, name, comparison_type, base_period_id, compare_period_id, created_by)
VALUES ($1,$2,$3,$4,$5,$6)
RETURNING id, company_id, name, comparison_type, base_period_id, compare_period_id, dimension_filters,
          threshold_amount::float8, threshold_percent::float8, is_active, created_by, created_at;

-- name: ListRules :many
SELECT id, company_id, name, comparison_type, base_period_id, compare_period_id, dimension_filters,
       threshold_amount::float8, threshold_percent::float8, is_active, created_by, created_at
FROM variance_rules
WHERE (@company_id::bigint = 0 OR company_id = @company_id)
ORDER BY created_at DESC
LIMIT 100;

-- name: GetRule :one
SELECT id, company_id, name, comparison_type, base_period_id, compare_period_id, dimension_filters,
       threshold_amount::float8, threshold_percent::float8, is_active, created_by, created_at
FROM variance_rules WHERE id = $1;

-- name: InsertSnapshot :one
INSERT INTO variance_snapshots (rule_id, period_id, status, generated_by)
VALUES ($1,$2,'PENDING',$3)
RETURNING id, rule_id, period_id, status, generated_at, generated_by, error_message, payload, created_at, updated_at;

-- name: ListSnapshots :many
SELECT vs.id, vs.rule_id, vs.period_id, vs.status, vs.generated_at, vs.generated_by, vs.error_message, vs.payload,
       vs.created_at, vs.updated_at,
       vr.id, vr.company_id, vr.name, vr.comparison_type, vr.base_period_id, vr.compare_period_id, vr.dimension_filters,
       vr.threshold_amount::float8, vr.threshold_percent::float8, vr.is_active, vr.created_by, vr.created_at
FROM variance_snapshots vs
JOIN variance_rules vr ON vr.id = vs.rule_id
ORDER BY
  CASE WHEN @sort_by::text = 'created_at' AND @sort_dir::text = 'asc' THEN vs.created_at END ASC,
  CASE WHEN @sort_by::text = 'created_at' AND @sort_dir::text = 'desc' THEN vs.created_at END DESC,
  CASE WHEN @sort_by::text = 'status' AND @sort_dir::text = 'asc' THEN vs.status END ASC,
  CASE WHEN @sort_by::text = 'status' AND @sort_dir::text = 'desc' THEN vs.status END DESC,
  CASE WHEN @sort_by::text = 'period_id' AND @sort_dir::text = 'asc' THEN vs.period_id END ASC,
  CASE WHEN @sort_by::text = 'period_id' AND @sort_dir::text = 'desc' THEN vs.period_id END DESC,
  vs.created_at DESC -- Default
LIMIT $1 OFFSET $2;

-- name: GetSnapshot :one
SELECT vs.id, vs.rule_id, vs.period_id, vs.status, vs.generated_at, vs.generated_by, vs.error_message, vs.payload,
       vs.created_at, vs.updated_at,
       vr.id, vr.company_id, vr.name, vr.comparison_type, vr.base_period_id, vr.compare_period_id, vr.dimension_filters,
       vr.threshold_amount::float8, vr.threshold_percent::float8, vr.is_active, vr.created_by, vr.created_at
FROM variance_snapshots vs
JOIN variance_rules vr ON vr.id = vs.rule_id
WHERE vs.id = $1;

-- name: UpdateStatus :exec
UPDATE variance_snapshots SET status = $2, updated_at = NOW() WHERE id = $1;

-- name: SavePayload :exec
UPDATE variance_snapshots
SET payload = $2,
    error_message = $3,
    updated_at = NOW(),
    generated_at = CASE WHEN $3::text IS NULL OR $3::text = '' THEN NOW() ELSE generated_at END
WHERE id = $1;

-- name: LoadPayload :one
SELECT payload FROM variance_snapshots WHERE id = $1;

-- name: AggregateBalances :many
SELECT acc.code, acc.name, SUM(jl.debit - jl.credit)::float8 AS amount
FROM journal_lines jl
JOIN journal_entries je ON je.id = jl.je_id AND je.status = 'POSTED'
JOIN accounts acc ON acc.id = jl.account_id
JOIN accounting_periods ap ON ap.id = $1
WHERE je.period_id = ap.period_id AND COALESCE(jl.dim_company_id, 0) = $2
GROUP BY acc.code, acc.name;

-- name: LoadAccountingPeriod :one
SELECT ap.id, ap.period_id, ap.name, ap.start_date, ap.end_date
FROM accounting_periods ap WHERE ap.id = $1;
