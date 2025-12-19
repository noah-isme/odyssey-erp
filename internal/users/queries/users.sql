-- name: ListUsers :many
SELECT id, email, name, is_active, created_at, updated_at 
FROM users 
ORDER BY id;
