-- BOMs are immutable manufacturing revisions.  Legacy active BOMs represent
-- the approved revision that was in use before revision control was added.
ALTER TABLE mrp_boms
    ADD COLUMN IF NOT EXISTS revision_status TEXT NOT NULL DEFAULT 'DRAFT'
        CHECK (revision_status IN ('DRAFT', 'APPROVED', 'SUPERSEDED')),
    ADD COLUMN IF NOT EXISTS approved_by BIGINT REFERENCES users(id) ON DELETE RESTRICT,
    ADD COLUMN IF NOT EXISTS approved_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS change_reason TEXT;

UPDATE mrp_boms
SET revision_status = 'APPROVED',
    approved_by = COALESCE(approved_by, created_by),
    approved_at = COALESCE(approved_at, NOW()),
    change_reason = COALESCE(NULLIF(change_reason, ''), 'Migrated legacy BOM')
WHERE active AND revision_status = 'DRAFT';

CREATE UNIQUE INDEX IF NOT EXISTS idx_mrp_boms_one_approved_revision_per_effective_date
    ON mrp_boms(company_id, product_id, effective_from)
    WHERE revision_status = 'APPROVED';

CREATE INDEX IF NOT EXISTS idx_mrp_boms_effective_approved
    ON mrp_boms(company_id, product_id, effective_from DESC)
    WHERE revision_status = 'APPROVED';

CREATE OR REPLACE FUNCTION prevent_approved_bom_revision_edit()
RETURNS trigger AS $$
BEGIN
    IF OLD.revision_status = 'APPROVED' AND NEW.revision_status <> 'SUPERSEDED' THEN
        RAISE EXCEPTION 'approved BOM revisions are immutable';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_prevent_approved_bom_revision_edit ON mrp_boms;
CREATE TRIGGER trg_prevent_approved_bom_revision_edit
    BEFORE UPDATE ON mrp_boms
    FOR EACH ROW EXECUTE FUNCTION prevent_approved_bom_revision_edit();

CREATE OR REPLACE FUNCTION prevent_approved_bom_line_edit()
RETURNS trigger AS $$
DECLARE
    target_bom_id BIGINT;
BEGIN
    target_bom_id := CASE WHEN TG_OP = 'DELETE' THEN OLD.bom_id ELSE NEW.bom_id END;
    IF EXISTS (SELECT 1 FROM mrp_boms WHERE id = target_bom_id AND revision_status = 'APPROVED') THEN
        RAISE EXCEPTION 'approved BOM revision lines are immutable';
    END IF;
    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_prevent_approved_bom_line_edit ON mrp_bom_lines;
CREATE TRIGGER trg_prevent_approved_bom_line_edit
    BEFORE INSERT OR UPDATE OR DELETE ON mrp_bom_lines
    FOR EACH ROW EXECUTE FUNCTION prevent_approved_bom_line_edit();
