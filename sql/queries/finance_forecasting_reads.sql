-- name: GetLatestForecastRun :one
SELECT * FROM forecast_runs 
WHERE company_id = $1 AND scenario_id = $2 AND status = 'COMPLETED' 
ORDER BY completed_at DESC LIMIT 1;

-- name: ListForecastDailyBucketsByRun :many
SELECT * FROM forecast_daily_buckets WHERE run_id = $1 ORDER BY bucket_date ASC;
