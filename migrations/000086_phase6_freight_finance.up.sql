-- Phase 6: Freight Finance
-- Migrate up: Database schema for rate cards, freight charges, GL posting

CREATE TABLE rate_cards (
  id BIGSERIAL PRIMARY KEY,
  company_id BIGINT NOT NULL,
  carrier_id BIGINT,
  origin_city VARCHAR(100) NOT NULL,
  origin_country VARCHAR(100) NOT NULL,
  destination_city VARCHAR(100) NOT NULL,
  destination_country VARCHAR(100) NOT NULL,
  service_level VARCHAR(50) NOT NULL, -- STANDARD, EXPRESS, OVERNIGHT, ECONOMY
  min_weight NUMERIC(15,4),
  max_weight NUMERIC(15,4),
  base_rate NUMERIC(15,4) NOT NULL,
  per_kg_rate NUMERIC(15,4),
  per_cbm_rate NUMERIC(15,4),
  currency VARCHAR(3) NOT NULL DEFAULT 'USD',
  effective_date DATE NOT NULL,
  expiration_date DATE,
  is_active BOOLEAN NOT NULL DEFAULT true,
  created_by BIGINT NOT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
  CONSTRAINT fk_rate_cards_company FOREIGN KEY (company_id) REFERENCES companies(id),
  CONSTRAINT fk_rate_cards_carrier FOREIGN KEY (carrier_id) REFERENCES carriers(id)
);

CREATE INDEX idx_rate_cards_company_status ON rate_cards(company_id, is_active);
CREATE INDEX idx_rate_cards_route ON rate_cards(origin_city, destination_city, service_level);
CREATE INDEX idx_rate_cards_effective ON rate_cards(effective_date, expiration_date);

CREATE TABLE rate_surcharges (
  id BIGSERIAL PRIMARY KEY,
  company_id BIGINT NOT NULL,
  rate_card_id BIGINT NOT NULL,
  surcharge_type VARCHAR(50) NOT NULL, -- FUEL, HOLIDAY, ZONE, HANDLING, INSURANCE
  surcharge_name VARCHAR(100) NOT NULL,
  surcharge_amount NUMERIC(15,4) NOT NULL,
  surcharge_percent NUMERIC(5,2),
  effective_date DATE NOT NULL,
  expiration_date DATE,
  created_at TIMESTAMP NOT NULL DEFAULT NOW(),
  CONSTRAINT fk_surcharges_company FOREIGN KEY (company_id) REFERENCES companies(id),
  CONSTRAINT fk_surcharges_rate_card FOREIGN KEY (rate_card_id) REFERENCES rate_cards(id) ON DELETE CASCADE
);

CREATE INDEX idx_surcharges_rate_card ON rate_surcharges(rate_card_id);

CREATE TABLE freight_charges (
  id BIGSERIAL PRIMARY KEY,
  company_id BIGINT NOT NULL,
  shipment_id BIGINT,
  load_id BIGINT,
  carrier_id BIGINT,
  rate_card_id BIGINT,
  origin_city VARCHAR(100) NOT NULL,
  destination_city VARCHAR(100) NOT NULL,
  service_level VARCHAR(50),
  weight_kg NUMERIC(15,4),
  volume_cbm NUMERIC(15,4),
  base_charge NUMERIC(15,4) NOT NULL,
  weight_charge NUMERIC(15,4) DEFAULT 0,
  volume_charge NUMERIC(15,4) DEFAULT 0,
  surcharge_total NUMERIC(15,4) DEFAULT 0,
  freight_total NUMERIC(15,4) NOT NULL,
  currency VARCHAR(3) NOT NULL DEFAULT 'USD',
  status VARCHAR(50) NOT NULL DEFAULT 'CALCULATED', -- CALCULATED, INVOICED, PAID
  invoice_number VARCHAR(100),
  invoice_date DATE,
  gl_posting_id BIGINT,
  cost_center_id BIGINT,
  notes TEXT,
  created_by BIGINT NOT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
  CONSTRAINT fk_freight_charges_company FOREIGN KEY (company_id) REFERENCES companies(id),
  CONSTRAINT fk_freight_charges_carrier FOREIGN KEY (carrier_id) REFERENCES carriers(id),
  CONSTRAINT fk_freight_charges_rate_card FOREIGN KEY (rate_card_id) REFERENCES rate_cards(id)
);

CREATE INDEX idx_freight_charges_company_status ON freight_charges(company_id, status);
CREATE INDEX idx_freight_charges_shipment ON freight_charges(shipment_id);
CREATE INDEX idx_freight_charges_load ON freight_charges(load_id);
CREATE INDEX idx_freight_charges_gl_posting ON freight_charges(gl_posting_id);

CREATE TABLE landed_costs (
  id BIGSERIAL PRIMARY KEY,
  company_id BIGINT NOT NULL,
  shipment_id BIGINT NOT NULL,
  load_id BIGINT,
  freight_charge_id BIGINT NOT NULL,
  po_id BIGINT,
  product_cost NUMERIC(15,4) NOT NULL,
  freight_cost NUMERIC(15,4) NOT NULL,
  duty_cost NUMERIC(15,4) DEFAULT 0,
  tax_cost NUMERIC(15,4) DEFAULT 0,
  insurance_cost NUMERIC(15,4) DEFAULT 0,
  other_cost NUMERIC(15,4) DEFAULT 0,
  total_landed_cost NUMERIC(15,4) NOT NULL,
  cost_per_unit NUMERIC(15,4),
  currency VARCHAR(3) NOT NULL DEFAULT 'USD',
  allocation_method VARCHAR(50) NOT NULL DEFAULT 'WEIGHT', -- WEIGHT, VOLUME, ITEM_COUNT, MANUAL
  created_at TIMESTAMP NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
  CONSTRAINT fk_landed_costs_company FOREIGN KEY (company_id) REFERENCES companies(id),
  CONSTRAINT fk_landed_costs_freight FOREIGN KEY (freight_charge_id) REFERENCES freight_charges(id)
);

CREATE INDEX idx_landed_costs_company ON landed_costs(company_id);
CREATE INDEX idx_landed_costs_shipment ON landed_costs(shipment_id);
CREATE INDEX idx_landed_costs_load ON landed_costs(load_id);


CREATE TABLE freight_audit_log (
  id BIGSERIAL PRIMARY KEY,
  company_id BIGINT NOT NULL,
  freight_charge_id BIGINT NOT NULL,
  audit_type VARCHAR(50) NOT NULL, -- CREATED, CALCULATED, INVOICED, POSTED, RECONCILED
  old_value NUMERIC(15,4),
  new_value NUMERIC(15,4),
  reason VARCHAR(255),
  user_id BIGINT NOT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT NOW(),
  CONSTRAINT fk_audit_log_company FOREIGN KEY (company_id) REFERENCES companies(id),
  CONSTRAINT fk_audit_log_freight FOREIGN KEY (freight_charge_id) REFERENCES freight_charges(id)
);

CREATE INDEX idx_audit_log_freight ON freight_audit_log(freight_charge_id);
CREATE INDEX idx_audit_log_created ON freight_audit_log(created_at);

-- Add foreign key to shipments for freight_charge_id (optional, denormalized for performance)
-- ALTER TABLE shipments ADD COLUMN freight_charge_id BIGINT REFERENCES freight_charges(id);

COMMIT;
