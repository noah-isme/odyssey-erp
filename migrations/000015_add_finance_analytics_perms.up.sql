INSERT INTO permissions (name, description) VALUES
('finance.gl.view', 'View General Ledger'),
('finance.view_analytics', 'View Finance Analytics'),
('finance.export_analytics', 'Export Finance Analytics')
ON CONFLICT (name) DO NOTHING;

DO $$
DECLARE
    role_id BIGINT;
    perm_id BIGINT;
    perm_name TEXT;
    perms TEXT[] := ARRAY['finance.gl.view', 'finance.view_analytics', 'finance.export_analytics'];
BEGIN
    SELECT id INTO role_id FROM roles WHERE name = 'admin';
    IF role_id IS NOT NULL THEN
        FOREACH perm_name IN ARRAY perms
        LOOP
            SELECT id INTO perm_id FROM permissions WHERE name = perm_name;
            INSERT INTO role_permissions (role_id, permission_id) VALUES (role_id, perm_id) ON CONFLICT DO NOTHING;
        END LOOP;
    END IF;
END $$;
