DROP TRIGGER IF EXISTS trg_prevent_approved_bom_revision_edit ON mrp_boms;
DROP FUNCTION IF EXISTS prevent_approved_bom_revision_edit();
DROP TRIGGER IF EXISTS trg_prevent_approved_bom_line_edit ON mrp_bom_lines;
DROP FUNCTION IF EXISTS prevent_approved_bom_line_edit();
DROP INDEX IF EXISTS idx_mrp_boms_effective_approved;
DROP INDEX IF EXISTS idx_mrp_boms_one_approved_revision_per_effective_date;
ALTER TABLE mrp_boms
    DROP COLUMN IF EXISTS change_reason,
    DROP COLUMN IF EXISTS approved_at,
    DROP COLUMN IF EXISTS approved_by,
    DROP COLUMN IF EXISTS revision_status;
