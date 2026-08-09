DROP TRIGGER IF EXISTS trg_prevent_mrp_signature_mutation ON mrp_electronic_signatures;
DROP TRIGGER IF EXISTS trg_prevent_mrp_audit_event_mutation ON audit_events;
DROP TRIGGER IF EXISTS trg_prevent_mrp_decision_mutation ON compliance_decisions;
DROP TRIGGER IF EXISTS trg_prevent_mrp_snapshot_mutation ON mrp_record_snapshots;
DROP FUNCTION IF EXISTS prevent_mrp_compliance_mutation();

ALTER TABLE mrp_controlled_record_policies
    DROP COLUMN IF EXISTS reauthentication_required,
    DROP COLUMN IF EXISTS approver_roles;

ALTER TABLE mrp_electronic_signatures
    DROP COLUMN IF EXISTS auth_method,
    DROP COLUMN IF EXISTS snapshot_id;

ALTER TABLE signature_challenges
    DROP COLUMN IF EXISTS reauthenticated_at,
    DROP COLUMN IF EXISTS reauthentication_method,
    DROP COLUMN IF EXISTS record_hash,
    DROP COLUMN IF EXISTS signer_id,
    DROP COLUMN IF EXISTS record_type,
    DROP COLUMN IF EXISTS company_id;

ALTER TABLE compliance_decisions
    DROP COLUMN IF EXISTS snapshot_id;

DROP TABLE IF EXISTS mrp_record_snapshots;
