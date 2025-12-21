-- name: RbacListRoles :many
SELECT id, name, description, created_at, updated_at
FROM roles
ORDER BY name;

-- name: GetRole :one
SELECT id, name, description, created_at, updated_at
FROM roles
WHERE id = $1;

-- name: RbacCreateRole :one
INSERT INTO roles (name, description)
VALUES ($1, $2)
RETURNING id, name, description, created_at, updated_at;

-- name: UpdateRole :one
UPDATE roles
SET name = $2,
    description = $3,
    updated_at = NOW()
WHERE id = $1
RETURNING id, name, description, created_at, updated_at;

-- name: DeleteRole :execrows
DELETE FROM roles WHERE id = $1;

-- name: ListPermissions :many
SELECT id, name, description
FROM permissions
ORDER BY name;

-- name: CreatePermission :one
INSERT INTO permissions (name, description)
VALUES ($1, $2)
ON CONFLICT (name) DO UPDATE SET description = EXCLUDED.description
RETURNING id, name, description;

-- name: AttachPermissionToRole :exec
INSERT INTO role_permissions (role_id, permission_id)
VALUES ($1, $2)
ON CONFLICT DO NOTHING;

-- name: DetachPermissionFromRole :exec
DELETE FROM role_permissions
WHERE role_id = $1 AND permission_id = $2;

-- name: ListRolePermissions :many
SELECT p.id, p.name, p.description
FROM role_permissions rp
JOIN permissions p ON p.id = rp.permission_id
WHERE rp.role_id = $1
ORDER BY p.name;

-- name: AssignRoleToUser :exec
INSERT INTO user_roles (user_id, role_id)
VALUES ($1, $2)
ON CONFLICT DO NOTHING;

-- name: RemoveRoleFromUser :exec
DELETE FROM user_roles
WHERE user_id = $1 AND role_id = $2;

-- name: UserEffectivePermissions :many
SELECT DISTINCT p.name
FROM user_roles ur
JOIN role_permissions rp ON rp.role_id = ur.role_id
JOIN permissions p ON p.id = rp.permission_id
WHERE ur.user_id = $1
ORDER BY p.name;
