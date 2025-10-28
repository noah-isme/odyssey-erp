-- name: MonthlyPL :many
SELECT m.period,
       m.company_id,
       m.branch_id,
       m.revenue::double precision AS revenue,
       m.cogs::double precision AS cogs,
       m.opex::double precision AS opex,
       m.net::double precision AS net
FROM mv_pl_monthly m
WHERE m.period BETWEEN sqlc.arg(from_period) AND sqlc.arg(to_period)
  AND m.company_id = sqlc.arg(company_id)
  AND (sqlc.narg(branch_id) IS NULL OR m.branch_id = sqlc.narg(branch_id))
ORDER BY m.period;

-- name: MonthlyCashflow :many
SELECT m.period,
       m.company_id,
       m.branch_id,
       m.cash_in::double precision AS cash_in,
       m.cash_out::double precision AS cash_out
FROM mv_cashflow_monthly m
WHERE m.period BETWEEN sqlc.arg(from_period) AND sqlc.arg(to_period)
  AND m.company_id = sqlc.arg(company_id)
  AND (sqlc.narg(branch_id) IS NULL OR m.branch_id = sqlc.narg(branch_id))
ORDER BY m.period;

-- name: AgingAR :many
SELECT a.bucket, a.amount::double precision AS amount, a.as_of
FROM mv_ar_aging a
WHERE a.as_of = sqlc.arg(as_of)
  AND a.company_id = sqlc.arg(company_id)
  AND (sqlc.narg(branch_id) IS NULL OR a.branch_id = sqlc.narg(branch_id))
ORDER BY a.bucket;

-- name: AgingAP :many
SELECT a.bucket, a.amount::double precision AS amount, a.as_of
FROM mv_ap_aging a
WHERE a.as_of = sqlc.arg(as_of)
  AND a.company_id = sqlc.arg(company_id)
  AND (sqlc.narg(branch_id) IS NULL OR a.branch_id = sqlc.narg(branch_id))
ORDER BY a.bucket;

-- name: KpiSummary :one
WITH pl AS (
  SELECT
    COALESCE(SUM(m.net)::double precision, 0) AS net_profit,
    COALESCE(SUM(m.revenue)::double precision, 0) AS revenue,
    COALESCE(SUM(m.opex)::double precision, 0) AS opex,
    COALESCE(SUM(m.cogs)::double precision, 0) AS cogs
  FROM mv_pl_monthly m
  WHERE m.period = sqlc.arg(period)
    AND m.company_id = sqlc.arg(company_id)
    AND (sqlc.narg(branch_id) IS NULL OR m.branch_id = sqlc.narg(branch_id))
), cash AS (
  SELECT
    COALESCE(SUM(m.cash_in)::double precision, 0) AS cash_in,
    COALESCE(SUM(m.cash_out)::double precision, 0) AS cash_out
  FROM mv_cashflow_monthly m
  WHERE m.period = sqlc.arg(period)
    AND m.company_id = sqlc.arg(company_id)
    AND (sqlc.narg(branch_id) IS NULL OR m.branch_id = sqlc.narg(branch_id))
), ar AS (
  SELECT COALESCE(SUM(a.amount)::double precision, 0) AS outstanding
  FROM mv_ar_aging a
  WHERE a.as_of = sqlc.arg(as_of)
    AND a.company_id = sqlc.arg(company_id)
    AND (sqlc.narg(branch_id) IS NULL OR a.branch_id = sqlc.narg(branch_id))
), ap AS (
  SELECT COALESCE(SUM(a.amount)::double precision, 0) AS outstanding
  FROM mv_ap_aging a
  WHERE a.as_of = sqlc.arg(as_of)
    AND a.company_id = sqlc.arg(company_id)
    AND (sqlc.narg(branch_id) IS NULL OR a.branch_id = sqlc.narg(branch_id))
)
SELECT
  pl.net_profit,
  pl.revenue,
  pl.opex,
  pl.cogs,
  cash.cash_in,
  cash.cash_out,
  ar.outstanding AS ar_outstanding,
  ap.outstanding AS ap_outstanding
FROM pl
CROSS JOIN cash
CROSS JOIN ar
CROSS JOIN ap;
