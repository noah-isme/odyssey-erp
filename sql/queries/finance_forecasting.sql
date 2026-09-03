-- name: CreateForecastScenario :one
INSERT INTO forecast_scenarios (company_id, name, policy_version)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetForecastScenario :one
SELECT * FROM forecast_scenarios WHERE id = $1;

-- name: ForecastScenarioBelongsToCompany :one
SELECT EXISTS(
    SELECT 1 FROM forecast_scenarios
    WHERE id = $1 AND company_id = $2
);

-- name: ListForecastScenarios :many
SELECT * FROM forecast_scenarios WHERE company_id = $1 ORDER BY id;

-- name: CreateForecastRun :one
INSERT INTO forecast_runs (company_id, scenario_id, status, fx_snapshot)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: UpdateForecastRunStatus :exec
UPDATE forecast_runs
SET status = $2,
    completed_at = $3,
    error_details = $4,
    fx_snapshot = COALESCE($5, fx_snapshot)
WHERE id = $1;

-- name: GetForecastRun :one
SELECT * FROM forecast_runs WHERE id = $1;

-- name: CreateForecastDailyBucket :one
INSERT INTO forecast_daily_buckets (run_id, bank_account_id, currency, bucket_date, opening_balance, total_inflow, total_outflow, closing_balance)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: ListForecastDailyBuckets :many
SELECT * FROM forecast_daily_buckets WHERE run_id = $1 ORDER BY bucket_date;

-- name: CreateForecastSourceLine :one
INSERT INTO forecast_source_lines (run_id, daily_bucket_id, source_type, source_ref, amount, currency, expected_date, certainty)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: ListForecastSourceLines :many
SELECT * FROM forecast_source_lines WHERE run_id = $1 AND daily_bucket_id = $2;

-- name: CreateForecastAdjustment :one
INSERT INTO forecast_adjustments (company_id, scenario_id, adjustment_type, description, amount, currency, expected_date)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: ListForecastAdjustments :many
SELECT * FROM forecast_adjustments WHERE scenario_id = $1;
