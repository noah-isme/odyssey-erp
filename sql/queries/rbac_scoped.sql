-- name: RbacCreateScopedRoleAssignment :one
INSERT INTO rbac_user_role_assignments (
    company_id,
    user_id,
    role_id,
    branch_id,
    valid_from,
    valid_to
)
VALUES (
    @company_id,
    @user_id,
    @role_id,
    sqlc.narg(branch_id)::bigint,
    @valid_from::timestamptz,
    sqlc.narg(valid_to)::timestamptz
)
RETURNING id, company_id, user_id, role_id, branch_id, valid_from, valid_to, created_at;

-- name: RbacListScopedRoleAssignments :many
SELECT id, company_id, user_id, role_id, branch_id, valid_from, valid_to, created_at
FROM rbac_user_role_assignments
WHERE company_id = @company_id
  AND user_id = @user_id
ORDER BY valid_from DESC, id DESC;

-- name: RbacDeleteScopedRoleAssignment :execrows
DELETE FROM rbac_user_role_assignments
WHERE company_id = @company_id
  AND id = @id;

-- Active-scope semantics:
--   * valid_from is inclusive;
--   * valid_to is exclusive;
--   * a NULL branch assignment is company-wide;
--   * a branch-scoped assignment is usable only for that branch;
--   * a company-level lookup never widens to a branch-scoped assignment.
-- Legacy user_roles are intentionally not included here. They remain available
-- through UserEffectivePermissions for compatibility with global callers.
-- name: RbacEffectivePermissionsInScope :many
SELECT DISTINCT p.name
FROM rbac_user_role_assignments ura
JOIN role_permissions rp ON rp.role_id = ura.role_id
JOIN permissions p ON p.id = rp.permission_id
WHERE ura.user_id = @user_id
  AND ura.company_id = @company_id
  AND ura.valid_from <= @at::timestamptz
  AND (ura.valid_to IS NULL OR @at::timestamptz < ura.valid_to)
  AND (
      (sqlc.narg(branch_id)::bigint IS NULL AND ura.branch_id IS NULL)
      OR
      (sqlc.narg(branch_id)::bigint IS NOT NULL
          AND (ura.branch_id IS NULL OR ura.branch_id = sqlc.narg(branch_id)::bigint))
  )
  AND (
      sqlc.narg(branch_id)::bigint IS NULL
      OR EXISTS (
          SELECT 1
          FROM branches b
          WHERE b.id = sqlc.narg(branch_id)::bigint
            AND b.company_id = @company_id
      )
  )
ORDER BY p.name;

-- name: RbacOpenAccessReview :one
INSERT INTO rbac_access_reviews (
    company_id,
    subject_user_id,
    review_key,
    opened_by_user_id
)
VALUES (@company_id, @subject_user_id, @review_key, @opened_by_user_id)
ON CONFLICT (company_id, subject_user_id, review_key)
DO UPDATE SET review_key = EXCLUDED.review_key
RETURNING id, company_id, subject_user_id, review_key, status, decision,
          opened_by_user_id, decided_by_user_id, decided_at, created_at, updated_at;

-- name: RbacGetAccessReview :one
SELECT id, company_id, subject_user_id, review_key, status, decision,
       opened_by_user_id, decided_by_user_id, decided_at, created_at, updated_at
FROM rbac_access_reviews
WHERE company_id = @company_id
  AND id = @id;

-- name: RbacListOpenAccessReviews :many
SELECT id, company_id, subject_user_id, review_key, status, decision,
       opened_by_user_id, decided_by_user_id, decided_at, created_at, updated_at
FROM rbac_access_reviews
WHERE company_id = @company_id
  AND status = 'OPEN'
ORDER BY created_at ASC, id ASC;

-- name: RbacCompleteAccessReview :one
UPDATE rbac_access_reviews
SET status = 'COMPLETED',
    decision = @decision,
    decided_by_user_id = @decided_by_user_id,
    decided_at = NOW(),
    updated_at = NOW()
WHERE company_id = @company_id
  AND id = @id
  AND status = 'OPEN'
RETURNING id, company_id, subject_user_id, review_key, status, decision,
          opened_by_user_id, decided_by_user_id, decided_at, created_at, updated_at;
