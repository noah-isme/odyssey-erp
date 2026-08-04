-- Rollback: Phase 5 - Distribution Planning

-- BEGIN;

-- Drop indexes
DROP INDEX IF EXISTS idx_transfer_order_lines_transfer;
DROP INDEX IF EXISTS idx_transfer_orders_load;
DROP INDEX IF EXISTS idx_transfer_orders_from_to_warehouse;
DROP INDEX IF EXISTS idx_transfer_orders_company_status;
DROP INDEX IF EXISTS idx_route_stops_route;
DROP INDEX IF EXISTS idx_delivery_routes_load;
DROP INDEX IF EXISTS idx_delivery_routes_company_status;
DROP INDEX IF EXISTS idx_load_items_load;
DROP INDEX IF EXISTS idx_loads_carrier;
DROP INDEX IF EXISTS idx_loads_vehicle_driver;
DROP INDEX IF EXISTS idx_loads_origin_destination;
DROP INDEX IF EXISTS idx_loads_company_status;
DROP INDEX IF EXISTS idx_planning_rules_company_warehouse;
DROP INDEX IF EXISTS idx_planning_horizons_company_warehouse;

-- Drop tables in reverse order of dependencies
DROP TABLE IF EXISTS transfer_order_lines CASCADE;
DROP TABLE IF EXISTS transfer_orders CASCADE;
DROP TABLE IF EXISTS route_stops CASCADE;
DROP TABLE IF EXISTS delivery_routes CASCADE;
DROP TABLE IF EXISTS load_items CASCADE;
DROP TABLE IF EXISTS loads CASCADE;
DROP TABLE IF EXISTS planning_rules CASCADE;
DROP TABLE IF EXISTS planning_horizons CASCADE;

-- Remove RBAC permissions
DELETE FROM role_permissions WHERE permission_id IN (
    SELECT id FROM permissions WHERE name LIKE 'distribution.%'
);

DELETE FROM permissions WHERE name LIKE 'distribution.%';

-- COMMIT;
