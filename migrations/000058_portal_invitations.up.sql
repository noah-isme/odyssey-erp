CREATE TABLE IF NOT EXISTS portal_invitations (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    email TEXT NOT NULL,
    display_name TEXT NOT NULL DEFAULT '',
    portal_type TEXT NOT NULL CHECK (portal_type IN ('CUSTOMER','SUPPLIER','EMPLOYEE')),
    customer_id BIGINT REFERENCES customers(id) ON DELETE CASCADE,
    supplier_id BIGINT REFERENCES suppliers(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    accepted_at TIMESTAMPTZ,
    created_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK ((portal_type='CUSTOMER' AND customer_id IS NOT NULL) OR
           (portal_type='SUPPLIER' AND supplier_id IS NOT NULL) OR
           (portal_type='EMPLOYEE'))
);
CREATE INDEX IF NOT EXISTS idx_portal_invitations_lookup ON portal_invitations(token_hash, expires_at) WHERE accepted_at IS NULL;
