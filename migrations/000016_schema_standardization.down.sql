-- Rollback Migration 016: Schema Standardization
-- Note: Reverting BIGINT back to INTEGER is destructive if values exceed INTEGER range

-- Remove timestamps (cannot truly revert DEFAULT NOW(), just drop columns)
ALTER TABLE branches DROP COLUMN IF EXISTS created_at, DROP COLUMN IF EXISTS updated_at;
ALTER TABLE warehouses DROP COLUMN IF EXISTS created_at, DROP COLUMN IF EXISTS updated_at;
ALTER TABLE units DROP COLUMN IF EXISTS created_at, DROP COLUMN IF EXISTS updated_at;
ALTER TABLE categories DROP COLUMN IF EXISTS created_at, DROP COLUMN IF EXISTS updated_at;

-- Note: Not reverting BIGINT → INTEGER as it would be data-destructive
-- and the change to BIGINT is a safe expansion
