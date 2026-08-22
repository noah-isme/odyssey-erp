-- v0.11-finance payment operations workbench read permission. Recovery
-- mutations remain guarded by finance.payment.execute.
INSERT INTO permissions (name, description)
VALUES ('finance.payment.view', 'View payment execution and settlement operations')
ON CONFLICT (name) DO UPDATE SET description = EXCLUDED.description;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.name = 'finance.payment.view'
WHERE LOWER(TRIM(r.name)) IN ('admin', 'administrator', 'finance manager', 'finance user')
ON CONFLICT DO NOTHING;
