DELETE FROM role_permissions rp
USING roles r, permissions p
WHERE rp.role_id = r.id
  AND rp.permission_id = p.id
  AND LOWER(TRIM(r.name)) IN ('admin', 'administrator')
  AND p.name IN (
      'procurement.contract.view',
      'procurement.contract.manage',
      'procurement.contract.create',
      'procurement.contract.submit',
      'procurement.contract.approve',
      'procurement.contract.terminate',
      'procurement.supplier_rating.view',
      'procurement.supplier_rating.create',
      'procurement.supplier_rating.publish',
      'procurement.price_history.view',
      'procurement.variance.view',
      'procurement.variance.approve'
  );
