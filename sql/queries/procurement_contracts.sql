-- =============================================================================
-- SUPPLIER CONTRACTS (Phase 3: Vendor Intelligence)
-- =============================================================================

-- name: CreateSupplierContract :one
INSERT INTO supplier_contracts (
    company_id, supplier_id, version, status, currency,
    effective_from, effective_to, payment_terms, incoterms,
    renewal_notice_days, created_by, note, created_at, updated_at
) VALUES ($1, $2, 1, 'DRAFT', $3, $4, $5, $6, $7, $8, $9, $10, NOW(), NOW())
RETURNING id;

-- name: GetSupplierContract :one
SELECT id, company_id, supplier_id, version, status, currency,
       effective_from, effective_to, payment_terms, incoterms,
       renewal_notice_days, expiry_notification_sent,
       created_by, approved_by, approved_at, terminated_at, note,
       created_at, updated_at
FROM supplier_contracts
WHERE id = $1;

-- name: GetSupplierContractByVersion :one
SELECT id, company_id, supplier_id, version, status, currency,
       effective_from, effective_to, payment_terms, incoterms,
       renewal_notice_days, expiry_notification_sent,
       created_by, approved_by, approved_at, terminated_at, note,
       created_at, updated_at
FROM supplier_contracts
WHERE company_id = $1 AND supplier_id = $2 AND version = $3;

-- name: ListSupplierContracts :many
SELECT id, company_id, supplier_id, version, status, currency,
       effective_from, effective_to, payment_terms, incoterms,
       renewal_notice_days, expiry_notification_sent,
       created_by, approved_by, approved_at, terminated_at, note,
       created_at, updated_at
FROM supplier_contracts
WHERE company_id = $1
ORDER BY supplier_id, version DESC;

-- name: ListActiveContractsBySupplier :many
SELECT id, company_id, supplier_id, version, status, currency,
       effective_from, effective_to, payment_terms, incoterms,
       renewal_notice_days, expiry_notification_sent,
       created_by, approved_by, approved_at, terminated_at, note,
       created_at, updated_at
FROM supplier_contracts
WHERE company_id = $1 AND supplier_id = $2 AND status = 'ACTIVE'
  AND effective_from <= CURRENT_DATE
  AND (effective_to IS NULL OR effective_to >= CURRENT_DATE)
ORDER BY version DESC;

-- name: UpdateSupplierContractStatus :exec
UPDATE supplier_contracts
SET status = $2, updated_at = NOW()
WHERE id = $1;

-- name: ApproveSupplierContract :exec
UPDATE supplier_contracts
SET status = 'ACTIVE', approved_by = $2, approved_at = NOW(), updated_at = NOW()
WHERE id = $1 AND status = 'APPROVAL';

-- name: TerminateSupplierContract :exec
UPDATE supplier_contracts
SET status = 'TERMINATED', terminated_at = NOW(), updated_at = NOW()
WHERE id = $1 AND status = 'ACTIVE';

-- name: SetContractExpiryNotificationSent :exec
UPDATE supplier_contracts
SET expiry_notification_sent = TRUE, updated_at = NOW()
WHERE id = $1;

-- =============================================================================
-- CONTRACT PRICE LINES
-- =============================================================================

-- name: InsertContractPriceLine :exec
INSERT INTO contract_price_lines (
    contract_id, product_id, min_quantity, unit_price, tax_rate,
    lead_time_days, moq
) VALUES ($1, $2, $3, $4, $5, $6, $7);

-- name: GetContractPriceLines :many
SELECT id, contract_id, product_id, min_quantity, unit_price, tax_rate,
       lead_time_days, moq
FROM contract_price_lines
WHERE contract_id = $1
ORDER BY product_id, min_quantity;

-- name: GetContractPriceLineForProduct :one
SELECT id, contract_id, product_id, min_quantity, unit_price, tax_rate,
       lead_time_days, moq
FROM contract_price_lines
WHERE contract_id = $1 AND product_id = $2 AND min_quantity <= $3
ORDER BY min_quantity DESC
LIMIT 1;

-- name: DeleteContractPriceLines :exec
DELETE FROM contract_price_lines WHERE contract_id = $1;

-- =============================================================================
-- PRICE HISTORY (Immutable)
-- =============================================================================

-- name: RecordPriceHistory :one
INSERT INTO price_history (
    company_id, supplier_id, product_id, source_type, source_id,
    currency, unit_price, quantity, tax_rate, moq, lead_time_days,
    fx_rate, base_currency_price, observation_date, note, created_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, NOW())
RETURNING id;

-- name: GetPriceHistoryBySource :many
SELECT id, company_id, supplier_id, product_id, source_type, source_id,
       currency, unit_price, quantity, tax_rate, moq, lead_time_days,
       fx_rate, base_currency_price, observation_date, note, created_at
FROM price_history
WHERE source_type = $1 AND source_id = $2
ORDER BY observation_date DESC;

-- name: ListPriceHistoryBySupplierProduct :many
SELECT id, company_id, supplier_id, product_id, source_type, source_id,
       currency, unit_price, quantity, tax_rate, moq, lead_time_days,
       fx_rate, base_currency_price, observation_date, note, created_at
