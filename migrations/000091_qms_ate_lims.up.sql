CREATE TABLE qms_ate_integrations (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL,
    equipment_name VARCHAR(255) NOT NULL,
    ip_address VARCHAR(255) NOT NULL,
    protocol VARCHAR(50) NOT NULL,
    status VARCHAR(50) NOT NULL,
    last_ping_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE TABLE qms_ate_test_results (
    id BIGSERIAL PRIMARY KEY,
    equipment_id BIGINT NOT NULL REFERENCES qms_ate_integrations(id) ON DELETE CASCADE,
    product_serial VARCHAR(255) NOT NULL,
    test_sequence VARCHAR(255) NOT NULL,
    pass BOOLEAN NOT NULL,
    raw_data TEXT NOT NULL,
    tested_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE TABLE qms_lab_samples (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL,
    sample_number VARCHAR(255) NOT NULL,
    source_type VARCHAR(50) NOT NULL,
    source_id BIGINT NOT NULL,
    status VARCHAR(50) NOT NULL,
    priority VARCHAR(50) NOT NULL,
    assigned_lab VARCHAR(255) NOT NULL,
    collected_by BIGINT NOT NULL,
    collected_at TIMESTAMP WITH TIME ZONE NOT NULL,
    completed_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE TABLE qms_lab_tests (
    id BIGSERIAL PRIMARY KEY,
    sample_id BIGINT NOT NULL REFERENCES qms_lab_samples(id) ON DELETE CASCADE,
    test_name VARCHAR(255) NOT NULL,
    method VARCHAR(255) NOT NULL,
    result_value VARCHAR(255) NOT NULL,
    is_pass BOOLEAN NOT NULL,
    tested_by BIGINT NOT NULL,
    tested_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);
