CREATE TABLE IF NOT EXISTS treasury_payment_policies (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL,
    calendar_id BIGINT,
    max_batch_amount NUMERIC(19,4),
    max_item_amount NUMERIC(19,4),
    cut_off_time TIME,
    bank_format VARCHAR(100),
    requires_maker_checker BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS treasury_supplier_bank_accounts (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL,
    supplier_id BIGINT NOT NULL,
    bank_name VARCHAR(255) NOT NULL,
    account_number VARCHAR(255) NOT NULL, -- Simplified for simulation, normally encrypted
    routing_number VARCHAR(100),
    currency VARCHAR(3) NOT NULL,
    effective_from TIMESTAMPTZ NOT NULL,
    effective_to TIMESTAMPTZ,
    verification_status VARCHAR(50) NOT NULL DEFAULT 'UNVERIFIED', -- UNVERIFIED, PENDING_APPROVAL, VERIFIED, REJECTED
    evidence_ref VARCHAR(255),
    hold_payments BOOLEAN NOT NULL DEFAULT TRUE,
    created_by BIGINT NOT NULL,
    approved_by BIGINT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS treasury_payment_calendars (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL,
    name VARCHAR(255) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS treasury_payment_calendar_holidays (
    id BIGSERIAL PRIMARY KEY,
    calendar_id BIGINT NOT NULL REFERENCES treasury_payment_calendars(id) ON DELETE CASCADE,
    holiday_date DATE NOT NULL,
    description VARCHAR(255)
);
