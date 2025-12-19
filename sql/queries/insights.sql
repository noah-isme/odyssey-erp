-- name: CompareMonthlyNetRevenue :many
SELECT period,
       COALESCE(SUM(net), 0)::double precision AS net,
       COALESCE(SUM(revenue), 0)::double precision AS revenue
FROM mv_pl_monthly
WHERE period BETWEEN sqlc.arg(from_period) AND sqlc.arg(to_period)
  AND company_id = sqlc.arg(company_id)
  AND (sqlc.narg(branch_id)::bigint IS NULL OR branch_id = sqlc.narg(branch_id)::bigint)
GROUP BY period
ORDER BY period;

-- name: ContributionByBranch :many
SELECT branch_id,
       COALESCE(SUM(net), 0)::double precision AS net,
       COALESCE(SUM(revenue), 0)::double precision AS revenue
FROM mv_pl_monthly
WHERE period = sqlc.arg(period)
  AND company_id = sqlc.arg(company_id)
GROUP BY branch_id
ORDER BY branch_id;
