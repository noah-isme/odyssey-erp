CREATE TABLE IF NOT EXISTS portal_documents (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    portal_type TEXT NOT NULL CHECK (portal_type IN ('CUSTOMER','SUPPLIER','EMPLOYEE')),
    filename TEXT NOT NULL,
    content_type TEXT NOT NULL DEFAULT 'application/octet-stream',
    content BYTEA NOT NULL,
    uploaded_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_portal_documents_owner ON portal_documents(company_id,user_id,portal_type,uploaded_at DESC);
