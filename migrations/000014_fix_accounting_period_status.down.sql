ALTER TABLE accounting_periods
    ALTER COLUMN status DROP DEFAULT,
    ALTER COLUMN status TYPE TEXT USING status::text,
    ALTER COLUMN status SET DEFAULT 'OPEN';

DROP TYPE IF EXISTS accounting_period_status;
