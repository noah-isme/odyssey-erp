DROP TABLE IF EXISTS fx_revaluation_reversals;
DROP TABLE IF EXISTS fx_revaluation_idempotency;
DROP TABLE IF EXISTS fx_revaluations;
DELETE FROM account_mappings WHERE module='FX' AND key IN ('fx.realized.gain','fx.realized.loss','fx.revaluation.gain','fx.revaluation.loss');
DROP TABLE IF EXISTS fx_journal_idempotency;
DROP TABLE IF EXISTS fx_fetch_runs;
DROP TABLE IF EXISTS fx_daily_rates;

ALTER TABLE ar_payment_allocations DROP COLUMN IF EXISTS original_currency_amount, DROP COLUMN IF EXISTS base_amount, DROP COLUMN IF EXISTS currency, DROP COLUMN IF EXISTS base_currency, DROP COLUMN IF EXISTS fx_rate, DROP COLUMN IF EXISTS fx_rate_date, DROP COLUMN IF EXISTS fx_rate_source, DROP COLUMN IF EXISTS fx_rate_locked_at;
ALTER TABLE ap_payment_allocations DROP COLUMN IF EXISTS original_currency_amount, DROP COLUMN IF EXISTS base_amount, DROP COLUMN IF EXISTS currency, DROP COLUMN IF EXISTS base_currency, DROP COLUMN IF EXISTS fx_rate, DROP COLUMN IF EXISTS fx_rate_date, DROP COLUMN IF EXISTS fx_rate_source, DROP COLUMN IF EXISTS fx_rate_locked_at;
ALTER TABLE ar_payments DROP COLUMN IF EXISTS currency, DROP COLUMN IF EXISTS original_currency_amount, DROP COLUMN IF EXISTS base_currency, DROP COLUMN IF EXISTS base_amount, DROP COLUMN IF EXISTS fx_rate, DROP COLUMN IF EXISTS fx_rate_date, DROP COLUMN IF EXISTS fx_rate_source, DROP COLUMN IF EXISTS fx_rate_locked_at;
ALTER TABLE ap_payments DROP COLUMN IF EXISTS currency, DROP COLUMN IF EXISTS original_currency_amount, DROP COLUMN IF EXISTS base_currency, DROP COLUMN IF EXISTS base_amount, DROP COLUMN IF EXISTS fx_rate, DROP COLUMN IF EXISTS fx_rate_date, DROP COLUMN IF EXISTS fx_rate_source, DROP COLUMN IF EXISTS fx_rate_locked_at;
ALTER TABLE ar_invoices DROP COLUMN IF EXISTS base_currency, DROP COLUMN IF EXISTS original_currency_amount, DROP COLUMN IF EXISTS base_amount, DROP COLUMN IF EXISTS fx_rate, DROP COLUMN IF EXISTS fx_rate_date, DROP COLUMN IF EXISTS fx_rate_source, DROP COLUMN IF EXISTS fx_rate_locked_at;
ALTER TABLE ap_invoices DROP COLUMN IF EXISTS base_currency, DROP COLUMN IF EXISTS original_currency_amount, DROP COLUMN IF EXISTS base_amount, DROP COLUMN IF EXISTS fx_rate, DROP COLUMN IF EXISTS fx_rate_date, DROP COLUMN IF EXISTS fx_rate_source, DROP COLUMN IF EXISTS fx_rate_locked_at;
ALTER TABLE companies DROP CONSTRAINT IF EXISTS companies_base_currency_iso_chk;
ALTER TABLE companies DROP COLUMN IF EXISTS base_currency;

DELETE FROM role_permissions WHERE permission_id IN (SELECT id FROM permissions WHERE name LIKE 'finance.fx.%');
DELETE FROM permissions WHERE name LIKE 'finance.fx.%';
