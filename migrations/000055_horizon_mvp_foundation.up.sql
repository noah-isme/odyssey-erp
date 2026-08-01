-- P7 Horizon MVP persistence foundation.
-- All operational records are company-scoped and retain lifecycle/audit data.

CREATE TABLE IF NOT EXISTS horizon_idempotency_keys (
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    actor_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    idempotency_key TEXT NOT NULL,
    operation TEXT NOT NULL,
    response JSONB NOT NULL DEFAULT '{}'::JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (company_id, operation, idempotency_key)
);

CREATE TABLE IF NOT EXISTS wms_bins (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    warehouse_id BIGINT NOT NULL REFERENCES warehouses(id) ON DELETE RESTRICT,
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    capacity NUMERIC(18,4) CHECK (capacity IS NULL OR capacity >= 0),
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (company_id, warehouse_id, code)
);

CREATE TABLE IF NOT EXISTS wms_barcode_aliases (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    barcode TEXT NOT NULL,
    product_id BIGINT REFERENCES products(id) ON DELETE CASCADE,
    bin_id BIGINT REFERENCES wms_bins(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK ((product_id IS NOT NULL) <> (bin_id IS NOT NULL)),
    UNIQUE (company_id, barcode)
);

CREATE TABLE IF NOT EXISTS wms_pick_waves (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    warehouse_id BIGINT NOT NULL REFERENCES warehouses(id) ON DELETE RESTRICT,
    number TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'DRAFT' CHECK (status IN ('DRAFT','RELEASED','COMPLETED','CANCELLED')),
    created_by BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    released_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    released_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (company_id, number)
);

CREATE TABLE IF NOT EXISTS wms_pick_tasks (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    wave_id BIGINT NOT NULL REFERENCES wms_pick_waves(id) ON DELETE CASCADE,
    delivery_order_id BIGINT REFERENCES delivery_orders(id) ON DELETE RESTRICT,
    product_id BIGINT NOT NULL REFERENCES products(id) ON DELETE RESTRICT,
    source_bin_id BIGINT REFERENCES wms_bins(id) ON DELETE RESTRICT,
    requested_qty NUMERIC(18,4) NOT NULL CHECK (requested_qty > 0),
    picked_qty NUMERIC(18,4) NOT NULL DEFAULT 0 CHECK (picked_qty >= 0 AND picked_qty <= requested_qty),
    status TEXT NOT NULL DEFAULT 'OPEN' CHECK (status IN ('OPEN','PICKING','SHORT','PICKED','PACKED','SHIPPED','CANCELLED')),
    assigned_to BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS wms_pick_scans (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    task_id BIGINT NOT NULL REFERENCES wms_pick_tasks(id) ON DELETE CASCADE,
    barcode TEXT NOT NULL,
    quantity NUMERIC(18,4) NOT NULL CHECK (quantity > 0),
    actor_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    idempotency_key TEXT NOT NULL,
    scanned_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (company_id, task_id, idempotency_key)
);

CREATE TABLE IF NOT EXISTS mrp_boms (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    product_id BIGINT NOT NULL REFERENCES products(id) ON DELETE RESTRICT,
    version TEXT NOT NULL,
    effective_from DATE NOT NULL,
    effective_to DATE,
    scrap_pct NUMERIC(8,4) NOT NULL DEFAULT 0 CHECK (scrap_pct >= 0 AND scrap_pct <= 100),
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    UNIQUE (company_id, product_id, version),
    CHECK (effective_to IS NULL OR effective_to >= effective_from)
);

CREATE TABLE IF NOT EXISTS mrp_bom_lines (
    id BIGSERIAL PRIMARY KEY,
    bom_id BIGINT NOT NULL REFERENCES mrp_boms(id) ON DELETE CASCADE,
    component_product_id BIGINT NOT NULL REFERENCES products(id) ON DELETE RESTRICT,
    quantity NUMERIC(18,6) NOT NULL CHECK (quantity > 0),
    scrap_pct NUMERIC(8,4) NOT NULL DEFAULT 0 CHECK (scrap_pct >= 0 AND scrap_pct <= 100),
    UNIQUE (bom_id, component_product_id)
);

CREATE TABLE IF NOT EXISTS mrp_work_orders (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    number TEXT NOT NULL,
    product_id BIGINT NOT NULL REFERENCES products(id) ON DELETE RESTRICT,
    bom_id BIGINT REFERENCES mrp_boms(id) ON DELETE RESTRICT,
    planned_qty NUMERIC(18,4) NOT NULL CHECK (planned_qty > 0),
    completed_qty NUMERIC(18,4) NOT NULL DEFAULT 0 CHECK (completed_qty >= 0 AND completed_qty <= planned_qty),
    status TEXT NOT NULL DEFAULT 'DRAFT' CHECK (status IN ('DRAFT','RELEASED','IN_PROGRESS','COMPLETED','CANCELLED','CLOSED')),
    created_by BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (company_id, number)
);

CREATE TABLE IF NOT EXISTS pos_terminals (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    warehouse_id BIGINT REFERENCES warehouses(id) ON DELETE RESTRICT,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    UNIQUE (company_id, code)
);

CREATE TABLE IF NOT EXISTS pos_sessions (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    terminal_id BIGINT NOT NULL REFERENCES pos_terminals(id) ON DELETE RESTRICT,
    cashier_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    opening_float NUMERIC(20,2) NOT NULL DEFAULT 0,
    closing_amount NUMERIC(20,2),
    variance NUMERIC(20,2),
    status TEXT NOT NULL DEFAULT 'OPEN' CHECK (status IN ('OPEN','CLOSED','CANCELLED')),
    opened_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    closed_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS pos_tickets (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    session_id BIGINT NOT NULL REFERENCES pos_sessions(id) ON DELETE RESTRICT,
    number TEXT NOT NULL,
    currency CHAR(3) NOT NULL DEFAULT 'IDR',
    subtotal NUMERIC(20,2) NOT NULL DEFAULT 0,
    tax_amount NUMERIC(20,2) NOT NULL DEFAULT 0,
    total NUMERIC(20,2) NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'DRAFT' CHECK (status IN ('DRAFT','COMPLETED','REFUNDED','VOID')),
    created_by BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (company_id, number)
);

CREATE TABLE IF NOT EXISTS pos_ticket_lines (
    id BIGSERIAL PRIMARY KEY,
    ticket_id BIGINT NOT NULL REFERENCES pos_tickets(id) ON DELETE CASCADE,
    product_id BIGINT NOT NULL REFERENCES products(id) ON DELETE RESTRICT,
    quantity NUMERIC(18,4) NOT NULL CHECK (quantity > 0),
    unit_price NUMERIC(20,2) NOT NULL CHECK (unit_price >= 0),
    discount NUMERIC(20,2) NOT NULL DEFAULT 0 CHECK (discount >= 0),
    tax_amount NUMERIC(20,2) NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS pos_payments (
    id BIGSERIAL PRIMARY KEY,
    ticket_id BIGINT NOT NULL REFERENCES pos_tickets(id) ON DELETE CASCADE,
    method TEXT NOT NULL CHECK (method IN ('CASH','BANK','CARD')),
    amount NUMERIC(20,2) NOT NULL CHECK (amount > 0),
    reference TEXT,
    idempotency_key TEXT NOT NULL,
    UNIQUE (ticket_id, idempotency_key)
);

CREATE TABLE IF NOT EXISTS projects (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    currency CHAR(3) NOT NULL DEFAULT 'IDR',
    status TEXT NOT NULL DEFAULT 'OPEN' CHECK (status IN ('DRAFT','OPEN','ON_HOLD','COMPLETED','CANCELLED')),
    manager_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_by BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (company_id, code)
);

CREATE TABLE IF NOT EXISTS project_tasks (
    id BIGSERIAL PRIMARY KEY,
    project_id BIGINT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'OPEN' CHECK (status IN ('OPEN','IN_PROGRESS','DONE','CANCELLED')),
    UNIQUE (project_id, code)
);

CREATE TABLE IF NOT EXISTS project_members (
    project_id BIGINT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    PRIMARY KEY (project_id, user_id)
);

CREATE TABLE IF NOT EXISTS timesheets (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    project_id BIGINT NOT NULL REFERENCES projects(id) ON DELETE RESTRICT,
    task_id BIGINT NOT NULL REFERENCES project_tasks(id) ON DELETE RESTRICT,
    employee_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    work_date DATE NOT NULL,
    hours NUMERIC(8,2) NOT NULL CHECK (hours > 0 AND hours <= 24),
    description TEXT,
    billable BOOLEAN NOT NULL DEFAULT TRUE,
    status TEXT NOT NULL DEFAULT 'DRAFT' CHECK (status IN ('DRAFT','SUBMITTED','APPROVED','REJECTED','LOCKED')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS api_keys (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    key_hash TEXT NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ,
    created_by BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS api_key_scopes (
    api_key_id BIGINT NOT NULL REFERENCES api_keys(id) ON DELETE CASCADE,
    scope TEXT NOT NULL,
    PRIMARY KEY (api_key_id, scope)
);

CREATE TABLE IF NOT EXISTS webhook_subscriptions (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    event_type TEXT NOT NULL,
    endpoint TEXT NOT NULL,
    secret_hash TEXT NOT NULL,
    secret_ciphertext TEXT NOT NULL DEFAULT '',
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_by BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS webhook_deliveries (
    id BIGSERIAL PRIMARY KEY,
    subscription_id BIGINT NOT NULL REFERENCES webhook_subscriptions(id) ON DELETE CASCADE,
    event_id UUID NOT NULL,
    payload JSONB NOT NULL,
    attempt_count INT NOT NULL DEFAULT 0,
    response_status INT,
    next_attempt_at TIMESTAMPTZ,
    delivered_at TIMESTAMPTZ,
    dead_lettered_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (subscription_id, event_id)
);

CREATE TABLE IF NOT EXISTS portal_users (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    portal_type TEXT NOT NULL CHECK (portal_type IN ('CUSTOMER','SUPPLIER','EMPLOYEE')),
    customer_id BIGINT REFERENCES customers(id) ON DELETE CASCADE,
    supplier_id BIGINT REFERENCES suppliers(id) ON DELETE CASCADE,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    UNIQUE (user_id, portal_type),
    CHECK ((portal_type = 'CUSTOMER' AND customer_id IS NOT NULL) OR
           (portal_type = 'SUPPLIER' AND supplier_id IS NOT NULL) OR
           (portal_type = 'EMPLOYEE'))
);

CREATE INDEX IF NOT EXISTS idx_wms_tasks_company_status ON wms_pick_tasks(company_id, status);
CREATE INDEX IF NOT EXISTS idx_mrp_work_orders_company_status ON mrp_work_orders(company_id, status);
CREATE INDEX IF NOT EXISTS idx_pos_tickets_company_status ON pos_tickets(company_id, status);
CREATE INDEX IF NOT EXISTS idx_timesheets_company_status ON timesheets(company_id, status);
CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_retry ON webhook_deliveries(next_attempt_at) WHERE delivered_at IS NULL AND dead_lettered_at IS NULL;

INSERT INTO permissions (name, description) VALUES
    ('wms.view', 'View warehouse execution'), ('wms.manage', 'Manage bins and picking'),
    ('mrp.view', 'View manufacturing planning'), ('mrp.manage', 'Manage BOMs and work orders'),
    ('pos.view', 'View point of sale'), ('pos.manage', 'Manage terminals and POS sessions'),
    ('projects.view', 'View projects and timesheets'), ('projects.manage', 'Manage projects and approvals'),
    ('api.manage', 'Manage API keys and integrations'), ('webhooks.manage', 'Manage webhook subscriptions'),
    ('portal.manage', 'Manage portal users')
ON CONFLICT (name) DO UPDATE SET description = EXCLUDED.description;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r CROSS JOIN permissions p
WHERE LOWER(TRIM(r.name)) IN ('admin', 'administrator')
  AND p.name IN ('wms.view','wms.manage','mrp.view','mrp.manage','pos.view','pos.manage',
                 'projects.view','projects.manage','api.manage','webhooks.manage','portal.manage')
ON CONFLICT DO NOTHING;
