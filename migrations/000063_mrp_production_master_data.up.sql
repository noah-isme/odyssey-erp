CREATE TABLE IF NOT EXISTS mrp_work_centers (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    capacity_hours_per_day NUMERIC(8,2) NOT NULL CHECK (capacity_hours_per_day > 0),
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_by BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (company_id, code)
);

CREATE TABLE IF NOT EXISTS mrp_routings (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    product_id BIGINT NOT NULL REFERENCES products(id) ON DELETE RESTRICT,
    code TEXT NOT NULL,
    version TEXT NOT NULL,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_by BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (company_id, product_id, version),
    UNIQUE (company_id, code, version)
);

CREATE TABLE IF NOT EXISTS mrp_routing_operations (
    id BIGSERIAL PRIMARY KEY,
    routing_id BIGINT NOT NULL REFERENCES mrp_routings(id) ON DELETE CASCADE,
    work_center_id BIGINT NOT NULL REFERENCES mrp_work_centers(id) ON DELETE RESTRICT,
    sequence INT NOT NULL CHECK (sequence > 0),
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    setup_minutes NUMERIC(10,2) NOT NULL DEFAULT 0 CHECK (setup_minutes >= 0),
    run_minutes NUMERIC(10,2) NOT NULL DEFAULT 0 CHECK (run_minutes >= 0),
    yield_pct NUMERIC(8,4) NOT NULL DEFAULT 100 CHECK (yield_pct > 0 AND yield_pct <= 100),
    UNIQUE (routing_id, sequence),
    UNIQUE (routing_id, code)
);

CREATE INDEX IF NOT EXISTS idx_mrp_routings_company_product_active
    ON mrp_routings(company_id, product_id, active);
