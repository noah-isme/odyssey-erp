-- Phase 5 analytics materialized views and helpers

CREATE OR REPLACE FUNCTION analytics_aging_bucket(due_date DATE, as_of DATE)
RETURNS TEXT
LANGUAGE plpgsql
AS $$
DECLARE
    age_days INTEGER;
BEGIN
    IF due_date IS NULL OR as_of IS NULL THEN
        RETURN 'UNKNOWN';
    END IF;
    age_days := (as_of - due_date);
    IF age_days <= 30 THEN
        RETURN '0-30';
    ELSIF age_days <= 60 THEN
        RETURN '31-60';
    ELSIF age_days <= 90 THEN
        RETURN '61-90';
    ELSE
        RETURN '>90';
    END IF;
END;
$$;

CREATE MATERIALIZED VIEW IF NOT EXISTS mv_pl_monthly AS
SELECT
    to_char(date_trunc('month', je.date), 'YYYY-MM') AS period,
    COALESCE(jl.dim_company_id, 0) AS company_id,
    COALESCE(jl.dim_branch_id, 0) AS branch_id,
    SUM(CASE WHEN a.type = 'REVENUE' THEN jl.credit - jl.debit ELSE 0::numeric END) AS revenue,
    SUM(CASE WHEN a.type = 'EXPENSE' AND a.code >= '5000' AND a.code < '6000' THEN jl.debit - jl.credit ELSE 0::numeric END) AS cogs,
    SUM(CASE WHEN a.type = 'EXPENSE' AND NOT (a.code >= '5000' AND a.code < '6000') THEN jl.debit - jl.credit ELSE 0::numeric END) AS opex,
    SUM(CASE WHEN a.type = 'REVENUE' THEN jl.credit - jl.debit ELSE 0::numeric END)
      - SUM(CASE WHEN a.type = 'EXPENSE' THEN jl.debit - jl.credit ELSE 0::numeric END) AS net
FROM journal_lines jl
JOIN journal_entries je ON je.id = jl.je_id AND je.status = 'POSTED'
JOIN accounts a ON a.id = jl.account_id
GROUP BY period, company_id, branch_id
ORDER BY period, company_id, branch_id;

CREATE INDEX IF NOT EXISTS idx_mv_pl_monthly_period ON mv_pl_monthly(period);
CREATE INDEX IF NOT EXISTS idx_mv_pl_monthly_company_branch ON mv_pl_monthly(company_id, branch_id);

CREATE MATERIALIZED VIEW IF NOT EXISTS mv_cashflow_monthly AS
SELECT
    to_char(date_trunc('month', je.date), 'YYYY-MM') AS period,
    COALESCE(jl.dim_company_id, 0) AS company_id,
    COALESCE(jl.dim_branch_id, 0) AS branch_id,
    SUM(CASE WHEN a.type = 'ASSET' AND a.code LIKE '11%'
        THEN GREATEST(jl.debit - jl.credit, 0::numeric) ELSE 0::numeric END) AS cash_in,
    SUM(CASE WHEN a.type = 'ASSET' AND a.code LIKE '11%'
        THEN GREATEST(jl.credit - jl.debit, 0::numeric) ELSE 0::numeric END) AS cash_out
FROM journal_lines jl
JOIN journal_entries je ON je.id = jl.je_id AND je.status = 'POSTED'
JOIN accounts a ON a.id = jl.account_id
GROUP BY period, company_id, branch_id
ORDER BY period, company_id, branch_id;

CREATE INDEX IF NOT EXISTS idx_mv_cashflow_monthly_period ON mv_cashflow_monthly(period);
CREATE INDEX IF NOT EXISTS idx_mv_cashflow_monthly_company_branch ON mv_cashflow_monthly(company_id, branch_id);

CREATE MATERIALIZED VIEW IF NOT EXISTS mv_ar_aging AS
SELECT
    analytics_aging_bucket(je.date, CURRENT_DATE) AS bucket,
    COALESCE(jl.dim_company_id, 0) AS company_id,
    COALESCE(jl.dim_branch_id, 0) AS branch_id,
    CURRENT_DATE AS as_of,
    SUM(GREATEST(jl.debit - jl.credit, 0::numeric)) AS amount
FROM journal_lines jl
JOIN journal_entries je ON je.id = jl.je_id AND je.status = 'POSTED'
JOIN accounts a ON a.id = jl.account_id
WHERE a.type = 'ASSET' AND a.code LIKE '12%'
GROUP BY bucket, company_id, branch_id, as_of
ORDER BY bucket, company_id, branch_id;

CREATE INDEX IF NOT EXISTS idx_mv_ar_aging_bucket ON mv_ar_aging(bucket);
CREATE INDEX IF NOT EXISTS idx_mv_ar_aging_company_branch ON mv_ar_aging(company_id, branch_id);

CREATE MATERIALIZED VIEW IF NOT EXISTS mv_ap_aging AS
SELECT
    analytics_aging_bucket(je.date, CURRENT_DATE) AS bucket,
    COALESCE(jl.dim_company_id, 0) AS company_id,
    COALESCE(jl.dim_branch_id, 0) AS branch_id,
    CURRENT_DATE AS as_of,
    SUM(GREATEST(jl.credit - jl.debit, 0::numeric)) AS amount
FROM journal_lines jl
JOIN journal_entries je ON je.id = jl.je_id AND je.status = 'POSTED'
JOIN accounts a ON a.id = jl.account_id
WHERE a.type = 'LIABILITY' AND a.code LIKE '21%'
GROUP BY bucket, company_id, branch_id, as_of
ORDER BY bucket, company_id, branch_id;

CREATE INDEX IF NOT EXISTS idx_mv_ap_aging_bucket ON mv_ap_aging(bucket);
CREATE INDEX IF NOT EXISTS idx_mv_ap_aging_company_branch ON mv_ap_aging(company_id, branch_id);

