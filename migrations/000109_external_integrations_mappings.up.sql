CREATE TABLE connector_object_mappings (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id),
    connection_id BIGINT NOT NULL REFERENCES connector_connections(id),
    local_entity_type VARCHAR(100) NOT NULL,
    local_entity_id BIGINT NOT NULL,
    remote_entity_type VARCHAR(100) NOT NULL,
    remote_entity_id VARCHAR(255) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(connection_id, local_entity_type, local_entity_id),
    UNIQUE(connection_id, remote_entity_type, remote_entity_id)
);

CREATE INDEX idx_connector_object_mappings_company ON connector_object_mappings(company_id, connection_id);

CREATE TABLE connector_sync_runs (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id),
    connection_id BIGINT NOT NULL REFERENCES connector_connections(id),
    sync_type VARCHAR(100) NOT NULL,
    status VARCHAR(50) NOT NULL,
    cursor_value VARCHAR(255),
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ,
    error_message TEXT
);

CREATE INDEX idx_connector_sync_runs_company ON connector_sync_runs(company_id, connection_id);
