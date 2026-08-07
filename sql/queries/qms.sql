-- =============================================================================
-- QMS NON-CONFORMANCE REPORTS
-- =============================================================================

-- name: InsertNCR :one
INSERT INTO ncrs (company_id, number, title, description, source_type, source_id, source_reference,
                  category, severity, status, detected_by, detected_at, detected_location,
                  responsible_party_id, assigned_to, target_closure_date, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
RETURNING id;

-- name: GetNCR :one
SELECT id, company_id, number, title, description, source_type, source_id, source_reference,
       category, severity, status, detected_by, detected_at, detected_location,
       responsible_party_id, assigned_to, target_closure_date, actual_closure_date,
       root_cause, containment_action, created_by, created_at, updated_at
FROM ncrs
WHERE id = $1;

-- name: ListNCRs :many
SELECT id, company_id, number, title, description, source_type, source_id, source_reference,
       category, severity, status, detected_by, detected_at, detected_location,
       responsible_party_id, assigned_to, target_closure_date, actual_closure_date,
       root_cause, containment_action, created_by, created_at, updated_at
FROM ncrs
WHERE company_id = $1
  AND ($2::text IS NULL OR source_type = $2)
  AND ($3::text IS NULL OR category = $3)
  AND ($4::text IS NULL OR severity = $4)
  AND ($5::text IS NULL OR status = $5)
  AND ($6::int8 IS NULL OR assigned_to = $6)
  AND ($7::timestamptz IS NULL OR detected_at >= $7)
  AND ($8::timestamptz IS NULL OR detected_at <= $8)
ORDER BY detected_at DESC
LIMIT $9 OFFSET $10;

-- name: UpdateNCR :exec
UPDATE ncrs
SET title = $2, description = $3, category = $4, severity = $5,
    responsible_party_id = $6, assigned_to = $7, target_closure_date = $8,
    root_cause = $9, containment_action = $10, updated_at = NOW()
WHERE id = $1;

-- name: QMSUpdateNCRStatus :exec
UPDATE ncrs
SET status = $2, updated_at = NOW()
WHERE id = $1;

-- name: CountNCRsWithPrefix :one
SELECT COUNT(*) FROM ncrs WHERE company_id = $1 AND number LIKE $2 || '%';

-- =============================================================================
-- NCR DISPOSITIONS
-- =============================================================================

-- name: InsertNCRDisposition :one
INSERT INTO ncr_dispositions (ncr_id, disposition_type, description, approved_by, approved_at)
VALUES ($1, $2, $3, $4, $5)
RETURNING id;

-- name: GetNCRDisposition :one
SELECT id, ncr_id, disposition_type, description, approved_by, approved_at, created_at
FROM ncr_dispositions
WHERE ncr_id = $1
ORDER BY created_at DESC
LIMIT 1;

-- =============================================================================
-- CORRECTIVE ACTIONS (CAPA)
-- =============================================================================

-- name: InsertCAPA :one
INSERT INTO capas (company_id, number, title, description, source_type, source_id, source_reference,
                   status, priority, owner_id, team_members, root_cause_method, target_date, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
RETURNING id;

-- name: GetCAPA :one
SELECT id, company_id, number, title, description, source_type, source_id, source_reference,
       status, priority, owner_id, team_members, root_cause, root_cause_method,
       corrective_action, preventive_action, verification_method, verification_result,
       effectiveness_check, target_date, completion_date, effectiveness_date,
       created_by, created_at, updated_at
FROM capas
WHERE id = $1;

-- name: ListCAPAs :many
SELECT id, company_id, number, title, description, source_type, source_id, source_reference,
       status, priority, owner_id, team_members, root_cause, root_cause_method,
       corrective_action, preventive_action, verification_method, verification_result,
       effectiveness_check, target_date, completion_date, effectiveness_date,
       created_by, created_at, updated_at
FROM capas
WHERE company_id = $1
  AND ($2::text IS NULL OR source_type = $2)
  AND ($3::text IS NULL OR status = $3)
  AND ($4::text IS NULL OR priority = $4)
  AND ($5::int8 IS NULL OR owner_id = $5)
  AND ($6::timestamptz IS NULL OR created_at >= $6)
  AND ($7::timestamptz IS NULL OR created_at <= $7)
ORDER BY created_at DESC
LIMIT $8 OFFSET $9;

-- name: UpdateCAPA :exec
UPDATE capas
SET title = $2, description = $3, priority = $4, owner_id = $5, team_members = $6,
    root_cause = $7, root_cause_method = $8, corrective_action = $9, preventive_action = $10,
    verification_method = $11, verification_result = $12, effectiveness_check = $13,
    target_date = $14, updated_at = NOW()
WHERE id = $1;

-- name: QMSUpdateCAPAStatus :exec
UPDATE capas
SET status = $2, updated_at = NOW()
WHERE id = $1;

-- name: CountCAPAsWithPrefix :one
SELECT COUNT(*) FROM capas WHERE company_id = $1 AND number LIKE $2 || '%';

-- =============================================================================
-- AUDITS
-- =============================================================================

-- name: InsertAudit :one
INSERT INTO audits (company_id, number, title, description, audit_type, status, standard,
                    scope, lead_auditor_id, audit_team_ids, auditee_id,
                    planned_start, planned_end, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
RETURNING id;

-- name: GetAudit :one
SELECT id, company_id, number, title, description, audit_type, status, standard,
       scope, lead_auditor_id, audit_team_ids, auditee_id,
       planned_start, planned_end, actual_start, actual_end,
       report_number, report_date, created_by, created_at, updated_at
FROM audits
WHERE id = $1;

-- name: ListAudits :many
SELECT id, company_id, number, title, description, audit_type, status, standard,
       scope, lead_auditor_id, audit_team_ids, auditee_id,
       planned_start, planned_end, actual_start, actual_end,
       report_number, report_date, created_by, created_at, updated_at
FROM audits
WHERE company_id = $1
  AND ($2::text IS NULL OR status = $2)
  AND ($3::text IS NULL OR audit_type = $3)
ORDER BY planned_start DESC NULLS LAST
LIMIT $4 OFFSET $5;

-- name: UpdateAudit :exec
UPDATE audits
SET title = $2, description = $3, scope = $4, lead_auditor_id = $5,
    audit_team_ids = $6, auditee_id = $7, planned_start = $8, planned_end = $9, updated_at = NOW()
WHERE id = $1;

-- name: CountAuditsWithPrefix :one
SELECT COUNT(*) FROM audits WHERE company_id = $1 AND number LIKE $2 || '%';

-- =============================================================================
-- AUDIT FINDINGS
-- =============================================================================

-- name: InsertAuditFinding :one
INSERT INTO audit_findings (audit_id, finding_number, category, clause, description,
                            evidence, requirement, risk_level, status, assigned_to, response_due_date)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING id;

-- name: GetAuditFinding :one
SELECT id, audit_id, finding_number, category, clause, description, evidence,
       requirement, risk_level, status, response, response_due_date, response_date,
       assigned_to, verified_by, verified_at, created_at, updated_at
FROM audit_findings
WHERE id = $1;

-- name: ListAuditFindings :many
SELECT id, audit_id, finding_number, category, clause, description, evidence,
       requirement, risk_level, status, response, response_due_date, response_date,
       assigned_to, verified_by, verified_at, created_at, updated_at
FROM audit_findings
WHERE audit_id = $1
ORDER BY created_at;

-- name: UpdateAuditFinding :exec
UPDATE audit_findings
SET category = $2, description = $3, evidence = $4, requirement = $5, risk_level = $6,
    status = $7, response = $8, response_due_date = $9, assigned_to = $10,
    verified_by = $11, verified_at = $12, updated_at = NOW()
WHERE id = $1;

-- =============================================================================
-- SUPPLIER QUALITY
-- =============================================================================

-- name: InsertSupplierQuality :one
INSERT INTO supplier_quality (company_id, supplier_id, status, quality_rating, risk_level,
                              approved_date, expiry_date, last_audit_date, next_audit_date, notes, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING id;

-- name: GetSupplierQuality :one
SELECT id, company_id, supplier_id, status, quality_rating, risk_level,
       approved_date, expiry_date, last_audit_date, next_audit_date, notes, created_by, created_at, updated_at
FROM supplier_quality
WHERE id = $1;

-- name: GetSupplierQualityBySupplier :one
SELECT id, company_id, supplier_id, status, quality_rating, risk_level,
       approved_date, expiry_date, last_audit_date, next_audit_date, notes, created_by, created_at, updated_at
FROM supplier_quality
WHERE company_id = $1 AND supplier_id = $2
ORDER BY created_at DESC
LIMIT 1;

-- name: ListSupplierQuality :many
SELECT id, company_id, supplier_id, status, quality_rating, risk_level,
       approved_date, expiry_date, last_audit_date, next_audit_date, notes, created_by, created_at, updated_at
FROM supplier_quality
WHERE company_id = $1
  AND ($2::text IS NULL OR status = $2)
ORDER BY supplier_id
LIMIT $3 OFFSET $4;

-- name: UpdateSupplierQuality :exec
UPDATE supplier_quality
SET status = $2, quality_rating = $3, risk_level = $4, approved_date = $5,
    expiry_date = $6, last_audit_date = $7, next_audit_date = $8, notes = $9, updated_at = NOW()
WHERE id = $1;

-- =============================================================================
-- SUPPLIER AUDITS
-- =============================================================================

-- name: InsertSupplierAudit :one
INSERT INTO supplier_audits (company_id, supplier_id, audit_number, audit_type, status,
                             standard, planned_date, actual_date, score, lead_auditor_id, report_number, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
RETURNING id;

-- name: GetSupplierAudit :one
SELECT id, company_id, supplier_id, audit_number, audit_type, status, standard,
       planned_date, actual_date, score, lead_auditor_id, report_number, created_by, created_at, updated_at
FROM supplier_audits
WHERE id = $1;

-- name: ListSupplierAudits :many
SELECT id, company_id, supplier_id, audit_number, audit_type, status, standard,
       planned_date, actual_date, score, lead_auditor_id, report_number, created_by, created_at, updated_at
FROM supplier_audits
WHERE company_id = $1
  AND ($2::int8 IS NULL OR supplier_id = $2)
ORDER BY planned_date DESC NULLS LAST
LIMIT $3 OFFSET $4;

-- =============================================================================
-- QUALITY OBJECTIVES
-- =============================================================================

-- name: InsertQualityObjective :one
INSERT INTO quality_objectives (company_id, name, description, metric_type, target_value,
                                unit, frequency, owner_id, status, start_date, end_date)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING id;

-- name: GetQualityObjective :one
SELECT id, company_id, name, description, metric_type, target_value, current_value,
       unit, frequency, owner_id, status, start_date, end_date, created_at, updated_at
FROM quality_objectives
WHERE id = $1;

-- name: ListQualityObjectives :many
SELECT id, company_id, name, description, metric_type, target_value, current_value,
       unit, frequency, owner_id, status, start_date, end_date, created_at, updated_at
FROM quality_objectives
WHERE company_id = $1
  AND ($2::text IS NULL OR status = $2)
ORDER BY name;

-- name: UpdateQualityObjective :exec
UPDATE quality_objectives
SET name = $2, description = $3, target_value = $4, current_value = $5, owner_id = $6,
    status = $7, end_date = $8, updated_at = NOW()
WHERE id = $1;

-- =============================================================================
-- QUALITY OBJECTIVE MEASUREMENTS
-- =============================================================================

-- name: InsertQualityObjectiveMeasurement :one
INSERT INTO quality_objective_measurements (objective_id, value, measurement_date, notes, recorded_by)
VALUES ($1, $2, $3, $4, $5)
RETURNING id;

-- name: GetQualityObjectiveMeasurement :one
SELECT id, objective_id, value, measurement_date, notes, recorded_by, created_at
FROM quality_objective_measurements
WHERE id = $1;

-- name: ListQualityObjectiveMeasurements :many
SELECT id, objective_id, value, measurement_date, notes, recorded_by, created_at
FROM quality_objective_measurements
WHERE objective_id = $1
ORDER BY measurement_date DESC
LIMIT $2;

-- =============================================================================
-- QMS INSPECTIONS
-- =============================================================================

-- name: InsertQMSInspection :one
INSERT INTO qms_inspections (company_id, name, description, reference_module, reference_id,
                             status, inspector_id, scheduled_at, started_at, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING id;

-- name: GetQMSInspection :one
SELECT id, company_id, name, description, reference_module, reference_id,
       status, inspector_id, scheduled_at, started_at, completed_at,
       created_by, created_at, updated_at
FROM qms_inspections
WHERE id = $1;

-- name: ListQMSInspections :many
SELECT id, company_id, name, description, reference_module, reference_id,
       status, inspector_id, scheduled_at, started_at, completed_at,
       created_by, created_at, updated_at
FROM qms_inspections
WHERE company_id = $1
  AND ($2::text IS NULL OR status = $2)
  AND ($3::text IS NULL OR reference_module = $3)
ORDER BY created_at DESC
LIMIT $4 OFFSET $5;

-- name: UpdateQMSInspectionStatus :exec
UPDATE qms_inspections
SET status = $2, started_at = COALESCE(started_at, $3), completed_at = $4, updated_at = NOW()
WHERE id = $1;

-- =============================================================================
-- QMS INSPECTION RESULTS
-- =============================================================================

-- name: InsertQMSInspectionResult :one
INSERT INTO qms_inspection_results (company_id, inspection_id, characteristic_name, expected_value, actual_value, is_conforming, notes, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING id;

-- name: ListQMSInspectionResults :many
SELECT id, company_id, inspection_id, characteristic_name, expected_value, actual_value, is_conforming, notes, created_by, created_at
FROM qms_inspection_results
WHERE inspection_id = $1
ORDER BY created_at ASC;

-- =============================================================================
-- CUSTOMER COMPLAINTS
-- =============================================================================

-- name: InsertCustomerComplaint :one
INSERT INTO customer_complaints (company_id, complaint_number, customer_id, title, description,
                                 status, severity, assigned_to, received_at, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING id;

-- name: GetCustomerComplaint :one
SELECT id, company_id, complaint_number, customer_id, title, description,
       status, severity, assigned_to, response_evidence, received_at, closed_at,
       created_by, created_at, updated_at
FROM customer_complaints
WHERE id = $1;

-- name: ListCustomerComplaints :many
SELECT id, company_id, complaint_number, customer_id, title, description,
       status, severity, assigned_to, response_evidence, received_at, closed_at,
       created_by, created_at, updated_at
FROM customer_complaints
WHERE company_id = $1
  AND ($2::text IS NULL OR status = $2)
  AND ($3::text IS NULL OR severity = $3)
  AND ($4::int8 IS NULL OR customer_id = $4)
ORDER BY received_at DESC
LIMIT $5 OFFSET $6;

-- name: UpdateCustomerComplaint :exec
UPDATE customer_complaints
SET title = $2, description = $3, severity = $4, assigned_to = $5,
    response_evidence = $6, updated_at = NOW()
WHERE id = $1;

-- name: UpdateCustomerComplaintStatus :exec
UPDATE customer_complaints
SET status = $2, closed_at = $3, updated_at = NOW()
WHERE id = $1;

-- =============================================================================
-- QMS HOLDS
-- =============================================================================

-- name: InsertQMSHold :one
INSERT INTO qms_holds (company_id, reference_module, reference_id, reason, created_by)
VALUES ($1, $2, $3, $4, $5)
RETURNING id;

-- name: ReleaseQMSHold :exec
UPDATE qms_holds
SET status = 'RELEASED', released_by = $2, released_at = NOW()
WHERE id = $1 AND company_id = $3 AND status = 'ACTIVE';

-- name: CreateSPCChart :one
INSERT INTO qms_spc_charts (
    company_id, name, characteristic, ucl, lcl, uwl, lwl, target_value, sample_interval_min, is_active
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10
) RETURNING id;

-- name: GetSPCChart :one
SELECT * FROM qms_spc_charts WHERE id = $1;

-- name: CreateSPCSample :one
INSERT INTO qms_spc_samples (
    chart_id, value, sampled_at, operator_id, is_outlier, notes
) VALUES (
    $1, $2, $3, $4, $5, $6
) RETURNING id;

-- name: GetSPCSamples :many
SELECT * FROM qms_spc_samples WHERE chart_id = $1 ORDER BY sampled_at DESC LIMIT $2;

-- name: CreateATEIntegration :one
INSERT INTO qms_ate_integrations (
    company_id, equipment_name, ip_address, protocol, status, last_ping_at
) VALUES (
    $1, $2, $3, $4, $5, $6
) RETURNING id;

-- name: GetATEIntegration :one
SELECT * FROM qms_ate_integrations WHERE id = $1;

-- name: CreateATETestResult :one
INSERT INTO qms_ate_test_results (
    equipment_id, product_serial, test_sequence, pass, raw_data, tested_at
) VALUES (
    $1, $2, $3, $4, $5, $6
) RETURNING id;

-- name: CreateLabSample :one
INSERT INTO qms_lab_samples (
    company_id, sample_number, source_type, source_id, status, priority, assigned_lab, collected_by, collected_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9
) RETURNING id;

-- name: GetLabSample :one
SELECT * FROM qms_lab_samples WHERE id = $1;

-- name: UpdateLabSampleStatus :exec
UPDATE qms_lab_samples SET status = $2, completed_at = $3 WHERE id = $1;

-- name: CreateLabTest :one
INSERT INTO qms_lab_tests (
    sample_id, test_name, method, result_value, is_pass, tested_by, tested_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7
) RETURNING id;

-- name: ListQMSHolds :many
SELECT id, company_id, reference_module, reference_id, reason, status, created_by, created_at, released_by, released_at
FROM qms_holds
WHERE company_id = $1
  AND ($2::text IS NULL OR reference_module = $2)
  AND ($3::int8 IS NULL OR reference_id = $3)
ORDER BY created_at DESC;

-- =============================================================================
-- QMS INSPECTION PLANS
-- =============================================================================

-- name: InsertQMSInspectionPlan :one
INSERT INTO qms_inspection_plans (company_id, name, description, reference_module, reference_id, created_by)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id;

-- name: GetQMSInspectionPlan :one
SELECT id, company_id, name, description, reference_module, reference_id, is_active, created_at, updated_at, created_by
FROM qms_inspection_plans
WHERE id = $1;