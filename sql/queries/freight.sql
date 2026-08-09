-- Phase 6: Freight Finance SQL Queries
-- name: CreateRateCard :one
INSERT INTO rate_cards (
  company_id, carrier_id, origin_city, origin_country,
  destination_city, destination_country, service_level,
  min_weight, max_weight, base_rate, per_kg_rate, per_cbm_rate,
  currency, effective_date, expiration_date, is_active, created_by
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17
)
RETURNING *;

-- name: GetRateCard :one
SELECT * FROM rate_cards
WHERE id = $1 AND company_id = $2;

-- name: ListRateCards :many
SELECT * FROM rate_cards
WHERE company_id = $1
  AND ($2::BIGINT = 0 OR carrier_id = $2)
  AND ($3::VARCHAR = '' OR origin_city = $3)
  AND ($4::VARCHAR = '' OR destination_city = $4)
  AND ($5::VARCHAR = '' OR service_level = $5)
  AND ($6::BOOLEAN IS FALSE OR is_active = TRUE)
  AND ($7::DATE IS NULL OR effective_date >= $7)
  AND ($8::DATE IS NULL OR effective_date <= $8)
ORDER BY effective_date DESC, id DESC
LIMIT $9 OFFSET $10;

-- name: GetApplicableRateCard :one
SELECT * FROM rate_cards
WHERE company_id = $1
  AND origin_city = $2
  AND destination_city = $3
  AND service_level = $4
  AND is_active = TRUE
  AND effective_date <= CURRENT_DATE
  AND (expiration_date IS NULL OR expiration_date >= CURRENT_DATE)
  AND ($5::NUMERIC IS NULL OR min_weight IS NULL OR min_weight <= $5)
  AND ($5::NUMERIC IS NULL OR max_weight IS NULL OR max_weight >= $5)
ORDER BY effective_date DESC
LIMIT 1;

-- name: UpdateRateCard :one
UPDATE rate_cards
SET base_rate = COALESCE($3, base_rate),
    per_kg_rate = COALESCE($4, per_kg_rate),
    per_cbm_rate = COALESCE($5, per_cbm_rate),
    expiration_date = COALESCE($6, expiration_date),
    is_active = COALESCE($7, is_active),
    updated_at = NOW()
WHERE id = $1 AND company_id = $2
RETURNING *;

-- name: DeactivateRateCard :exec
UPDATE rate_cards
SET is_active = FALSE, updated_at = NOW()
WHERE id = $1 AND company_id = $2;

