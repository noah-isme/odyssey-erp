-- name: AuthGetUserByEmail :one
SELECT id, email, password_hash, is_active, created_at, updated_at
FROM users
WHERE email = $1;
