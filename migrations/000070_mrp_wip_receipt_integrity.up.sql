-- Keep 000069 compatible; this migration closes its nullable-unique/default gap.
CREATE UNIQUE INDEX IF NOT EXISTS idx_mrp_wip_locations_one_default
    ON mrp_wip_locations(company_id, warehouse_id) WHERE work_center_id IS NULL;

CREATE OR REPLACE FUNCTION validate_mrp_wip_location()
RETURNS trigger AS $$
BEGIN
  IF NEW.warehouse_id = NEW.wip_warehouse_id THEN RAISE EXCEPTION 'WIP warehouse must differ from source warehouse'; END IF;
  IF NOT EXISTS (SELECT 1 FROM warehouses w JOIN branches b ON b.id=w.branch_id WHERE w.id=NEW.warehouse_id AND b.company_id=NEW.company_id)
     OR NOT EXISTS (SELECT 1 FROM warehouses w JOIN branches b ON b.id=w.branch_id WHERE w.id=NEW.wip_warehouse_id AND b.company_id=NEW.company_id) THEN
    RAISE EXCEPTION 'WIP warehouses must belong to company';
  END IF;
  IF NEW.work_center_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM mrp_work_centers wc WHERE wc.id=NEW.work_center_id AND wc.company_id=NEW.company_id) THEN
    RAISE EXCEPTION 'work center must belong to company';
  END IF;
  RETURN NEW;
END; $$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS trg_validate_mrp_wip_location ON mrp_wip_locations;
CREATE TRIGGER trg_validate_mrp_wip_location BEFORE INSERT OR UPDATE ON mrp_wip_locations FOR EACH ROW EXECUTE FUNCTION validate_mrp_wip_location();

CREATE TABLE IF NOT EXISTS mrp_production_receipt_costs (
 id BIGSERIAL PRIMARY KEY,
 company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
 production_event_id BIGINT NOT NULL REFERENCES mrp_production_events(id) ON DELETE RESTRICT,
 component_product_id BIGINT NOT NULL REFERENCES products(id) ON DELETE RESTRICT,
 quantity NUMERIC(18,6) NOT NULL CHECK(quantity>0),
 unit_cost NUMERIC(18,6) NOT NULL CHECK(unit_cost>=0),
 extended_amount NUMERIC(18,6) NOT NULL CHECK(extended_amount>=0),
 UNIQUE(production_event_id, component_product_id)
);
