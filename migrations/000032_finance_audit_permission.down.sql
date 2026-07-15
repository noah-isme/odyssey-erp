DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id
    FROM permissions
    WHERE name = 'finance.view_audit'
);

DELETE FROM permissions
WHERE name = 'finance.view_audit';
