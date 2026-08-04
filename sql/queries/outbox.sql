-- =============================================================================
-- CROSS-MODULE OUTBOX EVENTS
-- =============================================================================

-- name: InsertOutboxEvent :one
INSERT INTO outbox_events (company_id, correlation_id, causation_id, event_type,
                           aggregate_type, aggregate_id, aggregate_version,
                           payload, idempotency_key)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING id;

-- name: GetUnpublishedOutboxEvents :many
SELECT id, company_id, correlation_id, causation_id, event_type, aggregate_type,
       aggregate_id, aggregate_version, payload, idempotency_key, created_at,
       published_at, publish_attempts, last_error
FROM outbox_events
WHERE company_id = $1
  AND published_at IS NULL
  AND publish_attempts < $2
ORDER BY created_at ASC
LIMIT $3;

-- name: MarkOutboxEventPublished :exec
UPDATE outbox_events
SET published_at = NOW(), publish_attempts = publish_attempts + 1
WHERE id = $1;

-- name: MarkOutboxEventFailed :exec
UPDATE outbox_events
SET publish_attempts = publish_attempts + 1, last_error = $2
WHERE id = $1;
