ALTER TABLE timesheets
    DROP COLUMN IF EXISTS fx_rate_locked_at,
    DROP COLUMN IF EXISTS fx_rate_source,
    DROP COLUMN IF EXISTS fx_rate,
    DROP COLUMN IF EXISTS base_amount,
    DROP COLUMN IF EXISTS base_currency,
    DROP COLUMN IF EXISTS billable_rate;
