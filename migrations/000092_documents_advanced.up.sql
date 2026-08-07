CREATE TABLE doc_ocr_jobs (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL,
    document_version_id BIGINT NOT NULL REFERENCES document_versions(id) ON DELETE CASCADE,
    blob_id BIGINT NOT NULL,
    status VARCHAR(50) NOT NULL,
    extracted_text TEXT,
    error_message TEXT,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMP WITH TIME ZONE
);

CREATE TABLE doc_collaboration_sessions (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL,
    document_version_id BIGINT NOT NULL REFERENCES document_versions(id) ON DELETE CASCADE,
    session_token VARCHAR(255) NOT NULL UNIQUE,
    host_user_id BIGINT NOT NULL,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL
);

CREATE TABLE doc_search_indices (
    id BIGSERIAL PRIMARY KEY,
    document_id BIGINT NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    document_version_id BIGINT NOT NULL REFERENCES document_versions(id) ON DELETE CASCADE,
    title VARCHAR(500) NOT NULL,
    content TEXT NOT NULL,
    keywords TEXT,
    indexed_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- Note: We can add GIN indexes on document content for true Full Text Search if desired.
CREATE INDEX idx_doc_search_content ON doc_search_indices USING gin(to_tsvector('english', content));
