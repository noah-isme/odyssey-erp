DROP INDEX IF EXISTS idx_audit_logs_entity;
DROP INDEX IF EXISTS idx_suppliers_is_active;
DROP INDEX IF EXISTS idx_customers_is_active;
DROP INDEX IF EXISTS idx_products_deleted_at;
DROP INDEX IF EXISTS idx_products_is_active;
DROP INDEX IF EXISTS idx_products_tax;
DROP INDEX IF EXISTS idx_products_unit;
DROP INDEX IF EXISTS idx_products_category;

DROP TABLE IF EXISTS products;
DROP TABLE IF EXISTS customers;
DROP TABLE IF EXISTS suppliers;
DROP TABLE IF EXISTS categories;
DROP TABLE IF EXISTS taxes;
DROP TABLE IF EXISTS units;
DROP TABLE IF EXISTS warehouses;
DROP TABLE IF EXISTS branches;
DROP TABLE IF EXISTS companies;

ALTER TABLE audit_logs
    ALTER COLUMN meta SET DEFAULT NULL,
    ALTER COLUMN entity_id DROP NOT NULL,
    ALTER COLUMN actor_id DROP NOT NULL;

ALTER TABLE user_roles
    DROP COLUMN IF EXISTS created_at;

ALTER TABLE role_permissions
    DROP COLUMN IF EXISTS created_at;

ALTER TABLE permissions
    DROP COLUMN IF EXISTS description;

ALTER TABLE roles
    DROP COLUMN IF EXISTS updated_at,
    DROP COLUMN IF EXISTS created_at,
    DROP COLUMN IF EXISTS description;
