-- 000124_scoped_rbac_global_compatibility.up.sql
--
-- Preserve the historical global user_roles behavior when scoped permission
-- checks become the production path. There is no user/company membership table
-- in the legacy schema, so each global assignment is materialized as a
-- company-wide assignment for every existing company. A fixed epoch start is
-- used as the migration marker: it is active for the lifetime of the tenant,
-- and the existing unique scope index makes this insert idempotent.
INSERT INTO permissions (name, description)
VALUES
    ('permissions.assign', 'Assign roles in a company or branch scope'),
    ('permissions.review', 'Open and decide access reviews')
ON CONFLICT (name) DO UPDATE SET description = EXCLUDED.description;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
CROSS JOIN permissions p
WHERE LOWER(TRIM(r.name)) IN ('admin', 'administrator')
  AND p.name IN ('permissions.assign', 'permissions.review')
ON CONFLICT DO NOTHING;

INSERT INTO rbac_user_role_assignments (
    company_id,
    user_id,
    role_id,
    branch_id,
    valid_from,
    valid_to
)
SELECT
    c.id,
    ur.user_id,
    ur.role_id,
    NULL,
    TIMESTAMPTZ '1970-01-01 00:00:00+00',
    NULL
FROM companies c
CROSS JOIN user_roles ur
ON CONFLICT (company_id, user_id, role_id, valid_from)
    WHERE branch_id IS NULL
DO NOTHING;
