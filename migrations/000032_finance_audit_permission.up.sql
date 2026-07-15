INSERT INTO permissions (name, description)
VALUES ('finance.view_audit', 'View Finance Audit Timeline')
ON CONFLICT (name) DO UPDATE
SET description = EXCLUDED.description;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
CROSS JOIN permissions p
WHERE LOWER(r.name) = 'admin'
  AND p.name = 'finance.view_audit'
ON CONFLICT DO NOTHING;
