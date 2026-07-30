CREATE TABLE approval_policies (
    id BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    module TEXT NOT NULL,
    company_id BIGINT NULL REFERENCES companies(id) ON DELETE CASCADE,
    min_amount NUMERIC(18,2) NOT NULL DEFAULT 0,
    max_amount NUMERIC(18,2) NULL,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_by BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (max_amount IS NULL OR max_amount >= min_amount)
);
CREATE INDEX idx_approval_policies_resolution ON approval_policies(module, company_id, min_amount, is_active);

CREATE TABLE approval_policy_steps (
    id BIGSERIAL PRIMARY KEY,
    policy_id BIGINT NOT NULL REFERENCES approval_policies(id) ON DELETE CASCADE,
    step_order INTEGER NOT NULL CHECK (step_order > 0),
    name TEXT NOT NULL,
    approver_user_id BIGINT NULL REFERENCES users(id) ON DELETE RESTRICT,
    approver_role_id BIGINT NULL REFERENCES roles(id) ON DELETE RESTRICT,
    required_approvals INTEGER NOT NULL DEFAULT 1 CHECK (required_approvals > 0),
    escalation_hours INTEGER NULL CHECK (escalation_hours IS NULL OR escalation_hours > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (policy_id, step_order),
    CHECK ((approver_user_id IS NOT NULL) <> (approver_role_id IS NOT NULL))
);

CREATE TABLE approval_requests (
    id BIGSERIAL PRIMARY KEY,
    policy_id BIGINT NOT NULL REFERENCES approval_policies(id) ON DELETE RESTRICT,
    module TEXT NOT NULL,
    document_id BIGINT NOT NULL,
    company_id BIGINT NULL REFERENCES companies(id) ON DELETE RESTRICT,
    amount NUMERIC(18,2) NOT NULL DEFAULT 0,
    requester_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    current_step INTEGER NOT NULL DEFAULT 1,
    status TEXT NOT NULL CHECK (status IN ('PENDING','APPROVED','REJECTED','CANCELLED')),
    submitted_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX idx_approval_requests_active_document ON approval_requests(module, document_id)
    WHERE status = 'PENDING';

CREATE TABLE approval_assignments (
    id BIGSERIAL PRIMARY KEY,
    request_id BIGINT NOT NULL REFERENCES approval_requests(id) ON DELETE CASCADE,
    policy_step_id BIGINT NOT NULL REFERENCES approval_policy_steps(id) ON DELETE RESTRICT,
    step_order INTEGER NOT NULL,
    approver_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    delegated_from BIGINT NULL REFERENCES users(id) ON DELETE RESTRICT,
    status TEXT NOT NULL CHECK (status IN ('PENDING','APPROVED','REJECTED','CANCELLED')),
    due_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (request_id, policy_step_id, approver_id)
);
CREATE INDEX idx_approval_assignments_inbox ON approval_assignments(approver_id, status, created_at DESC);

CREATE TABLE approval_decisions (
    id BIGSERIAL PRIMARY KEY,
    request_id BIGINT NOT NULL REFERENCES approval_requests(id) ON DELETE CASCADE,
    assignment_id BIGINT NOT NULL REFERENCES approval_assignments(id) ON DELETE RESTRICT,
    actor_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    decision TEXT NOT NULL CHECK (decision IN ('APPROVE','REJECT')),
    note TEXT NOT NULL DEFAULT '',
    decided_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE approval_delegations (
    id BIGSERIAL PRIMARY KEY,
    delegator_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    delegate_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    module TEXT NULL,
    starts_at TIMESTAMPTZ NOT NULL,
    ends_at TIMESTAMPTZ NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (delegator_id <> delegate_id),
    CHECK (ends_at > starts_at)
);
CREATE INDEX idx_approval_delegations_active ON approval_delegations(delegator_id, starts_at, ends_at) WHERE is_active;

INSERT INTO permissions (name, description) VALUES
    ('approvals.inbox', 'View and decide assigned approvals'),
    ('approvals.policy.admin', 'Create and manage approval policies'),
    ('approvals.delegate', 'Manage approval delegations')
ON CONFLICT (name) DO UPDATE SET description=EXCLUDED.description;

INSERT INTO role_permissions(role_id, permission_id)
SELECT r.id, p.id FROM roles r CROSS JOIN permissions p
WHERE r.name = 'Admin' AND p.name LIKE 'approvals.%'
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions(role_id, permission_id)
SELECT r.id, p.id FROM roles r CROSS JOIN permissions p
WHERE r.name IN ('Finance Manager','Purchasing Manager') AND p.name = 'approvals.inbox'
ON CONFLICT DO NOTHING;
