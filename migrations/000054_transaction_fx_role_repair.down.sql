DELETE FROM role_permissions rp
USING roles r, permissions p
WHERE rp.role_id = r.id
  AND rp.permission_id = p.id
  AND LOWER(TRIM(r.name)) IN ('admin', 'administrator', 'finance manager', 'finance user')
  AND p.name IN ('finance.fx.view', 'finance.fx.manage', 'finance.fx.revalue', 'finance.fx.override');