-- name: CreateRateSurcharge :one
INSERT INTO rate_surcharges (
  company_id, rate_card_id, surcharge_type, surcharge_name,
  surcharge_amount, surcharge_percent, effective_date, expiration_date
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: ListRateSurcharges :many
SELECT * FROM rate_surcharges
WHERE rate_card_id = $1
  AND effective_date <= CURRENT_DATE
  AND (expiration_date IS NULL OR expiration_date >= CURRENT_DATE)
ORDER BY created_at ASC;

-- name: DeleteRateSurcharge :exec
DELETE FROM rate_surcharges WHERE id = $1;

-- name: CreateFreightCharge :one
INSERT INTO freight_charges (
  company_id, shipment_id, load_id, carrier_id, rate_card_id,
  origin_city, destination_city, service_level,
  weight_kg, volume_cbm, base_charge, weight_charge, volume_charge,
  surcharge_total, freight_total, currency, status, cost_center_id,
  notes, created_by
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20
)
RETURNING *;

-- name: GetFreightCharge :one
SELECT * FROM freight_charges
WHERE id = $1 AND company_id = $2;

-- name: ListFreightCharges :many
SELECT * FROM freight_charges
WHERE company_id = $1
  AND ($2::BIGINT = 0 OR shipment_id = $2)
  AND ($3::BIGINT = 0 OR load_id = $3)
  AND ($4::BIGINT = 0 OR carrier_id = $4)
  AND ($5::VARCHAR = '' OR status = $5)
  AND ($6::VARCHAR = '' OR origin_city = $6)
  AND ($7::VARCHAR = '' OR destination_city = $7)
  AND ($8::TIMESTAMP IS NULL OR created_at >= $8)
  AND ($9::TIMESTAMP IS NULL OR created_at <= $9)
ORDER BY created_at DESC, id DESC
LIMIT $10 OFFSET $11;

-- name: UpdateFreightCharge :one
UPDATE freight_charges
SET status = COALESCE($3, status),
    invoice_number = COALESCE($4, invoice_number),
    invoice_date = COALESCE($5, invoice_date),
    gl_posting_id = COALESCE($6, gl_posting_id),
    notes = COALESCE($7, notes),
    updated_at = NOW()
WHERE id = $1 AND company_id = $2
RETURNING *;

-- name: UpdateFreightChargeStatus :exec
UPDATE freight_charges
SET status = $3, updated_at = NOW()
WHERE id = $1 AND company_id = $2;

-- name: CreateLandedCost :one
INSERT INTO landed_costs (
  company_id, shipment_id, load_id, freight_charge_id, po_id,
  product_cost, freight_cost, duty_cost, tax_cost, insurance_cost, other_cost,
  total_landed_cost, cost_per_unit, currency, allocation_method
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
RETURNING *;

-- name: GetLandedCost :one
SELECT * FROM landed_costs
WHERE id = $1 AND company_id = $2;

-- name: GetLandedCostByShipment :one
SELECT * FROM landed_costs
WHERE shipment_id = $1 AND company_id = $2
ORDER BY created_at DESC
LIMIT 1;

-- name: ListLandedCosts :many
SELECT * FROM landed_costs
WHERE company_id = $1
  AND ($2::BIGINT = 0 OR shipment_id = $2)
  AND ($3::BIGINT = 0 OR load_id = $3)
  AND ($4::BIGINT = 0 OR po_id = $4)
  AND ($5::TIMESTAMP IS NULL OR created_at >= $5)
  AND ($6::TIMESTAMP IS NULL OR created_at <= $6)
ORDER BY created_at DESC, id DESC
LIMIT $7 OFFSET $8;

-- name: CreateFreightAuditLog :exec
INSERT INTO freight_audit_log (
  company_id, freight_charge_id, audit_type, old_value, new_value, reason, user_id
) VALUES ($1, $2, $3, $4, $5, $6, $7);

-- name: ListFreightAuditLogs :many
SELECT * FROM freight_audit_log
WHERE company_id = $1 AND freight_charge_id = $2
ORDER BY created_at DESC;

-- Cost centers are shared with accounting dimensions. Freight keeps its
-- company-scoped access here so GL posting never crosses tenant boundaries.
-- name: CreateFreightCostCenter :one
INSERT INTO cost_centers (
  company_id, code, name, cost_center_type, warehouse_id, gl_account, manager_id, is_active
) VALUES ($1, $2, $3, $4, $5, $6, $7, TRUE)
RETURNING id, company_id, department_id, code, name, cost_center_type, warehouse_id,
          gl_account, manager_id, is_active, created_at, updated_at;

-- name: GetFreightCostCenter :one
SELECT id, company_id, department_id, code, name, cost_center_type, warehouse_id,
       gl_account, manager_id, is_active, created_at, updated_at
FROM cost_centers
WHERE id = $1 AND company_id = $2;

-- name: GetFreightCostCenterByCode :one
SELECT id, company_id, department_id, code, name, cost_center_type, warehouse_id,
       gl_account, manager_id, is_active, created_at, updated_at
FROM cost_centers
WHERE company_id = $1 AND code = $2;

-- name: ListFreightCostCenters :many
SELECT id, company_id, department_id, code, name, cost_center_type, warehouse_id,
       gl_account, manager_id, is_active, created_at, updated_at
FROM cost_centers
WHERE company_id = $1
ORDER BY code ASC, id ASC;

-- name: UpdateFreightCostCenter :one
UPDATE cost_centers
SET name = COALESCE($3, name),
    cost_center_type = COALESCE($4, cost_center_type),
    warehouse_id = COALESCE($5, warehouse_id),
    gl_account = COALESCE($6, gl_account),
    manager_id = COALESCE($7, manager_id),
    is_active = COALESCE($8, is_active),
    updated_at = NOW()
WHERE id = $1 AND company_id = $2
RETURNING id, company_id, department_id, code, name, cost_center_type, warehouse_id,
          gl_account, manager_id, is_active, created_at, updated_at;
