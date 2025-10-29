-- Revert Phase 7 consolidation core schema

DROP INDEX IF EXISTS idx_mv_consol_balances_group;
DROP INDEX IF EXISTS idx_mv_consol_balances_period;
DROP MATERIALIZED VIEW IF EXISTS mv_consol_balances;

DELETE FROM permissions
WHERE name IN (
    'finance.view_consolidation',
    'finance.post_elimination',
    'finance.manage_consolidation',
    'finance.export_consolidation'
);

DROP TABLE IF EXISTS fx_policy;
DROP TABLE IF EXISTS fx_rates;
DROP INDEX IF EXISTS idx_elimination_lines_group_account;
DROP INDEX IF EXISTS idx_elimination_lines_header;
DROP TABLE IF EXISTS elimination_journal_lines;
DROP INDEX IF EXISTS idx_elimination_headers_source;
DROP INDEX IF EXISTS idx_elimination_headers_group_period;
DROP TABLE IF EXISTS elimination_journal_headers;
DROP TABLE IF EXISTS ic_rules;
DROP INDEX IF EXISTS idx_account_map_group_account;
DROP TABLE IF EXISTS account_map;
DROP INDEX IF EXISTS idx_consol_members_company;
DROP INDEX IF EXISTS idx_consol_members_group;
DROP TABLE IF EXISTS consol_members;
DROP INDEX IF EXISTS idx_consol_group_accounts_group;
DROP TABLE IF EXISTS consol_group_accounts;
DROP TABLE IF EXISTS consol_groups;

DROP INDEX IF EXISTS idx_journal_lines_ic_party;
ALTER TABLE journal_lines
    DROP COLUMN IF EXISTS ic_party_id;

DROP INDEX IF EXISTS idx_accounts_ic_flag;
ALTER TABLE accounts
    DROP COLUMN IF EXISTS ic_flag;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM pg_enum e
        JOIN pg_type t ON t.oid = e.enumtypid
        WHERE t.typname = 'period_status' AND e.enumlabel = 'OPEN_CONSOL'
    ) THEN
        CREATE TYPE period_status_old AS ENUM ('OPEN', 'CLOSED', 'LOCKED');

        ALTER TABLE periods
            ALTER COLUMN status TYPE period_status_old
            USING (
                CASE
                    WHEN status = 'OPEN_CONSOL' THEN 'OPEN'
                    ELSE status::text
                END
            )::period_status_old;

        DROP TYPE period_status;
        ALTER TYPE period_status_old RENAME TO period_status;
    END IF;
END$$;
