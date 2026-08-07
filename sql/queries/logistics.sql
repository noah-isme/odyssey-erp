-- name: CreateRouteOptimizationJob :one
INSERT INTO logistics_route_optimization_jobs (
    company_id, trip_id, status, engine, started_at
) VALUES (
    $1, $2, $3, $4, $5
)
RETURNING id;

-- name: GetRouteOptimizationJob :one
SELECT * FROM logistics_route_optimization_jobs WHERE id = $1;

-- name: UpdateRouteOptimizationJobStatus :exec
UPDATE logistics_route_optimization_jobs
SET status = $2, error_message = $3, completed_at = $4
WHERE id = $1;

-- name: CreateRouteSequence :one
INSERT INTO logistics_route_sequences (
    optimization_job_id, trip_stop_id, optimized_sequence, estimated_arrival_at, estimated_distance_km
) VALUES (
    $1, $2, $3, $4, $5
)
RETURNING id;

-- name: GetRouteSequences :many
SELECT * FROM logistics_route_sequences WHERE optimization_job_id = $1 ORDER BY optimized_sequence ASC;
