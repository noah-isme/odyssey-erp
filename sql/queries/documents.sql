-- =============================================================================
-- DOCUMENT CLASSIFICATIONS
-- =============================================================================

-- name: ListDocumentClassifications :many
SELECT id, company_id, code, name, description, default_retention_policy_id, requires_approval, requires_signature, allowed_extensions, max_size_bytes, sort_order, active, created_at, created_by
FROM document_classifications
ORDER BY sort_order, code;

-- name: GetDocumentClassification :one
SELECT id, company_id, code, name, description, default_retention_policy_id, requires_approval, requires_signature, allowed_extensions, max_size_bytes, sort_order, active, created_at, created_by
FROM document_classifications
WHERE id = $1;

-- =============================================================================
-- DOCUMENT CATEGORIES
-- =============================================================================

-- name: ListDocumentCategories :many
SELECT id, company_id, parent_id, code, name, description, default_classification_id, numbering_rule_id, active, created_at, created_by
FROM document_categories
WHERE company_id = $1
ORDER BY code;

-- name: GetDocumentCategory :one
SELECT id, company_id, parent_id, code, name, description, default_classification_id, numbering_rule_id, active, created_at, created_by
FROM document_categories
WHERE id = $1;

-- name: InsertDocumentCategory :one
INSERT INTO document_categories (company_id, parent_id, code, name, description, active, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id, company_id, parent_id, code, name, description, default_classification_id, numbering_rule_id, active, created_at, created_by;

-- =============================================================================
-- DOCUMENT NUMBERING RULES
-- =============================================================================

-- name: ListDocumentNumberingRules :many
SELECT id, company_id, code, name, prefix, suffix, pattern, sequence_start, sequence_current, scope, scope_id, active, created_at, created_by
FROM document_numbering_rules
WHERE company_id = $1
ORDER BY created_at;

-- name: GetNumberingRuleForCategory :one
SELECT id, company_id, code, name, prefix, suffix, pattern, sequence_start, sequence_current, scope, scope_id, active, created_at, created_by
FROM document_numbering_rules
WHERE company_id = $1 AND scope = 'CATEGORY' AND scope_id = $2 AND active = true
LIMIT 1;

-- name: GetDefaultNumberingRule :one
SELECT id, company_id, code, name, prefix, suffix, pattern, sequence_start, sequence_current, scope, scope_id, active, created_at, created_by
FROM document_numbering_rules
WHERE company_id = $1 AND scope = 'COMPANY' AND active = true
LIMIT 1;

-- name: IncrementNumberingSequence :exec
UPDATE document_numbering_rules
SET sequence_current = sequence_current + 1
WHERE id = $1;

-- =============================================================================
-- DOCUMENTS
-- =============================================================================

-- name: InsertDocument :one
INSERT INTO documents (company_id, document_number, title, description, category_id, classification_id, numbering_rule_id, owner_id, status, created_by, updated_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING id;

-- name: GetDocument :one
SELECT d.id, d.company_id, d.category_id, d.classification_id, d.numbering_rule_id,
       d.document_number, d.title, d.description, d.owner_id, d.status,
       d.effective_from, d.effective_to, d.current_version_id,
       d.migration_source, d.migration_source_id,
       d.created_at, d.created_by, d.updated_at, d.updated_by,
       c.name AS category_name, cl.name AS classification_name,
       u.name AS owner_name, co.name AS company_name
FROM documents d
LEFT JOIN document_categories c ON d.category_id = c.id
LEFT JOIN document_classifications cl ON d.classification_id = cl.id
LEFT JOIN users u ON d.owner_id = u.id
LEFT JOIN companies co ON d.company_id = co.id
WHERE d.id = $1;

-- name: ListDocuments :many
SELECT d.id, d.company_id, d.category_id, d.classification_id, d.numbering_rule_id,
       d.document_number, d.title, d.description, d.owner_id, d.status,
       d.effective_from, d.effective_to, d.current_version_id,
       d.migration_source, d.migration_source_id,
       d.created_at, d.created_by, d.updated_at, d.updated_by,
       c.name AS category_name, cl.name AS classification_name,
       u.name AS owner_name, co.name AS company_name
FROM documents d
LEFT JOIN document_categories c ON d.category_id = c.id
LEFT JOIN document_classifications cl ON d.classification_id = cl.id
LEFT JOIN users u ON d.owner_id = u.id
LEFT JOIN companies co ON d.company_id = co.id
WHERE d.company_id = $1
  AND ($2::int8 IS NULL OR d.category_id = $2)
  AND ($3::int8 IS NULL OR d.classification_id = $3)
  AND ($4::int8 IS NULL OR d.owner_id = $4)
  AND ($5::text IS NULL OR d.status = $5)
  AND ($6::text IS NULL OR d.title ILIKE '%' || $6 || '%'
       OR d.description ILIKE '%' || $6 || '%'
       OR d.document_number ILIKE '%' || $6 || '%')
ORDER BY d.updated_at DESC
LIMIT $7 OFFSET $8;

-- name: UpdateDocument :exec
UPDATE documents
SET title = $2, description = $3, category_id = $4, classification_id = $5,
    owner_id = $6, updated_by = $7, updated_at = NOW()
WHERE id = $1;

-- name: DeleteDocument :exec
UPDATE documents
SET status = 'ARCHIVED', updated_at = NOW()
WHERE id = $1;

-- =============================================================================
-- DOCUMENT VERSIONS
-- =============================================================================

-- name: InsertDocumentVersion :one
INSERT INTO document_versions (company_id, document_id, version_number, blob_id, classification_id, change_summary, status, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING id;

-- name: GetDocumentVersion :one
SELECT dv.id, dv.company_id, dv.document_id, dv.version_number, dv.version_label,
       dv.blob_id, dv.status, dv.classification_id, dv.change_summary,
       dv.review_snapshot, dv.approved_by, dv.approved_at, dv.effective_at,
       dv.superseded_at, dv.superseded_by_version_id, dv.created_by, dv.created_at,
       d.document_number AS document_number, d.title AS document_title,
       b.storage_key AS blob_storage_key, u.name AS created_by_name
FROM document_versions dv
LEFT JOIN documents d ON dv.document_id = d.id
LEFT JOIN storage_blobs b ON dv.blob_id = b.id
LEFT JOIN users u ON dv.created_by = u.id
WHERE dv.id = $1;

-- name: ListDocumentVersions :many
SELECT dv.id, dv.company_id, dv.document_id, dv.version_number, dv.version_label,
       dv.blob_id, dv.status, dv.classification_id, dv.change_summary,
       dv.review_snapshot, dv.approved_by, dv.approved_at, dv.effective_at,
       dv.superseded_at, dv.superseded_by_version_id, dv.created_by, dv.created_at,
       d.document_number AS document_number, d.title AS document_title,
       b.storage_key AS blob_storage_key, u.name AS created_by_name
FROM document_versions dv
LEFT JOIN documents d ON dv.document_id = d.id
LEFT JOIN storage_blobs b ON dv.blob_id = b.id
LEFT JOIN users u ON dv.created_by = u.id
WHERE dv.company_id = $1
  AND dv.document_id = $2
  AND ($3::text IS NULL OR dv.status = $3)
ORDER BY dv.version_number DESC
LIMIT $4 OFFSET $5;

-- name: GetLatestDocumentVersion :one
SELECT dv.id, dv.company_id, dv.document_id, dv.version_number, dv.version_label,
       dv.blob_id, dv.status, dv.classification_id, dv.change_summary,
       dv.review_snapshot, dv.approved_by, dv.approved_at, dv.effective_at,
       dv.superseded_at, dv.superseded_by_version_id, dv.created_by, dv.created_at,
       d.document_number AS document_number, d.title AS document_title,
       b.storage_key AS blob_storage_key, u.name AS created_by_name
FROM document_versions dv
LEFT JOIN documents d ON dv.document_id = d.id
LEFT JOIN storage_blobs b ON dv.blob_id = b.id
LEFT JOIN users u ON dv.created_by = u.id
WHERE dv.document_id = $1
ORDER BY dv.version_number DESC
LIMIT 1;

-- name: SetCurrentDocumentVersion :exec
UPDATE documents
SET current_version_id = $2, updated_at = NOW()
WHERE id = $1;

-- name: UpdateDocumentVersionStatus :exec
UPDATE document_versions
SET status = $2
WHERE id = $1;

-- =============================================================================
-- STORAGE BLOBS
-- =============================================================================

-- name: GetStorageBlob :one
SELECT id, company_id, storage_key, storage_driver, bucket, size_bytes,
       checksum_sha256, declared_content_type, detected_content_type,
       encryption_metadata, malware_scan_status, malware_scan_details,
       metadata, reference_count, created_at, created_by
FROM storage_blobs
WHERE id = $1;

-- name: HasOtherDocumentBlobReferences :one
SELECT EXISTS (
    SELECT 1
    FROM document_versions
    WHERE blob_id = $1
      AND id <> $2
      AND status <> 'DISPOSED'
);

-- =============================================================================
-- DOCUMENT ACLS
-- =============================================================================

-- name: InsertDocumentACL :one
INSERT INTO document_acls (company_id, document_id, classification_id, principal_type, principal_id, permission, effect, granted_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING id;

-- name: DeleteDocumentACL :exec
DELETE FROM document_acls WHERE id = $1;

-- name: ListDocumentACLs :many
SELECT id, company_id, document_id, classification_id, principal_type, principal_id, permission, effect, granted_by, granted_at, expires_at
FROM document_acls
WHERE company_id = $1
  AND ($2::int8 IS NULL OR document_id = $2)
  AND ($3::int8 IS NULL OR classification_id = $3)
ORDER BY granted_at DESC;

-- =============================================================================
-- DOCUMENT LINKS
-- =============================================================================

-- name: InsertDocumentLink :one
INSERT INTO document_links (company_id, document_version_id, target_module, target_id, target_company_id, link_type, description, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING id;

-- name: DeleteDocumentLink :exec
DELETE FROM document_links WHERE id = $1;

-- name: ListDocumentLinks :many
SELECT id, company_id, document_version_id, target_module, target_id, target_company_id, link_type, description, created_at, created_by
FROM document_links
WHERE document_version_id = $1
ORDER BY created_at DESC;

-- =============================================================================
-- DOCUMENT REVIEW STEPS
-- =============================================================================

-- name: GetDocumentReviewStepsForDocument :many
SELECT rs.id, rs.company_id, rs.document_version_id, rs.step_order, rs.name,
       rs.reviewer_role_id, rs.reviewer_user_id, rs.required_approvals, rs.status, rs.due_at, rs.created_at
FROM document_review_steps rs
JOIN document_versions dv ON dv.id = rs.document_version_id
WHERE dv.document_id = $1 AND rs.status = 'PENDING'
ORDER BY rs.step_order;

-- name: InsertDocumentReviewStep :exec
INSERT INTO document_review_steps (company_id, document_version_id, step_order, name, reviewer_role_id, reviewer_user_id, required_approvals, status, due_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);

-- name: UpdateReviewStepStatus :exec
UPDATE document_review_steps
SET status = $2
WHERE id = $1;

-- name: AreAllReviewStepsApproved :one
SELECT NOT EXISTS (
    SELECT 1
    FROM document_review_steps
    WHERE document_version_id = $1 AND status != 'APPROVED'
);

-- =============================================================================
-- DOCUMENT REVIEW DECISIONS
-- =============================================================================

-- name: InsertReviewDecision :one
INSERT INTO document_review_decisions (company_id, review_step_id, reviewer_id, decision, comment)
VALUES ($1, $2, $3, $4, $5)
RETURNING id;

-- =============================================================================
-- DOCUMENT ACCESS EVENTS
-- =============================================================================

-- name: InsertDocumentAccessEvent :exec
INSERT INTO document_access_events (company_id, document_version_id, actor_id, action, ip_address, user_agent)
VALUES ($1, $2, $3, $4, $5::inet, $6);

-- =============================================================================
-- USER ROLES (for ACL checking)
-- =============================================================================

-- name: GetUserRoles :many
SELECT role_id FROM user_roles WHERE user_id = $1;

-- =============================================================================
-- LEGAL HOLDS
-- =============================================================================

-- name: InsertLegalHold :one
INSERT INTO legal_holds (company_id, name, description, issued_by, scope_type, scope_id)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id;

-- name: ReleaseLegalHold :exec
UPDATE legal_holds
SET status = 'RELEASED', released_by = $2, released_at = NOW()
WHERE id = $1;

-- name: HasActiveLegalHold :one
SELECT EXISTS (
    SELECT 1 FROM legal_holds lh
    WHERE lh.company_id = $1 AND lh.status = 'ACTIVE'
);

-- name: HasActiveLegalHoldForVersion :one
SELECT EXISTS (
    SELECT 1
    FROM document_versions dv
    JOIN documents d ON d.id = dv.document_id
    WHERE dv.id = $1
      AND (
        EXISTS (
          SELECT 1
          FROM legal_holds lh
          WHERE lh.company_id = dv.company_id
            AND lh.status = 'ACTIVE'
            AND (
                (lh.scope_type = 'DOCUMENT_VERSION' AND lh.scope_id = dv.id)
                OR (lh.scope_type = 'DOCUMENT' AND lh.scope_id = dv.document_id)
                OR (lh.scope_type = 'CLASSIFICATION' AND lh.scope_id = dv.classification_id)
                OR (lh.scope_type = 'CATEGORY' AND lh.scope_id = d.category_id)
                OR (lh.scope_type = 'COMPANY' AND lh.scope_id = dv.company_id)
            )
        )
        OR EXISTS (
          SELECT 1
          FROM legal_hold_references lhr
          JOIN legal_holds lh ON lh.id = lhr.legal_hold_id
          WHERE lhr.document_version_id = dv.id
            AND lhr.company_id = dv.company_id
            AND lh.status = 'ACTIVE'
        )
      )
);

-- =============================================================================
-- DOCUMENT RETENTION
-- =============================================================================

-- name: InsertDocumentRetention :exec
INSERT INTO document_retention (company_id, document_version_id, policy_id, trigger_date, expiry_date)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (document_version_id, policy_id) DO UPDATE
SET trigger_date = EXCLUDED.trigger_date,
    expiry_date = EXCLUDED.expiry_date,
    status = 'ACTIVE',
    calculated_at = NOW();

-- name: GetActiveRetentionPolicyForVersion :one
SELECT rp.id, rp.retention_period_days
FROM retention_policies rp
JOIN document_versions dv ON dv.company_id = rp.company_id
JOIN documents d ON d.id = dv.document_id
WHERE dv.id = $1
  AND rp.active = TRUE
  AND (rp.classification_ids IS NULL OR dv.classification_id = ANY(rp.classification_ids))
  AND (rp.category_ids IS NULL OR d.category_id = ANY(rp.category_ids))
ORDER BY
    (CASE WHEN rp.classification_ids IS NULL THEN 0 ELSE 1 END
     + CASE WHEN rp.category_ids IS NULL THEN 0 ELSE 1 END) DESC,
    rp.retention_period_days DESC,
    rp.id
LIMIT 1;

-- name: GetRetentionPolicyForCompany :one
SELECT id, company_id, retention_period_days
FROM retention_policies
WHERE id = $1 AND company_id = $2 AND active = TRUE;

-- =============================================================================
-- DOCUMENT SIGNATURES
-- =============================================================================

-- name: InsertDocumentSignatureChallenge :one
INSERT INTO document_signature_challenges (company_id, document_version_id, signer_id, expiry)
VALUES ($1, $2, $3, $4)
RETURNING challenge_id;

-- name: GetDocumentSignatureChallenge :one
SELECT challenge_id, company_id, document_version_id, signer_id, expiry, created_at
FROM document_signature_challenges
WHERE challenge_id = $1;

-- name: InsertDocumentSignature :one
INSERT INTO document_signatures (company_id, document_version_id, challenge_id, signer_id, record_version, record_hash, meaning, policy_version, auth_method)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING id, company_id, document_version_id, challenge_id, signer_id, record_version, record_hash, meaning, policy_version, auth_method, signed_at;

-- =============================================================================
-- DISPOSITION REQUESTS
-- =============================================================================

-- name: GetPendingDispositions :many
SELECT id, company_id, document_version_id, requested_by, reason, status, approved_by, approved_at, executed_at, executed_by, execution_evidence, error_message, created_at
FROM disposition_requests
WHERE status = 'APPROVED'
ORDER BY created_at
LIMIT 50;

-- name: CreateDispositionRequest :one
INSERT INTO disposition_requests (company_id, document_version_id, requested_by, reason)
VALUES ($1, $2, $3, $4)
RETURNING id, company_id, document_version_id, requested_by, reason, status,
          approved_by, approved_at, executed_at, executed_by, execution_evidence,
          error_message, created_at;

-- name: GetDispositionRequest :one
SELECT id, company_id, document_version_id, requested_by, reason, status,
       approved_by, approved_at, executed_at, executed_by, execution_evidence,
       error_message, created_at
FROM disposition_requests
WHERE id = $1;

-- name: GetOpenDispositionRequestForVersion :one
SELECT id, company_id, document_version_id, requested_by, reason, status,
       approved_by, approved_at, executed_at, executed_by, execution_evidence,
       error_message, created_at
FROM disposition_requests
WHERE document_version_id = $1 AND status IN ('PENDING', 'APPROVED')
ORDER BY created_at DESC
LIMIT 1;

-- name: UpdateDispositionRequest :one
UPDATE disposition_requests
SET status = $2,
    approved_by = CASE WHEN $2 = 'APPROVED' THEN $3 ELSE approved_by END,
    approved_at = CASE WHEN $2 = 'APPROVED' THEN NOW() ELSE approved_at END
WHERE id = $1 AND status = 'PENDING'
RETURNING id, company_id, document_version_id, requested_by, reason, status,
          approved_by, approved_at, executed_at, executed_by, execution_evidence,
          error_message, created_at;

-- name: UpdateDispositionExecution :exec
UPDATE disposition_requests
SET status = $2, executed_at = $3, executed_by = $4, execution_evidence = $5, error_message = $6
WHERE id = $1;

-- name: ListExpiredDocumentRetention :many
SELECT id, company_id, document_version_id, policy_id
FROM document_retention
WHERE status = 'ACTIVE' AND expiry_date <= NOW()
ORDER BY expiry_date, id
LIMIT 100;

-- name: MarkDocumentRetentionExpired :exec
UPDATE document_retention
SET status = 'EXPIRED', calculated_at = NOW()
WHERE id = $1 AND status = 'ACTIVE';

-- name: InsertStorageBlob :one
INSERT INTO storage_blobs (
    company_id, storage_key, storage_driver, bucket, size_bytes, checksum_sha256, 
    declared_content_type, detected_content_type, malware_scan_status, created_by
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10
) RETURNING id;

-- name: CreateDocumentOCRJob :one
INSERT INTO doc_ocr_jobs (
    company_id, document_version_id, blob_id, status, created_at
) VALUES (
    $1, $2, $3, $4, NOW()
) RETURNING id;

-- name: DeleteDocumentRetention :exec
DELETE FROM document_retention
WHERE id = $1 AND company_id = $2;

-- name: GetDocumentOCRJob :one
SELECT * FROM doc_ocr_jobs WHERE id = $1;

-- name: UpdateDocumentOCRJob :exec
UPDATE doc_ocr_jobs 
SET status = $2, extracted_text = $3, error_message = $4, completed_at = $5
WHERE id = $1;

-- name: CreateCollaborationSession :one
INSERT INTO doc_collaboration_sessions (
    company_id, document_version_id, session_token, host_user_id, active, expires_at
) VALUES (
    $1, $2, $3, $4, $5, $6
) RETURNING id;

-- name: GetCollaborationSession :one
SELECT * FROM doc_collaboration_sessions
WHERE session_token = $1 AND active = true AND expires_at > NOW();

-- name: GetCollaborationSessionByID :one
SELECT id, company_id, document_version_id, session_token, host_user_id, active, created_at, expires_at
FROM doc_collaboration_sessions
WHERE id = $1;

-- name: DisableCollaborationSession :exec
UPDATE doc_collaboration_sessions SET active = false WHERE id = $1;

-- name: CreateCollaborationChange :one
INSERT INTO doc_collaboration_changes (session_id, actor_id, operation, payload, occurred_at)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, session_id, actor_id, operation, payload, occurred_at AS timestamp;

-- name: IndexDocumentSearch :one
WITH removed AS (
    DELETE FROM doc_search_indices WHERE document_version_id = $2
)
INSERT INTO doc_search_indices (
    document_id, document_version_id, title, content, keywords, indexed_at
) VALUES (
    $1, $2, $3, $4, $5, NOW()
) RETURNING id;

-- name: SearchDocumentsFullText :many
SELECT d.*, v.version_number
FROM doc_search_indices i
JOIN documents d ON i.document_id = d.id
JOIN document_versions v ON i.document_version_id = v.id
WHERE d.company_id = $1
AND to_tsvector('english', coalesce(i.title, '') || ' ' || coalesce(i.content, '') || ' ' || coalesce(i.keywords, ''))
    @@ plainto_tsquery('english', $2)
ORDER BY ts_rank(
    to_tsvector('english', coalesce(i.title, '') || ' ' || coalesce(i.content, '') || ' ' || coalesce(i.keywords, '')),
    plainto_tsquery('english', $2)
) DESC
LIMIT $3;
