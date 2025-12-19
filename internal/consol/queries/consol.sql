-- name: FindPeriodID :one
SELECT id FROM periods WHERE code = $1;

-- name: GetGroup :one
SELECT name, reporting_currency FROM consol_groups WHERE id = $1;

-- name: Members :many
SELECT cm.company_id, c.name, cm.enabled
FROM consol_members cm
JOIN companies c ON c.id = cm.company_id
WHERE cm.group_id = $1
ORDER BY c.name;

-- name: MemberCurrencies :many
SELECT cm.company_id, 'IDR'::text as currency
FROM consol_members cm
JOIN companies c ON c.id = cm.company_id
WHERE cm.group_id = $1;

-- name: DeleteConsolBalances :exec
DELETE FROM mv_consol_balances WHERE period_id = $1 AND group_id = $2;

-- name: CalculateConsolBalances :exec
INSERT INTO mv_consol_balances (period_id, group_id, group_account_id, local_ccy_amt, group_ccy_amt, members)
SELECT $1 AS period_id,
       $2 AS group_id,
       base.group_account_id,
       SUM(base.local_amt) AS local_ccy_amt,
       SUM(base.local_amt) AS group_ccy_amt,
       jsonb_agg(
           jsonb_build_object(
               'company_id', base.company_id,
               'company_name', base.company_name,
               'local_ccy_amt', base.local_amt
           ) ORDER BY base.company_id
       ) AS members
FROM (
    SELECT
        je.period_id,
        cm.group_id,
        am.group_account_id,
        cm.company_id,
        c.name AS company_name,
        SUM(jl.debit - jl.credit) AS local_amt
    FROM journal_lines jl
    JOIN journal_entries je ON je.id = jl.je_id AND je.status = 'POSTED' AND je.period_id = $1
    JOIN consol_members cm ON cm.company_id = jl.dim_company_id AND cm.group_id = $2 AND cm.enabled
    JOIN companies c ON c.id = cm.company_id
    JOIN account_map am ON am.group_id = cm.group_id AND am.company_id = cm.company_id AND am.local_account_id = jl.account_id
    GROUP BY je.period_id, cm.group_id, am.group_account_id, cm.company_id, c.name
) AS base
GROUP BY base.group_account_id;

-- name: ListGroupIDs :many
SELECT id FROM consol_groups ORDER BY id;

-- name: ActiveConsolidationPeriod :one
SELECT code FROM periods WHERE status = 'OPEN_CONSOL' ORDER BY start_date DESC LIMIT 1;

-- name: Balances :many
SELECT mv.group_account_id,
       ga.code,
       ga.name,
       mv.local_ccy_amt,
       mv.group_ccy_amt,
       mv.members
FROM mv_consol_balances mv
JOIN consol_group_accounts ga ON ga.id = mv.group_account_id
WHERE mv.group_id = $1 AND mv.period_id = $2
ORDER BY ga.code;

-- name: ConsolBalancesByType :many
SELECT
    mv.group_account_id,
    ga.code,
    ga.name,
    ga.type,
    mv.local_ccy_amt,
    mv.group_ccy_amt,
    mv.members
FROM mv_consol_balances mv
JOIN consol_group_accounts ga ON ga.id = mv.group_account_id
WHERE mv.group_id = $1 AND mv.period_id = $2
ORDER BY ga.code;

-- name: FxRateForPeriod :one
SELECT average_rate, closing_rate FROM fx_rates WHERE as_of_date = $1 AND pair = $2 LIMIT 1;

-- name: UpsertFxRate :exec
INSERT INTO fx_rates (as_of_date, pair, average_rate, closing_rate)
VALUES ($1, $2, $3, $4)
ON CONFLICT (as_of_date, pair)
DO UPDATE SET average_rate = EXCLUDED.average_rate, closing_rate = EXCLUDED.closing_rate;
