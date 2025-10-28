-- name: GetAccounts :many
SELECT id, code, name, type, parent_id, is_active, created_at, updated_at
FROM accounts
ORDER BY code;
