CREATE TABLE qms_spc_charts (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL,
    name VARCHAR(255) NOT NULL,
    characteristic VARCHAR(255) NOT NULL,
    ucl NUMERIC(10, 4) NOT NULL,
    lcl NUMERIC(10, 4) NOT NULL,
    uwl NUMERIC(10, 4) NOT NULL,
    lwl NUMERIC(10, 4) NOT NULL,
    target_value NUMERIC(10, 4) NOT NULL,
    sample_interval_min INT NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE TABLE qms_spc_samples (
    id BIGSERIAL PRIMARY KEY,
    chart_id BIGINT NOT NULL REFERENCES qms_spc_charts(id) ON DELETE CASCADE,
    value NUMERIC(10, 4) NOT NULL,
    sampled_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    operator_id BIGINT NOT NULL,
    is_outlier BOOLEAN NOT NULL DEFAULT FALSE,
    notes TEXT
);
