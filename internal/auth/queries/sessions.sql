-- name: CreateSession :exec
INSERT INTO sessions (id, user_id, created_at, expires_at, ip, ua)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: DeleteSession :exec
DELETE FROM sessions
WHERE id = $1;
