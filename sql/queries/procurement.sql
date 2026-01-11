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

