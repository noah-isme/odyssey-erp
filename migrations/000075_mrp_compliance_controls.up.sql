CREATE TABLE IF NOT EXISTS mrp_controlled_record_policies (
 id BIGSERIAL PRIMARY KEY,company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,record_type TEXT NOT NULL,
 requires_signature BOOLEAN NOT NULL DEFAULT FALSE,retention_days INT NOT NULL DEFAULT 2555 CHECK(retention_days>0),active BOOLEAN NOT NULL DEFAULT TRUE,
 UNIQUE(company_id,record_type));
CREATE TABLE IF NOT EXISTS mrp_electronic_signatures (
 id BIGSERIAL PRIMARY KEY,company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,record_type TEXT NOT NULL,record_id BIGINT NOT NULL,
 record_version TEXT NOT NULL,record_hash TEXT NOT NULL,meaning TEXT NOT NULL,signer_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
 reauthentication_evidence TEXT NOT NULL,created_at TIMESTAMPTZ NOT NULL DEFAULT NOW());
CREATE TABLE IF NOT EXISTS mrp_audit_events (
 id BIGSERIAL PRIMARY KEY,company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,record_type TEXT NOT NULL,record_id BIGINT NOT NULL,
 event_type TEXT NOT NULL,actor_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,detail JSONB NOT NULL DEFAULT '{}'::JSONB,created_at TIMESTAMPTZ NOT NULL DEFAULT NOW());
CREATE OR REPLACE FUNCTION prevent_mrp_audit_mutation() RETURNS trigger AS $$ BEGIN RAISE EXCEPTION 'MRP audit events are immutable'; END; $$ LANGUAGE plpgsql;
CREATE TRIGGER trg_prevent_mrp_audit_mutation BEFORE UPDATE OR DELETE ON mrp_audit_events FOR EACH ROW EXECUTE FUNCTION prevent_mrp_audit_mutation();
INSERT INTO permissions(name,description) VALUES ('mrp.planner','Plan and resolve manufacturing exceptions'),('mrp.operator','Report production execution'),('mrp.quality.inspect','Record manufacturing inspections'),('mrp.quality.approve','Release quality holds and dispositions'),('mrp.manager','Approve manufacturing controlled decisions') ON CONFLICT(name) DO UPDATE SET description=EXCLUDED.description;
INSERT INTO role_permissions(role_id,permission_id) SELECT r.id,p.id FROM roles r JOIN permissions p ON p.name IN ('mrp.planner','mrp.operator','mrp.quality.inspect','mrp.quality.approve','mrp.manager') WHERE LOWER(TRIM(r.name)) IN ('admin','administrator') ON CONFLICT DO NOTHING;
