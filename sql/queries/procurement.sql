-- =============================================================================
-- PURCHASE REQUESTS (PR)
-- =============================================================================

-- name: CreatePR :one
INSERT INTO prs (number, supplier_id, request_by, status, note, created_at)
VALUES ($1, $2, $3, $4, $5, NOW())
RETURNING id;

-- name: InsertPRLine :exec
INSERT INTO pr_lines (pr_id, product_id, qty, note)
VALUES ($1, $2, $3, $4);

-- name: GetPR :one
SELECT id, number, supplier_id, request_by, status, note
FROM prs WHERE id = $1;

-- name: GetPRLines :many
SELECT id, pr_id, product_id, qty, note
FROM pr_lines WHERE pr_id = $1 ORDER BY id;

-- name: UpdatePRStatus :exec
UPDATE prs SET status = $1 WHERE id = $2;

-- =============================================================================
-- PURCHASE ORDERS (PO)
-- =============================================================================

-- name: CreatePO :one
INSERT INTO pos (number, supplier_id, status, currency, expected_date, note, created_at)
VALUES ($1, $2, $3, $4, $5, $6, NOW())
RETURNING id;

-- name: InsertPOLine :exec
INSERT INTO po_lines (po_id, product_id, qty, price, tax_id, note)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: GetPO :one
SELECT id, number, supplier_id, status, currency, expected_date, note
FROM pos WHERE id = $1;

-- name: GetPOLines :many
SELECT id, po_id, product_id, qty, price, tax_id, note
FROM po_lines WHERE po_id = $1 ORDER BY id;

-- name: UpdatePOStatus :exec
UPDATE pos SET status = $1 WHERE id = $2;

-- name: SetPOApproval :exec
UPDATE pos SET approved_by = $1, approved_at = $2 WHERE id = $3;

-- =============================================================================
-- GOODS RECEIPTS (GRN)
-- =============================================================================

-- name: CreateGRN :one
INSERT INTO grns (number, po_id, supplier_id, warehouse_id, status, received_at, note, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
RETURNING id;

-- name: InsertGRNLine :exec
INSERT INTO grn_lines (grn_id, product_id, qty, unit_cost)
VALUES ($1, $2, $3, $4);

-- name: GetGRN :one
SELECT id, number, po_id, supplier_id, warehouse_id, status, received_at, note
FROM grns WHERE id = $1;

-- name: GetGRNLines :many
SELECT id, grn_id, product_id, qty, unit_cost
FROM grn_lines WHERE grn_id = $1 ORDER BY id;

-- name: UpdateGRNStatus :exec
UPDATE grns SET status = $1 WHERE id = $2;

-- name: POExistsByNumber :one
SELECT EXISTS(SELECT 1 FROM pos WHERE number = $1) AS exists;

-- name: PRExistsByNumber :one
SELECT EXISTS(SELECT 1 FROM prs WHERE number = $1) AS exists;

-- name: GRNExistsByNumber :one
SELECT EXISTS(SELECT 1 FROM grns WHERE number = $1) AS exists;

-- =============================================================================
-- GOODS RETURNS (from GRN)
-- =============================================================================

-- name: GetGRNLine :one
SELECT id, grn_id, product_id, qty, unit_cost, lot_number, expiry_date, serial_numbers
FROM grn_lines
WHERE id = $1;

-- name: CreateGoodsReturnGRN :one
INSERT INTO goods_return_grns (
    number, company_id, supplier_id, grn_id, warehouse_id,
    return_date, status, reason, notes, created_by, created_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW(), NOW())
RETURNING id;

-- name: CreateGoodsReturnGRNLine :one
INSERT INTO goods_return_grn_lines (
    goods_return_grn_id, grn_line_id, product_id, quantity_returned, unit_cost,
    notes, line_order, created_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, NOW(), NOW())
RETURNING id;

-- name: UpdateGoodsReturnGRNStatus :exec
UPDATE goods_return_grns
SET status = $2, updated_at = NOW()
WHERE id = $1;

-- name: ConfirmGoodsReturnGRN :exec
UPDATE goods_return_grns
SET status = 'CONFIRMED', confirmed_by = $2, confirmed_at = NOW(), updated_at = NOW()
WHERE id = $1 AND status = 'DRAFT';

-- name: CancelGoodsReturnGRN :exec
UPDATE goods_return_grns
SET status = 'CANCELLED', voided_by = $2, voided_at = NOW(), updated_at = NOW()
WHERE id = $1 AND status IN ('DRAFT', 'CONFIRMED');

-- name: GetGoodsReturnGRN :one
SELECT
    r.id, r.number, r.company_id, r.supplier_id, r.grn_id, r.warehouse_id,
    r.return_date, r.status, r.reason, r.notes,
    r.created_by, r.confirmed_by, r.confirmed_at, r.voided_by, r.voided_at,
    r.created_at, r.updated_at
FROM goods_return_grns r
WHERE r.id = $1;

-- name: ListGoodsReturnGRNLines :many
SELECT id, goods_return_grn_id, grn_line_id, product_id,
       quantity_returned, unit_cost, notes, line_order, created_at, updated_at
FROM goods_return_grn_lines
WHERE goods_return_grn_id = $1
ORDER BY line_order;

-- name: ListGoodsReturnGRNs :many
SELECT
    r.id, r.number, r.company_id, r.supplier_id, r.grn_id, r.warehouse_id,
    r.return_date, r.status, r.reason, r.notes,
    r.created_by, r.confirmed_by, r.confirmed_at, r.voided_by, r.voided_at,
    r.created_at, r.updated_at
FROM goods_return_grns r
ORDER BY r.created_at DESC;

-- name: GenerateGoodsReturnGRNNumber :one
SELECT generate_goods_return_grn_number();

