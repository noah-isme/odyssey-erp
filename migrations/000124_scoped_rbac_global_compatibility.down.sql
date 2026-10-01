-- Remove only the deterministic compatibility rows created by 000124. The
-- predicate also requires the legacy assignment to remain present, avoiding
-- removal of a later, intentionally retained scoped assignment if a global
-- role was revoked after the migration.
DELETE FROM rbac_user_role_assignments ura
USING user_roles ur
WHERE ura.user_id = ur.user_id
  AND ura.role_id = ur.role_id
  AND ura.branch_id IS NULL
  AND ura.valid_from = TIMESTAMPTZ '1970-01-01 00:00:00+00';
