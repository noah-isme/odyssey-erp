-- Repair FX role grants for databases where 000053 was already applied with
-- case-sensitive role matching. This migration is additive and idempotent.
INSERT INTO permissions (name, description) VALUES
    ('finance.fx.view', 'View transaction FX rates and valuation results'),
    ('finance.fx.manage', 'Manage transaction FX configuration and fetches'),
    ('finance.fx.revalue', 'Execute FX revaluation'),
    ('finance.fx.override', 'Approve manual FX rate overrides')
ON CONFLICT (name) DO UPDATE SET description = EXCLUDED.description;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
CROSS JOIN permissions p
WHERE LOWER(TRIM(r.name)) IN ('admin', 'administrator', 'finance manager')
  AND p.name IN ('finance.fx.view', 'finance.fx.manage', 'finance.fx.revalue', 'finance.fx.override')
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
CROSS JOIN permissions p
WHERE LOWER(TRIM(r.name)) = 'finance user'
  AND p.name = 'finance.fx.view'
ON CONFLICT DO NOTHING;
