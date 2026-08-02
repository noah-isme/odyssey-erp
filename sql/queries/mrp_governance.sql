-- =============================================================================
-- POLICY VERSIONS
-- =============================================================================

-- name: CreatePolicyVersion :one
INSERT INTO policy_versions (
    company_id, record_type, decision_name, effective_from, effective_to,
    enforcement_mode, signature_required, approver_roles, separation_of_duties,
    required_evidence, retention_period_days, version, status, created_by
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
RETURNING id, company_id, record_type, decision_name, effective_from, effective_to,
          enforcement_mode, signature_required, approver_roles, separation_of_duties,
          required_evidence, retention_period_days, version, status, created_at, created_by;

-- name: GetActivePolicyForDecision :one
SELECT id, company_id, record_type, decision_name, effective_from, effective_to,
       enforcement_mode, signature_required, approver_roles, separation_of_duties,
       required_evidence, retention_period_days, version, status, created_at, created_by
FROM policy_versions
WHERE company_id = $1
  AND record_type = $2
  AND decision_name = $3
  AND status = 'ACTIVE'
  AND effective_from <= $4::timestamptz
  AND (effective_to IS NULL OR effective_to > $4::timestamptz)
ORDER BY effective_from DESC
LIMIT 1;

-- name: ListPoliciesByCompany :many
SELECT id, company_id, record_type, decision_name, effective_from, effective_to,
       enforcement_mode, signature_required, approver_roles, separation_of_duties,
       required_evidence, retention_period_days, version, status, created_at, created_by
FROM policy_versions
WHERE company_id = $1
ORDER BY record_type, decision_name, effective_from DESC;

-- name: UpdatePolicyStatus :exec
UPDATE policy_versions
SET status = $1
WHERE id = $2;

-- =============================================================================
-- COMPLIANCE DECISIONS
-- =============================================================================

-- name: CreateComplianceDecision :one
INSERT INTO compliance_decisions (
    company_id, policy_version_id, record_type, record_id, action,
    actor_id, reason, decision_id, record_version, record_hash
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING id, company_id, policy_version_id, record_type, record_id, action,
          actor_id, reason, decision_id, record_version, record_hash, created_at;

-- name: GetComplianceDecision :one
SELECT id, company_id, policy_version_id, record_type, record_id, action,
       actor_id, reason, decision_id, record_version, record_hash, created_at
FROM compliance_decisions
WHERE id = $1;

-- name: GetComplianceDecisionByUUID :one
SELECT id, company_id, policy_version_id, record_type, record_id, action,
       actor_id, reason, decision_id, record_version, record_hash, created_at
FROM compliance_decisions
WHERE decision_id = $1;

-- name: ListDecisionsByRecord :many
SELECT id, company_id, policy_version_id, record_type, record_id, action,
       actor_id, reason, decision_id, record_version, record_hash, created_at
FROM compliance_decisions
WHERE company_id = $1
  AND record_type = $2
  AND record_id = $3
ORDER BY created_at DESC;

-- name: ListDecisionsByCompany :many
SELECT id, company_id, policy_version_id, record_type, record_id, action,
       actor_id, reason, decision_id, record_version, record_hash, created_at
FROM compliance_decisions
WHERE company_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- =============================================================================
-- SIGNATURE CHALLENGES
-- =============================================================================

-- name: CreateSignatureChallenge :one
INSERT INTO signature_challenges (
    challenge_id, policy_version_id, record_id, record_version,
    expiry, reauthentication_required
)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, challenge_id, policy_version_id, record_id, record_version,
          expiry, reauthentication_required, used, created_at;

-- name: GetSignatureChallenge :one
SELECT id, challenge_id, policy_version_id, record_id, record_version,
       expiry, reauthentication_required, used, created_at
FROM signature_challenges
WHERE challenge_id = $1;

-- name: MarkChallengeUsed :exec
UPDATE signature_challenges
SET used = TRUE
WHERE id = $1;

-- name: ListExpiredChallenges :many
SELECT id, challenge_id, policy_version_id, record_id, record_version,
       expiry, reauthentication_required, used, created_at
FROM signature_challenges
WHERE expiry < NOW()
  AND used = FALSE
ORDER BY expiry DESC
LIMIT $1;

-- =============================================================================
-- EVIDENCE RECORDS
-- =============================================================================

-- name: CreateEvidenceRecord :one
INSERT INTO evidence_records (decision_id, evidence_type, content)
VALUES ($1, $2, $3)
RETURNING id, decision_id, evidence_type, content, created_at;

-- name: GetEvidenceRecordsForDecision :many
SELECT id, decision_id, evidence_type, content, created_at
FROM evidence_records
WHERE decision_id = $1
ORDER BY created_at DESC;

-- =============================================================================
-- AUDIT EVENTS
-- =============================================================================

-- name: CreateAuditEvent :one
INSERT INTO audit_events (
    company_id, correlation_id, causation_id, decision_id,
    entity_type, entity_id, action, actor_id, details
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING id, company_id, correlation_id, causation_id, decision_id,
          entity_type, entity_id, action, actor_id, details, created_at;

-- name: GetAuditEvent :one
SELECT id, company_id, correlation_id, causation_id, decision_id,
       entity_type, entity_id, action, actor_id, details, created_at
FROM audit_events
WHERE id = $1;

-- name: ListAuditEventsByCorrelation :many
SELECT id, company_id, correlation_id, causation_id, decision_id,
       entity_type, entity_id, action, actor_id, details, created_at
FROM audit_events
WHERE correlation_id = $1
ORDER BY created_at ASC;

-- name: ListAuditEventsByCompany :many
SELECT id, company_id, correlation_id, causation_id, decision_id,
       entity_type, entity_id, action, actor_id, details, created_at
FROM audit_events
WHERE company_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- =============================================================================
-- QUALITY INSPECTIONS
-- =============================================================================

-- name: CreateQualityInspection :one
INSERT INTO quality_inspections (
    company_id, product_id, work_order_id, operation_id,
    inspection_plan_id, status, result_snapshot, result_version
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING id, company_id, product_id, work_order_id, operation_id,
          inspection_plan_id, status, result_snapshot, result_version, created_at, updated_at;

-- name: GetQualityInspection :one
SELECT id, company_id, product_id, work_order_id, operation_id,
       inspection_plan_id, status, result_snapshot, result_version, created_at, updated_at
FROM quality_inspections
WHERE id = $1;

-- name: ListInspectionsByWorkOrder :many
SELECT id, company_id, product_id, work_order_id, operation_id,
       inspection_plan_id, status, result_snapshot, result_version, created_at, updated_at
FROM quality_inspections
WHERE work_order_id = $1
ORDER BY created_at DESC;

-- name: UpdateInspectionStatus :exec
UPDATE quality_inspections
SET status = $1, result_snapshot = $2, result_version = $3, updated_at = NOW()
WHERE id = $4;

-- =============================================================================
-- QUALITY HOLDS
-- =============================================================================

-- name: CreateQualityHold :one
INSERT INTO quality_holds (company_id, inspection_id, record_type, record_id, status, created_by)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, company_id, inspection_id, record_type, record_id, status, created_by, created_at, updated_at;

-- name: GetQualityHold :one
SELECT id, company_id, inspection_id, record_type, record_id, status, created_by, created_at, updated_at
FROM quality_holds
WHERE id = $1;

-- name: ListOpenHolds :many
SELECT id, company_id, inspection_id, record_type, record_id, status, created_by, created_at, updated_at
FROM quality_holds
WHERE company_id = $1 AND status = 'OPEN'
ORDER BY created_at DESC;

-- name: UpdateHoldStatus :exec
UPDATE quality_holds
SET status = $1, updated_at = NOW()
WHERE id = $2;

-- =============================================================================
-- QUALITY NCRs
-- =============================================================================

-- name: CreateQualityNCR :one
INSERT INTO quality_ncrs (company_id, number, status, created_by)
VALUES ($1, $2, $3, $4)
RETURNING id, company_id, number, status, created_by, created_at, updated_at;

-- name: GetQualityNCR :one
SELECT id, company_id, number, status, created_by, created_at, updated_at
FROM quality_ncrs
WHERE id = $1;

-- name: ListNCRsByCompany :many
SELECT id, company_id, number, status, created_by, created_at, updated_at
FROM quality_ncrs
WHERE company_id = $1
ORDER BY created_at DESC;

-- name: UpdateNCRStatus :exec
UPDATE quality_ncrs
SET status = $1, updated_at = NOW()
WHERE id = $2;

-- =============================================================================
-- QUALITY CAPAs
-- =============================================================================

-- name: CreateQualityCAPA :one
INSERT INTO quality_capas (company_id, number, status, created_by)
VALUES ($1, $2, $3, $4)
RETURNING id, company_id, number, status, created_by, created_at, updated_at;

-- name: GetQualityCAPA :one
SELECT id, company_id, number, status, created_by, created_at, updated_at
FROM quality_capas
WHERE id = $1;

-- name: ListCAPAsByCompany :many
SELECT id, company_id, number, status, created_by, created_at, updated_at
FROM quality_capas
WHERE company_id = $1
ORDER BY created_at DESC;

-- name: UpdateCAPAStatus :exec
UPDATE quality_capas
SET status = $1, updated_at = NOW()
WHERE id = $2;

-- =============================================================================
-- SUBCONTRACT RECEIPTS
-- =============================================================================

-- name: CreateSubcontractReceipt :one
INSERT INTO subcontract_receipts (company_id, work_order_id, operation_id, status, sent_qty)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, company_id, work_order_id, operation_id, status, sent_qty, received_qty, created_at, updated_at;

-- name: GetSubcontractReceipt :one
SELECT id, company_id, work_order_id, operation_id, status, sent_qty, received_qty, created_at, updated_at
FROM subcontract_receipts
WHERE id = $1;

-- name: ListSubcontractsByWorkOrder :many
SELECT id, company_id, work_order_id, operation_id, status, sent_qty, received_qty, created_at, updated_at
FROM subcontract_receipts
WHERE work_order_id = $1
ORDER BY created_at DESC;

-- name: UpdateSubcontractReceiptStatus :exec
UPDATE subcontract_receipts
SET status = $1, received_qty = $2, updated_at = NOW()
WHERE id = $3;
