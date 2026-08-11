-- Ensure upgraded installations grant the Phase 3 procurement permissions to
-- administrator roles. The original Phase 3 migration created these
-- permissions but did not assign every new permission to existing admins.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.name IN (
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
)
WHERE LOWER(TRIM(r.name)) IN ('admin', 'administrator')
ON CONFLICT DO NOTHING;
