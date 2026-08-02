-- Seed data for manufacturing governance system
-- Run with: psql $PG_DSN < scripts/seed_governance.sql

BEGIN TRANSACTION;

-- Insert policy versions
INSERT INTO mrp_policy_versions (company_id, policy_name, version, description, created_by, effective_date)
VALUES
  (1, 'BOM Approval Policy', '1.0', 'Gate requires QUALITY_LEAD and ENGINEERING sign-off', 1, NOW()),
  (1, 'Work Order Release Policy', '1.0', 'Gate requires PLANNER and PRODUCTION_MANAGER approval', 1, NOW()),
  (1, 'Quality Hold Release Policy', '1.0', 'Gate requires QUALITY_MANAGER authorization', 1, NOW()),
  (1, 'NCR Disposition Policy', '1.0', 'Gate requires QUALITY_LEAD and ENGINEERING evaluation', 1, NOW()),
  (1, 'CAPA Closure Policy', '1.0', 'Gate requires QUALITY_MANAGER and PROCESS_OWNER sign-off', 1, NOW());

-- Insert actor roles (required for multi-actor gates)
INSERT INTO mrp_actor_roles (company_id, actor_id, role_name, actor_name, is_active)
VALUES
  (1, 100, 'QUALITY_LEAD', 'Alice Chen', true),
  (1, 101, 'ENGINEERING', 'Bob Smith', true),
  (1, 102, 'PLANNER', 'Carol Johnson', true),
  (1, 103, 'PRODUCTION_MANAGER', 'David Lee', true),
  (1, 104, 'QUALITY_MANAGER', 'Eva Martinez', true),
  (1, 105, 'PROCESS_OWNER', 'Frank Wilson', true);

-- Insert sample compliance decisions (pending gate approval)
INSERT INTO mrp_compliance_decisions (
  company_id,
  policy_id,
  record_type,
  record_id,
  actor_id,
  actor_role,
  decision_action,
  decision_reason,
  validation_status,
  created_at
)
VALUES
  (1, 1, 'BOM', 1, 100, 'QUALITY_LEAD', 'Approve', 'BOM structure complete and verified', 'PASSED', NOW()),
  (1, 1, 'BOM', 1, 101, 'ENGINEERING', 'Approve', 'Engineering review complete', 'PASSED', NOW()),
  (1, 2, 'WorkOrder', 1, 102, 'PLANNER', 'Release', 'Schedule confirmed', 'PASSED', NOW()),
  (1, 3, 'QualityHold', 1, 104, 'QUALITY_MANAGER', 'Release', 'Issue resolved', 'PASSED', NOW());

-- Insert signature challenges for non-repudiation
INSERT INTO mrp_signature_challenges (
  company_id,
  decision_id,
  challenge_text,
  challenge_hash,
  actor_id,
  actor_role,
  challenge_status,
  created_at,
  expires_at
)
VALUES
  (1, 1, 'Approve BOM-001 for production', 'challenge-1-hash-1000', 100, 'QUALITY_LEAD', 'PENDING', NOW(), NOW() + INTERVAL '1 day'),
  (1, 1, 'Approve BOM-001 engineering', 'challenge-1-hash-1001', 101, 'ENGINEERING', 'PENDING', NOW(), NOW() + INTERVAL '1 day'),
  (1, 2, 'Release WO-001', 'challenge-2-hash-1020', 102, 'PLANNER', 'SIGNED', NOW(), NOW() + INTERVAL '1 day'),
  (1, 3, 'Release QH-001', 'challenge-3-hash-1040', 104, 'QUALITY_MANAGER', 'SIGNED', NOW(), NOW() + INTERVAL '1 day');

-- Insert audit events (complete decision lifecycle)
INSERT INTO mrp_audit_events (
  company_id,
  record_type,
  record_id,
  event_type,
  actor_id,
  actor_role,
  event_data,
  created_at
)
VALUES
  (1, 'BOM', 1, 'decision_submitted', 100, 'QUALITY_LEAD', '{"action":"Approve","reason":"BOM structure complete"}', NOW() - INTERVAL '2 hours'),
  (1, 'BOM', 1, 'validation_passed', 100, 'QUALITY_LEAD', '{"validator":"BOMApprovalValidator","lines":5}', NOW() - INTERVAL '2 hours'),
  (1, 'BOM', 1, 'challenge_generated', 100, 'QUALITY_LEAD', '{"challenge_id":"challenge-1-hash-1000"}', NOW() - INTERVAL '2 hours'),
  (1, 'BOM', 1, 'signature_recorded', 100, 'QUALITY_LEAD', '{"signature":"sig-100-1"}', NOW() - INTERVAL '1.5 hours'),
  (1, 'BOM', 1, 'challenge_verified', 101, 'ENGINEERING', '{"challenge_verified":true}', NOW() - INTERVAL '1.5 hours'),
  (1, 'BOM', 1, 'gate_completed', 100, 'QUALITY_LEAD', '{"gate_type":"BOMApprovalGate","status":"APPROVED"}', NOW() - INTERVAL '1 hour'),
  (1, 'WorkOrder', 1, 'decision_submitted', 102, 'PLANNER', '{"action":"Release","reason":"Schedule confirmed"}', NOW() - INTERVAL '30 minutes'),
  (1, 'WorkOrder', 1, 'validation_passed', 102, 'PLANNER', '{"validator":"WorkOrderReleaseValidator"}', NOW() - INTERVAL '30 minutes'),
  (1, 'WorkOrder', 1, 'gate_completed', 102, 'PLANNER', '{"gate_type":"WorkOrderReleaseGate","status":"APPROVED"}', NOW() - INTERVAL '15 minutes');

