-- Phase 7 S2.1 intercompany helpers

CREATE VIEW ic_intercompany_balances AS
SELECT
    je.period_id,
    cm.group_id,
    jl.dim_company_id AS company_id,
    jl.ic_party_id AS counterparty_id,
    am.group_account_id,
    SUM(jl.debit - jl.credit) AS balance
FROM journal_lines jl
JOIN journal_entries je ON je.id = jl.je_id AND je.status = 'POSTED'
JOIN consol_members cm ON cm.company_id = jl.dim_company_id AND cm.enabled
JOIN account_map am ON am.group_id = cm.group_id
    AND am.company_id = jl.dim_company_id
    AND am.local_account_id = jl.account_id
WHERE jl.ic_party_id IS NOT NULL
GROUP BY je.period_id, cm.group_id, jl.dim_company_id, jl.ic_party_id, am.group_account_id;

CREATE VIEW ic_arap_pairs AS
WITH balances AS (
    SELECT
        b.period_id,
        b.group_id,
        b.company_id,
        b.counterparty_id,
        b.group_account_id,
        b.balance,
        ga.type
    FROM ic_intercompany_balances b
    JOIN consol_group_accounts ga ON ga.id = b.group_account_id
),
paired AS (
    SELECT
        a.group_id,
        a.period_id,
        a.company_id AS company_a_id,
        a.counterparty_id AS company_b_id,
        a.group_account_id AS ar_group_account_id,
        b.group_account_id AS ap_group_account_id,
        GREATEST(a.balance, 0) AS ar_amount,
        GREATEST(-b.balance, 0) AS ap_amount
    FROM balances a
    JOIN balances b ON b.group_id = a.group_id
        AND b.period_id = a.period_id
        AND b.company_id = a.counterparty_id
        AND b.counterparty_id = a.company_id
    JOIN ic_rules r ON r.group_id = a.group_id
        AND r.type = 'AR_AP'
        AND r.enabled
        AND r.src_group_acc = a.group_account_id
        AND r.dst_group_acc = b.group_account_id
    WHERE a.type = 'ASSET'
      AND b.type = 'LIABILITY'
)
SELECT
    group_id,
    period_id,
    company_a_id,
    company_b_id,
    ar_group_account_id,
    ap_group_account_id,
    SUM(ar_amount) AS ar_amount,
    SUM(ap_amount) AS ap_amount
FROM paired
GROUP BY group_id, period_id, company_a_id, company_b_id, ar_group_account_id, ap_group_account_id;
