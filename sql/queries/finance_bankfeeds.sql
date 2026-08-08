-- name: CreateBankConnection :one
INSERT INTO bank_connections (
    company_id, provider_id, connection_ref, status, consent_expires_at, health_status
) VALUES (
    $1, $2, $3, $4, $5, $6
) RETURNING *;

-- name: GetBankConnection :one
SELECT * FROM bank_connections WHERE id = $1;

-- name: ListBankConnections :many
SELECT * FROM bank_connections WHERE company_id = $1 ORDER BY created_at DESC;

-- name: UpdateBankConnectionStatus :exec
UPDATE bank_connections SET status = $2, health_status = $3, error_details = $4, updated_at = NOW() WHERE id = $1;

-- name: CreateBankConnectionAccount :one
INSERT INTO bank_connection_accounts (
    connection_id, bank_account_id, external_account_id
) VALUES (
    $1, $2, $3
) RETURNING *;

-- name: GetBankConnectionAccount :one
SELECT * FROM bank_connection_accounts WHERE connection_id = $1 AND external_account_id = $2;

-- name: ListBankConnectionAccounts :many
SELECT * FROM bank_connection_accounts WHERE connection_id = $1;

-- name: UpdateBankConnectionAccountCursor :exec
UPDATE bank_connection_accounts SET cursor = $2, last_synced_at = NOW(), updated_at = NOW() WHERE id = $1;

-- name: CreateBankFeedSyncRun :one
INSERT INTO bank_feed_sync_runs (
    connection_id, status
) VALUES (
    $1, $2
) RETURNING *;

-- name: UpdateBankFeedSyncRun :exec
UPDATE bank_feed_sync_runs SET status = $2, completed_at = $3, error_details = $4 WHERE id = $1;

-- name: CreateBankFeedEvent :one
INSERT INTO bank_feed_events (
    provider_id, event_type, payload, occurred_at
) VALUES (
    $1, $2, $3, $4
) RETURNING *;

-- name: UpdateBankFeedEventStatus :exec
UPDATE bank_feed_events SET status = $2, error_details = $3, updated_at = NOW() WHERE id = $1;
