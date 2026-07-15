INSERT INTO permissions (name, description) VALUES
    ('finance.view_insights', 'View Finance Insights'),
    ('finance.export_insights', 'Export Finance Insights')
ON CONFLICT (name) DO UPDATE
SET description = EXCLUDED.description;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
CROSS JOIN permissions p
WHERE LOWER(r.name) = 'admin'
  AND p.name IN ('finance.view_insights', 'finance.export_insights')
ON CONFLICT DO NOTHING;
