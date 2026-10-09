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

-- name: UpsertRole :one
INSERT INTO roles (name, description)
VALUES ($1, $2)
ON CONFLICT (name) DO UPDATE SET description = EXCLUDED.description
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

-- Workspace tenancy (v0.10.0-rc.11). Company visibility comes from active
-- rbac_user_role_assignments rows, company-wide or branch-scoped; permission
-- evaluation stays on user_roles (UserEffectivePermissions).

-- name: ListUserCompanies :many
SELECT DISTINCT c.id, c.name
FROM rbac_user_role_assignments a
JOIN companies c ON c.id = a.company_id
WHERE a.user_id = $1
  AND a.valid_from <= NOW()
  AND (a.valid_to IS NULL OR a.valid_to > NOW())
ORDER BY c.name, c.id;

-- name: UserHasActiveCompanyAssignment :one
SELECT EXISTS (
    SELECT 1
    FROM rbac_user_role_assignments a
    WHERE a.user_id = $1
      AND a.company_id = $2
      AND a.valid_from <= NOW()
      AND (a.valid_to IS NULL OR a.valid_to > NOW())
)::boolean AS has_assignment;

-- name: UserGlobalRoleNames :many
SELECT DISTINCT r.name
FROM user_roles ur
JOIN roles r ON r.id = ur.role_id
WHERE ur.user_id = $1
ORDER BY r.name;

-- name: GrantCompanyWideAssignment :execrows
-- Same shape as migration 000124: one company-wide assignment per existing
-- company, starting at the fixed epoch marker, idempotent on the company
-- scope key.
INSERT INTO rbac_user_role_assignments (
    company_id,
    user_id,
    role_id,
    branch_id,
    valid_from,
    valid_to
)
SELECT c.id, sqlc.arg(user_id)::bigint, sqlc.arg(role_id)::bigint, NULL, TIMESTAMPTZ '1970-01-01 00:00:00+00', NULL
FROM companies c
ON CONFLICT (company_id, user_id, role_id, valid_from)
    WHERE branch_id IS NULL
DO NOTHING;
