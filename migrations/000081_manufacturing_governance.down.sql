-- Rollback Manufacturing Governance schema

DROP TABLE IF EXISTS subcontract_receipts CASCADE;
DROP TABLE IF EXISTS quality_capas CASCADE;
DROP TABLE IF EXISTS quality_ncrs CASCADE;
DROP TABLE IF EXISTS quality_holds CASCADE;
DROP TABLE IF EXISTS quality_inspections CASCADE;
DROP TABLE IF EXISTS audit_events CASCADE;
DROP TABLE IF EXISTS evidence_records CASCADE;
DROP TABLE IF EXISTS signature_challenges CASCADE;
DROP TABLE IF EXISTS compliance_decisions CASCADE;
DROP TABLE IF EXISTS policy_versions CASCADE;
