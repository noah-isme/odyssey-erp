-- Rollback: Phase 4 - Transport Execution

BEGIN;

-- Drop indexes
DROP INDEX IF EXISTS idx_trip_stops_shipment;
DROP INDEX IF EXISTS idx_trip_stops_trip;
DROP INDEX IF EXISTS idx_trips_driver_status;
DROP INDEX IF EXISTS idx_trips_company_vehicle_status;
DROP INDEX IF EXISTS idx_shipments_dispatch_delivery;
DROP INDEX IF EXISTS idx_shipments_carrier;
DROP INDEX IF EXISTS idx_shipments_vehicle_driver;
DROP INDEX IF EXISTS idx_shipment_lines_shipment;
DROP INDEX IF EXISTS idx_shipments_company_status;
DROP INDEX IF EXISTS idx_drivers_company_status;
DROP INDEX IF EXISTS idx_vehicles_company_fleet_status;
DROP INDEX IF EXISTS idx_fleets_company_status;
DROP INDEX IF EXISTS idx_carrier_rate_cards_company_carrier;
DROP INDEX IF EXISTS idx_carriers_company_status;

-- Drop tables in reverse order of dependencies
DROP TABLE IF EXISTS trip_stops CASCADE;
DROP TABLE IF EXISTS trips CASCADE;
DROP TABLE IF EXISTS shipment_lines CASCADE;
DROP TABLE IF EXISTS shipments CASCADE;
DROP TABLE IF EXISTS drivers CASCADE;
DROP TABLE IF EXISTS vehicles CASCADE;
DROP TABLE IF EXISTS fleets CASCADE;
DROP TABLE IF EXISTS carrier_rate_cards CASCADE;
DROP TABLE IF EXISTS carriers CASCADE;

-- Remove RBAC permissions
DELETE FROM rbac_permissions WHERE permission_code LIKE 'logistics.%';

COMMIT;
