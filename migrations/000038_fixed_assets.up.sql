CREATE TABLE fixed_asset_categories (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    asset_account_id BIGINT NOT NULL REFERENCES accounts(id),
    accumulated_depreciation_account_id BIGINT NOT NULL REFERENCES accounts(id),
    depreciation_expense_account_id BIGINT NOT NULL REFERENCES accounts(id),
    useful_life_months INTEGER NOT NULL CHECK (useful_life_months > 0),
    residual_rate NUMERIC(5,2) NOT NULL DEFAULT 0 CHECK (residual_rate >= 0 AND residual_rate <= 100),
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    UNIQUE(company_id, code)
);
CREATE TABLE fixed_assets (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    category_id BIGINT NOT NULL REFERENCES fixed_asset_categories(id),
    number TEXT NOT NULL,
    name TEXT NOT NULL,
    acquisition_date DATE NOT NULL,
    in_service_date DATE NOT NULL,
    acquisition_cost NUMERIC(14,2) NOT NULL CHECK (acquisition_cost > 0),
    residual_value NUMERIC(14,2) NOT NULL DEFAULT 0 CHECK (residual_value >= 0),
    useful_life_months INTEGER NOT NULL CHECK (useful_life_months > 0),
    accumulated_depreciation NUMERIC(14,2) NOT NULL DEFAULT 0,
    last_depreciated_on DATE,
    status TEXT NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE','FULLY_DEPRECIATED','DISPOSED')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(), updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(company_id, number)
);
CREATE TABLE fixed_asset_disposals (
    id BIGSERIAL PRIMARY KEY, asset_id BIGINT NOT NULL REFERENCES fixed_assets(id), disposal_date DATE NOT NULL,
    proceeds NUMERIC(14,2) NOT NULL DEFAULT 0, journal_entry_id BIGINT REFERENCES journal_entries(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_fixed_assets_depreciation ON fixed_assets(status, in_service_date, last_depreciated_on);