-- Insert evidence records (immutable snapshots)
INSERT INTO mrp_evidence_records (
  company_id,
  decision_id,
  evidence_type,
  evidence_data,
  snapshot_hash,
  captured_at
)
VALUES
  (1, 1, 'BOM_STRUCTURE', '{"id":1,"lines":5,"scrap_pct":0.0,"status":"DRAFT"}', 'snapshot-hash-1', NOW() - INTERVAL '2 hours'),
  (1, 1, 'VALIDATION_RESULT', '{"valid":true,"lines_complete":true,"components_available":true}', 'snapshot-hash-2', NOW() - INTERVAL '2 hours'),
  (1, 2, 'WORKORDER_STATE', '{"id":1,"status":"PLANNED","bom_id":1,"quantity":100}', 'snapshot-hash-3', NOW() - INTERVAL '30 minutes');

-- Insert decision gates (track multi-actor approval status)
INSERT INTO mrp_decision_gates (
  company_id,
  gate_type,
  record_type,
  record_id,
  required_actors,
  signed_actors,
  gate_status,
  created_at
)
VALUES
  (1, 'BOMApprovalGate', 'BOM', 1, 2, 2, 'APPROVED', NOW() - INTERVAL '1 hour'),
  (1, 'WorkOrderReleaseGate', 'WorkOrder', 1, 2, 1, 'PENDING', NOW() - INTERVAL '30 minutes'),
  (1, 'HoldReleaseGate', 'QualityHold', 1, 1, 1, 'APPROVED', NOW() - INTERVAL '15 minutes');

-- Insert gate signatures (track who signed and when)
INSERT INTO mrp_gate_signatures (
  gate_id,
  actor_id,
  actor_role,
  decision,
  signature_data,
  signed_at
)
VALUES
  (1, 100, 'QUALITY_LEAD', 'APPROVE', 'sig-100-1', NOW() - INTERVAL '1.5 hours'),
  (1, 101, 'ENGINEERING', 'APPROVE', 'sig-101-1', NOW() - INTERVAL '1.3 hours'),
  (2, 102, 'PLANNER', 'APPROVE', 'sig-102-1', NOW() - INTERVAL '25 minutes'),
  (3, 104, 'QUALITY_MANAGER', 'APPROVE', 'sig-104-1', NOW() - INTERVAL '20 minutes');

-- Insert validation results
INSERT INTO mrp_validation_results (
  company_id,
  decision_id,
  validator_type,
  validation_status,
  validation_data,
  created_at
)
VALUES
  (1, 1, 'BOMApprovalValidator', 'PASSED', '{"lines":5,"all_components_available":true}', NOW() - INTERVAL '2 hours'),
  (1, 2, 'WorkOrderReleaseValidator', 'PASSED', '{"bom_assigned":true,"quantity_valid":true}', NOW() - INTERVAL '30 minutes'),
  (1, 3, 'HoldReleaseValidator', 'PASSED', '{"hold_reason_resolved":true}', NOW() - INTERVAL '15 minutes');

-- Insert policy attachments (supporting documents)
INSERT INTO mrp_policy_attachments (
  policy_id,
  attachment_name,
  attachment_url,
  attachment_type,
  uploaded_by,
  uploaded_at
)
VALUES
  (1, 'BOM_Approval_Procedure.pdf', '/attachments/bom-approval-v1.pdf', 'PDF', 1, NOW()),
  (2, 'WO_Release_Checklist.docx', '/attachments/wo-release-checklist.docx', 'DOCX', 1, NOW()),
  (3, 'Hold_Release_Guidelines.pdf', '/attachments/hold-release-guidelines.pdf', 'PDF', 1, NOW());

COMMIT;

-- Verify data was inserted
SELECT 'Policy Versions' as table_name, COUNT(*) as record_count FROM mrp_policy_versions
UNION ALL
SELECT 'Actor Roles', COUNT(*) FROM mrp_actor_roles
UNION ALL
SELECT 'Compliance Decisions', COUNT(*) FROM mrp_compliance_decisions
UNION ALL
SELECT 'Audit Events', COUNT(*) FROM mrp_audit_events
UNION ALL
SELECT 'Decision Gates', COUNT(*) FROM mrp_decision_gates
UNION ALL
SELECT 'Gate Signatures', COUNT(*) FROM mrp_gate_signatures;
