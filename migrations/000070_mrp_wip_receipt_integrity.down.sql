DROP TABLE IF EXISTS mrp_production_receipt_costs;
DROP TRIGGER IF EXISTS trg_validate_mrp_wip_location ON mrp_wip_locations;
DROP FUNCTION IF EXISTS validate_mrp_wip_location();
DROP INDEX IF EXISTS idx_mrp_wip_locations_one_default;
