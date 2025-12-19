-- =============================================================================
-- UNITS (id, code, name, created_at, updated_at)
-- =============================================================================

-- name: GetUnit :one
SELECT id, code, name, created_at, updated_at FROM units WHERE id = $1;

-- name: CreateUnit :one
INSERT INTO units (code, name, created_at, updated_at) 
VALUES ($1, $2, $3, $4) 
RETURNING id, code, name, created_at, updated_at;

-- name: UpdateUnit :exec
UPDATE units SET code = $1, name = $2, updated_at = $3 WHERE id = $4;

-- name: DeleteUnit :exec
DELETE FROM units WHERE id = $1;

-- =============================================================================
-- TAXES (id, code, name, rate) - no timestamps in schema
-- =============================================================================

-- name: GetTax :one
SELECT id, code, name, rate FROM taxes WHERE id = $1;

-- name: CreateTax :one
INSERT INTO taxes (code, name, rate) VALUES ($1, $2, $3) RETURNING id, code, name, rate;

-- name: UpdateTax :exec
UPDATE taxes SET code = $1, name = $2, rate = $3 WHERE id = $4;

-- name: DeleteTax :exec
DELETE FROM taxes WHERE id = $1;

-- =============================================================================
-- CATEGORIES (id, code, name, created_at, updated_at, parent_id nullable)
-- =============================================================================

-- name: GetCategory :one
SELECT id, code, name, created_at, updated_at FROM categories WHERE id = $1;

-- name: CreateCategory :one
INSERT INTO categories (code, name, created_at, updated_at) 
VALUES ($1, $2, $3, $4) 
RETURNING id, code, name, created_at, updated_at;

-- name: UpdateCategory :exec
UPDATE categories SET code = $1, name = $2, updated_at = $3 WHERE id = $4;

-- name: DeleteCategory :exec
DELETE FROM categories WHERE id = $1;

-- =============================================================================
-- SUPPLIERS (id, code, name, phone, email, address, is_active) - no timestamps
-- =============================================================================

-- name: GetSupplier :one
SELECT id, code, name, phone, email, address, is_active 
FROM suppliers WHERE id = $1;

-- name: CreateSupplier :one
INSERT INTO suppliers (code, name, phone, email, address, is_active) 
VALUES ($1, $2, $3, $4, $5, $6) 
RETURNING id, code, name, phone, email, address, is_active;

-- name: UpdateSupplier :exec
UPDATE suppliers 
SET code = $1, name = $2, phone = $3, email = $4, address = $5, is_active = $6 
WHERE id = $7;

-- name: DeleteSupplier :exec
DELETE FROM suppliers WHERE id = $1;

-- =============================================================================
-- COMPANIES (id, code, name, address, tax_id, created_at, updated_at)
-- =============================================================================

-- name: GetCompany :one
SELECT id, code, name, address, tax_id, created_at, updated_at 
FROM companies WHERE id = $1;

-- name: CreateCompany :one
INSERT INTO companies (code, name, address, tax_id, created_at, updated_at) 
VALUES ($1, $2, $3, $4, $5, $6) 
RETURNING id, code, name, address, tax_id, created_at, updated_at;

-- name: UpdateCompany :exec
UPDATE companies 
SET code = $1, name = $2, address = $3, tax_id = $4, updated_at = $5 
WHERE id = $6;

-- name: DeleteCompany :exec
DELETE FROM companies WHERE id = $1;

-- =============================================================================
-- BRANCHES (id, company_id, code, name, address, created_at, updated_at)
-- =============================================================================

-- name: GetBranch :one
SELECT id, company_id, code, name, address, created_at, updated_at 
FROM branches WHERE id = $1;

-- name: CreateBranch :one
INSERT INTO branches (company_id, code, name, address, created_at, updated_at) 
VALUES ($1, $2, $3, $4, $5, $6) 
RETURNING id, company_id, code, name, address, created_at, updated_at;

-- name: UpdateBranch :exec
UPDATE branches 
SET company_id = $1, code = $2, name = $3, address = $4, updated_at = $5 
WHERE id = $6;

-- name: DeleteBranch :exec
DELETE FROM branches WHERE id = $1;

-- =============================================================================
-- WAREHOUSES (id, branch_id, code, name, address, created_at, updated_at)
-- =============================================================================

-- name: GetWarehouse :one
SELECT id, branch_id, code, name, address, created_at, updated_at 
FROM warehouses WHERE id = $1;

-- name: CreateWarehouse :one
INSERT INTO warehouses (branch_id, code, name, address, created_at, updated_at) 
VALUES ($1, $2, $3, $4, $5, $6) 
RETURNING id, branch_id, code, name, address, created_at, updated_at;

-- name: UpdateWarehouse :exec
UPDATE warehouses 
SET branch_id = $1, code = $2, name = $3, address = $4, updated_at = $5 
WHERE id = $6;

-- name: DeleteWarehouse :exec
DELETE FROM warehouses WHERE id = $1;

-- =============================================================================
-- PRODUCTS (id, sku, name, category_id, unit_id, price, tax_id, is_active, deleted_at)
-- Note: uses 'sku' instead of 'code', no 'cost', no created/updated_at
-- =============================================================================

-- name: GetProduct :one
SELECT id, sku, name, category_id, unit_id, price, tax_id, is_active, deleted_at 
FROM products WHERE id = $1;

-- name: CreateProduct :one
INSERT INTO products (sku, name, category_id, unit_id, price, tax_id, is_active) 
VALUES ($1, $2, $3, $4, $5, $6, $7) 
RETURNING id, sku, name, category_id, unit_id, price, tax_id, is_active, deleted_at;

-- name: UpdateProduct :exec
UPDATE products 
SET sku = $1, name = $2, category_id = $3, unit_id = $4, price = $5, tax_id = $6, is_active = $7 
WHERE id = $8;

-- name: DeleteProduct :exec
DELETE FROM products WHERE id = $1;

-- name: SoftDeleteProduct :exec
UPDATE products SET deleted_at = $1 WHERE id = $2;
