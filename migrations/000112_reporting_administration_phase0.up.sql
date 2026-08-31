-- 000112_reporting_administration_phase0.up.sql

-- Phase 0: Foundations for Governed Reporting, Scoped RBAC, and Fiscal Calendars

-- 1. Governed Reporting Datasets
CREATE TABLE reporting_datasets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id BIGINT NOT NULL REFERENCES companies(id),
    version INT NOT NULL DEFAULT 1,
    key VARCHAR(255) NOT NULL,
    business_owner BIGINT REFERENCES users(id),
    technical_owner BIGINT REFERENCES users(id),
    status VARCHAR(50) NOT NULL DEFAULT 'DRAFT',
    description TEXT,
    grain VARCHAR(255),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(company_id, key, version)
);

CREATE TABLE reporting_dataset_fields (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    dataset_id UUID NOT NULL REFERENCES reporting_datasets(id) ON DELETE CASCADE,
    field_name VARCHAR(255) NOT NULL,
    field_type VARCHAR(50) NOT NULL,
    classification VARCHAR(50) NOT NULL DEFAULT 'PUBLIC',
    is_dimension BOOLEAN NOT NULL DEFAULT false,
    is_measure BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(dataset_id, field_name)
);

-- 2. Scoped Role Templates
CREATE TABLE role_templates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL UNIQUE,
    description TEXT,
    version INT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE company_roles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id BIGINT NOT NULL REFERENCES companies(id),
    template_id UUID REFERENCES role_templates(id),
    name VARCHAR(255) NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'DRAFT',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(company_id, name)
);

CREATE TABLE scoped_user_roles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id BIGINT NOT NULL REFERENCES companies(id),
    user_id BIGINT NOT NULL REFERENCES users(id),
    role_id UUID NOT NULL REFERENCES company_roles(id),
    branch_id BIGINT, -- Optional branch scope
    valid_from TIMESTAMPTZ,
    valid_to TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(company_id, user_id, role_id, branch_id)
);

-- 3. Fiscal Calendars
CREATE TABLE fiscal_calendars (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    company_id BIGINT NOT NULL REFERENCES companies(id),
    name VARCHAR(255) NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'DRAFT',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(company_id, name)
);

CREATE TABLE fiscal_periods (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    calendar_id UUID NOT NULL REFERENCES fiscal_calendars(id) ON DELETE CASCADE,
    period_type VARCHAR(50) NOT NULL, -- e.g., 'REGULAR', 'ADJUSTMENT'
    period_sequence INT NOT NULL,
    start_date DATE NOT NULL,
    end_date DATE NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'OPEN',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(calendar_id, period_sequence)
);

-- 4. Company Policies (Timezone & Locale)
CREATE TABLE company_policies (
    company_id BIGINT PRIMARY KEY REFERENCES companies(id),
    timezone VARCHAR(100) NOT NULL DEFAULT 'UTC',
    locale VARCHAR(20) NOT NULL DEFAULT 'id-ID',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
