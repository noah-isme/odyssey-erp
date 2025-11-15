CREATE TYPE board_pack_status AS ENUM ('PENDING','IN_PROGRESS','READY','FAILED');

CREATE TABLE board_pack_templates (
    id BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT,
    sections JSONB NOT NULL,
    is_default BOOLEAN NOT NULL DEFAULT FALSE,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_by BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_board_pack_templates_active ON board_pack_templates(is_active);
CREATE INDEX idx_board_pack_templates_default ON board_pack_templates(is_default);

CREATE TABLE board_packs (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    period_id BIGINT NOT NULL REFERENCES accounting_periods(id) ON DELETE CASCADE,
    template_id BIGINT NOT NULL REFERENCES board_pack_templates(id) ON DELETE RESTRICT,
    variance_snapshot_id BIGINT REFERENCES variance_snapshots(id) ON DELETE SET NULL,
    status board_pack_status NOT NULL DEFAULT 'PENDING',
    generated_at TIMESTAMPTZ,
    generated_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    file_path TEXT,
    file_size BIGINT,
    page_count INT,
    error_message TEXT,
    metadata JSONB NOT NULL DEFAULT '{}'::JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_board_packs_company_period ON board_packs(company_id, period_id);
CREATE INDEX idx_board_packs_status ON board_packs(status);
CREATE INDEX idx_board_packs_template ON board_packs(template_id);
