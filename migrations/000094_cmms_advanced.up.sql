CREATE TABLE cmms_iot_sensors (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    asset_id BIGINT NOT NULL REFERENCES cmms_assets(id) ON DELETE CASCADE,
    sensor_code VARCHAR(100) NOT NULL,
    sensor_type VARCHAR(50) NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'ACTIVE',
    last_reading_at TIMESTAMP WITH TIME ZONE,
    last_reading_value NUMERIC(15, 4),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    UNIQUE(company_id, sensor_code)
);

CREATE TABLE cmms_iot_readings (
    id BIGSERIAL PRIMARY KEY,
    sensor_id BIGINT NOT NULL REFERENCES cmms_iot_sensors(id) ON DELETE CASCADE,
    value NUMERIC(15, 4) NOT NULL,
    timestamp TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE TABLE cmms_predictive_models (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    asset_type VARCHAR(100) NOT NULL,
    model_name VARCHAR(100) NOT NULL,
    version VARCHAR(50) NOT NULL,
    accuracy NUMERIC(5, 4) NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    deployed_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE TABLE cmms_predictive_alerts (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    asset_id BIGINT NOT NULL REFERENCES cmms_assets(id) ON DELETE CASCADE,
    sensor_id BIGINT REFERENCES cmms_iot_sensors(id) ON DELETE SET NULL,
    model_id BIGINT REFERENCES cmms_predictive_models(id) ON DELETE SET NULL,
    severity VARCHAR(50) NOT NULL,
    description TEXT NOT NULL,
    generated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    resolved_at TIMESTAMP WITH TIME ZONE
);
