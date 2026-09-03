-- name: CreateReportingDataset :one
INSERT INTO reporting_datasets (
    company_id,
    version,
    key,
    business_owner,
    technical_owner,
    status,
    description,
    grain
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8
) RETURNING *;

-- name: GetReportingDataset :one
SELECT * FROM reporting_datasets
WHERE company_id = $1 AND key = $2 AND version = $3;

-- name: ListReportingDatasets :many
SELECT * FROM reporting_datasets
WHERE company_id = $1 AND status = 'PUBLISHED'
ORDER BY key ASC, version DESC;

-- name: CreateReportingDatasetField :one
INSERT INTO reporting_dataset_fields (
    dataset_id,
    field_name,
    field_type,
    classification,
    is_dimension,
    is_measure
) VALUES (
    $1, $2, $3, $4, $5, $6
) RETURNING *;

-- name: ListReportingDatasetFields :many
SELECT * FROM reporting_dataset_fields
WHERE dataset_id = $1
ORDER BY field_name ASC;

-- name: CreateReportRun :one
INSERT INTO report_runs (
    company_id,
    dataset_id,
    actor_id,
    status,
    query_cost_estimate
) VALUES (
    $1, $2, $3, $4, $5
) RETURNING *;

-- name: UpdateReportRunStatus :exec
UPDATE report_runs
SET status = $2,
    row_count = $3,
    error_message = $4,
    executed_sql = $5,
    execution_time_ms = $6,
    started_at = $7,
    completed_at = $8
WHERE id = $1;
