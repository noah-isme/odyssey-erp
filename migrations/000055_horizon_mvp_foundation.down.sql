DROP TABLE IF EXISTS portal_users;
DROP TABLE IF EXISTS webhook_deliveries;
DROP TABLE IF EXISTS webhook_subscriptions;
DROP TABLE IF EXISTS api_key_scopes;
DROP TABLE IF EXISTS api_keys;
DROP TABLE IF EXISTS timesheets;
DROP TABLE IF EXISTS project_members;
DROP TABLE IF EXISTS project_tasks;
DROP TABLE IF EXISTS projects;
DROP TABLE IF EXISTS pos_payments;
DROP TABLE IF EXISTS pos_ticket_lines;
DROP TABLE IF EXISTS pos_tickets;
DROP TABLE IF EXISTS pos_sessions;
DROP TABLE IF EXISTS pos_terminals;
DROP TABLE IF EXISTS mrp_work_orders;
DROP TABLE IF EXISTS mrp_bom_lines;
DROP TABLE IF EXISTS mrp_boms;
DROP TABLE IF EXISTS wms_pick_scans;
DROP TABLE IF EXISTS wms_pick_tasks;
DROP TABLE IF EXISTS wms_pick_waves;
DROP TABLE IF EXISTS wms_barcode_aliases;
DROP TABLE IF EXISTS wms_bins;
DROP TABLE IF EXISTS horizon_idempotency_keys;

DELETE FROM role_permissions rp
USING roles r, permissions p
WHERE rp.role_id = r.id AND rp.permission_id = p.id
  AND LOWER(TRIM(r.name)) IN ('admin', 'administrator')
  AND p.name IN ('wms.view','wms.manage','mrp.view','mrp.manage','pos.view','pos.manage',
                 'projects.view','projects.manage','api.manage','webhooks.manage','portal.manage');

DELETE FROM permissions
WHERE name IN ('wms.view','wms.manage','mrp.view','mrp.manage','pos.view','pos.manage',
               'projects.view','projects.manage','api.manage','webhooks.manage','portal.manage');
