DROP TABLE IF EXISTS mrp_material_movements;
DROP TABLE IF EXISTS mrp_wip_locations;
DROP TABLE IF EXISTS mrp_work_order_operations;
ALTER TABLE mrp_work_orders DROP COLUMN IF EXISTS routing_id;
