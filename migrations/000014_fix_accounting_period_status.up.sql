DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_type t
        JOIN pg_namespace n ON n.oid = t.typnamespace
        WHERE t.typname = 'accounting_period_status'
          AND (n.nspname = current_schema() OR n.nspname = 'public')
    ) THEN
        CREATE TYPE accounting_period_status AS ENUM ('OPEN','SOFT_CLOSED','HARD_CLOSED');
    END IF;
END $$;

ALTER TABLE accounting_periods
    ALTER COLUMN status DROP DEFAULT,
    ALTER COLUMN status TYPE accounting_period_status
        USING (
            CASE REPLACE(upper(COALESCE(status::text, '')), ' ', '_')
                WHEN '' THEN 'OPEN'
                WHEN 'LOCKED' THEN 'HARD_CLOSED'
                WHEN 'CLOSED' THEN 'SOFT_CLOSED'
                WHEN 'SOFT_CLOSED' THEN 'SOFT_CLOSED'
                WHEN 'HARD_CLOSED' THEN 'HARD_CLOSED'
                WHEN 'OPEN' THEN 'OPEN'
                ELSE 'OPEN'
            END
        )::accounting_period_status,
    ALTER COLUMN status SET DEFAULT 'OPEN';
