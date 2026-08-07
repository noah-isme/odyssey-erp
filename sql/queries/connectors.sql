-- name: CreateConnection :one
INSERT INTO connector_connections (
    company_id, provider, type, name, secret_ref, status, token_expiry
) VALUES (
    $1, $2, $3, $4, $5, $6, $7
) RETURNING *;

-- name: GetConnection :one
SELECT * FROM connector_connections
WHERE id = $1 AND company_id = $2;

-- name: ListConnections :many
SELECT * FROM connector_connections
WHERE company_id = $1
ORDER BY created_at DESC;

-- name: UpdateConnectionStatus :one
UPDATE connector_connections
SET status = $2, last_error = $3, updated_at = NOW()
WHERE id = $1 AND company_id = $4
RETURNING *;

-- name: EnqueueOutboxCommand :one
INSERT INTO connector_outbox_commands (
    company_id, connection_id, command_type, correlation_id, payload
) VALUES (
    $1, $2, $3, $4, $5
) RETURNING *;

-- name: GetPendingOutboxCommands :many
SELECT * FROM connector_outbox_commands
WHERE state IN ('pending', 'processing')
  AND next_attempt <= NOW()
ORDER BY next_attempt ASC
LIMIT $1;

-- name: UpdateOutboxCommandState :one
UPDATE connector_outbox_commands
SET state = $2,
    attempts = attempts + 1,
    next_attempt = $3,
    updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: InsertInboxEvent :one
INSERT INTO connector_inbox_events (
    company_id, connection_id, provider_event_id, raw_payload
) VALUES (
    $1, $2, $3, $4
) ON CONFLICT (connection_id, provider_event_id) DO NOTHING
RETURNING *;

-- name: GetUnprocessedInboxEvents :many
SELECT * FROM connector_inbox_events
WHERE company_id = $1 AND connection_id = $2 AND processed = false
ORDER BY created_at ASC
LIMIT $3;

-- name: MarkInboxEventProcessed :exec
UPDATE connector_inbox_events
SET processed = true, processed_at = NOW()
WHERE id = $1;

-- name: InsertCanonicalEvent :one
INSERT INTO connector_canonical_events (
    company_id, connection_id, event_type, event_time, correlation_id, causation_id, payload
) VALUES (
    $1, $2, $3, $4, $5, $6, $7
) RETURNING *;
