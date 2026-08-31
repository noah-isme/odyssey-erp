-- 000122_scoped_rbac_access_reviews.up.sql
--
-- Keep user_roles as the compatibility-preserving global assignment store. The
-- tables below are the tenant-safe path for assignments and access reviews.

-- A composite foreign key on assignments guarantees that a branch belongs to
-- the company carrying the assignment.
CREATE UNIQUE INDEX IF NOT EXISTS rbac_branches_id_company_key
    ON branches (id, company_id);

CREATE TABLE rbac_user_role_assignments (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role_id BIGINT NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    branch_id BIGINT,
    valid_from TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    valid_to TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT rbac_user_role_assignments_valid_range
        CHECK (valid_to IS NULL OR valid_to > valid_from),
    CONSTRAINT rbac_user_role_assignments_branch_company_fk
        FOREIGN KEY (branch_id, company_id)
        REFERENCES branches (id, company_id)
        ON DELETE CASCADE
);

-- NULL branch_id means company-wide. Partial indexes make that nullable scope
-- participate in idempotent uniqueness while still allowing dated history.
CREATE UNIQUE INDEX rbac_user_role_assignments_company_scope_key
    ON rbac_user_role_assignments (company_id, user_id, role_id, valid_from)
    WHERE branch_id IS NULL;

CREATE UNIQUE INDEX rbac_user_role_assignments_branch_scope_key
    ON rbac_user_role_assignments (company_id, user_id, role_id, branch_id, valid_from)
    WHERE branch_id IS NOT NULL;

CREATE INDEX rbac_user_role_assignments_active_lookup_idx
    ON rbac_user_role_assignments (company_id, user_id, valid_from, valid_to);

CREATE TABLE rbac_access_reviews (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    subject_user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    review_key TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'OPEN'
        CHECK (status IN ('OPEN', 'COMPLETED')),
    decision TEXT
        CHECK (decision IS NULL OR decision IN ('APPROVE', 'REVOKE')),
    opened_by_user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    decided_by_user_id BIGINT REFERENCES users(id) ON DELETE RESTRICT,
    decided_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT rbac_access_reviews_key_not_blank
        CHECK (btrim(review_key) <> ''),
    CONSTRAINT rbac_access_reviews_state_consistency
        CHECK (
            (status = 'OPEN'
                AND decision IS NULL
                AND decided_by_user_id IS NULL
                AND decided_at IS NULL)
            OR
            (status = 'COMPLETED'
                AND decision IS NOT NULL
                AND decided_by_user_id IS NOT NULL
                AND decided_at IS NOT NULL)
        ),
    CONSTRAINT rbac_access_reviews_idempotency_key
        UNIQUE (company_id, subject_user_id, review_key)
);

CREATE INDEX rbac_access_reviews_company_status_idx
    ON rbac_access_reviews (company_id, status, created_at DESC);
