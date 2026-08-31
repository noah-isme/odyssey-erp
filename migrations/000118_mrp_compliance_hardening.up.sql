-- MRP compliance hardening: canonical snapshots, actor-bound challenges, and
-- immutable evidence. Existing governance rows remain readable; new writes
-- must use the server-generated snapshot fields below.

CREATE TABLE IF NOT EXISTS mrp_record_snapshots (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    record_type TEXT NOT NULL,
    record_id BIGINT NOT NULL,
    record_version TEXT NOT NULL,
    snapshot JSONB NOT NULL,
    record_hash CHAR(64) NOT NULL CHECK (record_hash ~ '^[0-9a-f]{64}$'),
    captured_by BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    captured_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    retention_until TIMESTAMPTZ,
    UNIQUE (company_id, record_type, record_id, record_version)
);

CREATE INDEX IF NOT EXISTS idx_mrp_record_snapshots_record
    ON mrp_record_snapshots(company_id, record_type, record_id, captured_at DESC);
CREATE INDEX IF NOT EXISTS idx_mrp_record_snapshots_retention
    ON mrp_record_snapshots(retention_until)
    WHERE retention_until IS NOT NULL;

ALTER TABLE compliance_decisions
    ADD COLUMN IF NOT EXISTS snapshot_id BIGINT REFERENCES mrp_record_snapshots(id) ON DELETE RESTRICT;

ALTER TABLE signature_challenges
    ADD COLUMN IF NOT EXISTS company_id BIGINT REFERENCES companies(id) ON DELETE CASCADE,
    ADD COLUMN IF NOT EXISTS record_type TEXT,
    ADD COLUMN IF NOT EXISTS signer_id BIGINT REFERENCES users(id) ON DELETE RESTRICT,
    ADD COLUMN IF NOT EXISTS record_hash CHAR(64),
    ADD COLUMN IF NOT EXISTS reauthentication_method TEXT,
    ADD COLUMN IF NOT EXISTS reauthenticated_at TIMESTAMPTZ;

UPDATE signature_challenges sc
SET company_id = pv.company_id,
    record_type = pv.record_type
FROM policy_versions pv
WHERE pv.id = sc.policy_version_id
  AND (sc.company_id IS NULL OR sc.record_type IS NULL);

CREATE INDEX IF NOT EXISTS idx_signature_challenges_actor
    ON signature_challenges(company_id, signer_id, expiry)
    WHERE used = FALSE;

ALTER TABLE mrp_electronic_signatures
    ADD COLUMN IF NOT EXISTS snapshot_id BIGINT REFERENCES mrp_record_snapshots(id) ON DELETE RESTRICT,
    ADD COLUMN IF NOT EXISTS auth_method TEXT;

ALTER TABLE mrp_controlled_record_policies
    ADD COLUMN IF NOT EXISTS approver_roles TEXT[] NOT NULL DEFAULT ARRAY['mrp.manager']::TEXT[],
    ADD COLUMN IF NOT EXISTS reauthentication_required BOOLEAN NOT NULL DEFAULT TRUE;

CREATE OR REPLACE FUNCTION prevent_mrp_compliance_mutation() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION '% rows are immutable compliance evidence', TG_TABLE_NAME;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_prevent_mrp_snapshot_mutation ON mrp_record_snapshots;
CREATE TRIGGER trg_prevent_mrp_snapshot_mutation
    BEFORE UPDATE OR DELETE ON mrp_record_snapshots
    FOR EACH ROW EXECUTE FUNCTION prevent_mrp_compliance_mutation();

DROP TRIGGER IF EXISTS trg_prevent_mrp_decision_mutation ON compliance_decisions;
CREATE TRIGGER trg_prevent_mrp_decision_mutation
    BEFORE UPDATE OR DELETE ON compliance_decisions
    FOR EACH ROW EXECUTE FUNCTION prevent_mrp_compliance_mutation();

DROP TRIGGER IF EXISTS trg_prevent_mrp_audit_event_mutation ON audit_events;
CREATE TRIGGER trg_prevent_mrp_audit_event_mutation
    BEFORE UPDATE OR DELETE ON audit_events
    FOR EACH ROW EXECUTE FUNCTION prevent_mrp_compliance_mutation();

DROP TRIGGER IF EXISTS trg_prevent_mrp_signature_mutation ON mrp_electronic_signatures;
CREATE TRIGGER trg_prevent_mrp_signature_mutation
    BEFORE UPDATE OR DELETE ON mrp_electronic_signatures
    FOR EACH ROW EXECUTE FUNCTION prevent_mrp_compliance_mutation();
