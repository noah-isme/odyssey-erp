-- name: AuthGetUserByEmail :one
SELECT id, email, password_hash, is_active, created_at, updated_at
FROM users
WHERE email = $1;

-- name: UpsertAdminUser :one
INSERT INTO users (email, password_hash, is_active, created_at, updated_at)
VALUES ($1, $2, TRUE, NOW(), NOW())
ON CONFLICT (email) DO UPDATE
SET password_hash = EXCLUDED.password_hash, is_active = TRUE, updated_at = NOW()
RETURNING id;

-- name: CreateAdminUser :one
INSERT INTO users (email, password_hash, is_active, created_at, updated_at)
VALUES ($1, $2, TRUE, NOW(), NOW())
ON CONFLICT (email) DO NOTHING
RETURNING id;
