-- name: GetOpenPeriodByDate :one
SELECT id, code, start_date, end_date, status, closed_at, locked_by, created_at, updated_at
FROM periods
WHERE status = 'OPEN' AND start_date <= $1 AND end_date >= $1
LIMIT 1;
