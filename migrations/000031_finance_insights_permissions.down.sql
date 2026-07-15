DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id
    FROM permissions
    WHERE name IN ('finance.view_insights', 'finance.export_insights')
);

DELETE FROM permissions
WHERE name IN ('finance.view_insights', 'finance.export_insights');
