-- name: GetLatestForecastRun :one
SELECT * FROM forecast_runs 
WHERE company_id = $1 AND scenario_id = $2
ORDER BY created_at DESC, id DESC LIMIT 1;

-- name: ListForecastDailyBucketsByRun :many
SELECT * FROM forecast_daily_buckets WHERE run_id = $1 ORDER BY bucket_date ASC;