FROM price_history
WHERE company_id = $1 AND supplier_id = $2 AND product_id = $3
ORDER BY observation_date DESC;

-- name: ListPriceHistoryTrend :many
SELECT observation_date, currency, unit_price, base_currency_price,
       source_type, source_id
FROM price_history
WHERE company_id = $1 AND supplier_id = $2 AND product_id = $3
ORDER BY observation_date DESC
LIMIT $4;

-- =============================================================================
-- SUPPLIER SCORECARDS
-- =============================================================================

-- name: CreateSupplierScorecard :one
INSERT INTO supplier_scorecards (
    company_id, supplier_id, version, period_start, period_end,
    status, created_by, created_at
) VALUES ($1, $2, (
    SELECT COALESCE(MAX(version), 0) + 1
    FROM supplier_scorecards
    WHERE company_id = $1 AND supplier_id = $2
), $3, $4, 'DRAFT', $5, NOW())
RETURNING id;

-- name: GetSupplierScorecard :one
SELECT id, company_id, supplier_id, version, period_start, period_end, status,
       delivery_otif_score, delivery_otif_weight, delivery_otif_sample_size,
       quality_score, quality_weight, quality_sample_size,
       price_adherence_score, price_adherence_weight, price_adherence_sample_size,
       rfq_responsiveness_score, rfq_responsiveness_weight, rfq_responsiveness_sample_size,
       reviewer_assessment_score, reviewer_assessment_weight,
       overall_score, published_by, published_at, note, created_by, created_at
FROM supplier_scorecards
WHERE id = $1;

-- name: GetLatestSupplierScorecard :one
SELECT id, company_id, supplier_id, version, period_start, period_end, status,
       delivery_otif_score, delivery_otif_weight, delivery_otif_sample_size,
       quality_score, quality_weight, quality_sample_size,
       price_adherence_score, price_adherence_weight, price_adherence_sample_size,
       rfq_responsiveness_score, rfq_responsiveness_weight, rfq_responsiveness_sample_size,
       reviewer_assessment_score, reviewer_assessment_weight,
       overall_score, published_by, published_at, note, created_by, created_at
FROM supplier_scorecards
WHERE company_id = $1 AND supplier_id = $2
ORDER BY version DESC
LIMIT 1;

-- name: ListSupplierScorecards :many
SELECT id, company_id, supplier_id, version, period_start, period_end, status,
       delivery_otif_score, delivery_otif_weight, delivery_otif_sample_size,
       quality_score, quality_weight, quality_sample_size,
       price_adherence_score, price_adherence_weight, price_adherence_sample_size,
       rfq_responsiveness_score, rfq_responsiveness_weight, rfq_responsiveness_sample_size,
       reviewer_assessment_score, reviewer_assessment_weight,
       overall_score, published_by, published_at, note, created_by, created_at
FROM supplier_scorecards
WHERE company_id = $1 AND supplier_id = $2
ORDER BY version DESC;

-- name: UpdateScorecardScores :exec
UPDATE supplier_scorecards
SET delivery_otif_score = $2,
    delivery_otif_sample_size = $3,
    quality_score = $4,
    quality_sample_size = $5,
    price_adherence_score = $6,
    price_adherence_sample_size = $7,
    rfq_responsiveness_score = $8,
    rfq_responsiveness_sample_size = $9,
    reviewer_assessment_score = $10,
    overall_score = $11
WHERE id = $1;

-- name: PublishSupplierScorecard :exec
UPDATE supplier_scorecards
SET status = 'PUBLISHED', published_by = $2, published_at = NOW()
WHERE id = $1 AND status = 'DRAFT';

-- =============================================================================
-- PO CONTRACT VARIANCES
-- =============================================================================

-- name: CreatePOVariance :one
INSERT INTO po_contract_variances (
    company_id, po_id, po_line_id, contract_id,
    variance_type, variance_percentage, variance_reason, note,
    created_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW(), NOW())
RETURNING id;

-- name: GetPOVariance :one
SELECT id, company_id, po_id, po_line_id, contract_id,
       variance_type, variance_percentage, variance_reason,
       approval_status, approved_by, approved_at, note,
       created_at, updated_at
FROM po_contract_variances
WHERE id = $1;

-- name: ListPOVariancesByPO :many
SELECT id, company_id, po_id, po_line_id, contract_id,
       variance_type, variance_percentage, variance_reason,
       approval_status, approved_by, approved_at, note,
       created_at, updated_at
FROM po_contract_variances
WHERE po_id = $1
ORDER BY po_line_id;

-- name: ListPendingVariances :many
SELECT id, company_id, po_id, po_line_id, contract_id,
       variance_type, variance_percentage, variance_reason,
       approval_status, approved_by, approved_at, note,
       created_at, updated_at
FROM po_contract_variances
WHERE company_id = $1 AND approval_status = 'PENDING'
ORDER BY created_at DESC;

-- name: ApprovePOVariance :exec
UPDATE po_contract_variances
SET approval_status = 'APPROVED', approved_by = $2, approved_at = NOW(),
    updated_at = NOW()
WHERE id = $1;

-- name: RejectPOVariance :exec
UPDATE po_contract_variances
SET approval_status = 'REJECTED', approved_by = $2, approved_at = NOW(),
    updated_at = NOW()
WHERE id = $1;
