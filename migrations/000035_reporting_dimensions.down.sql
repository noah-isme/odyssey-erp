DROP INDEX IF EXISTS idx_journal_lines_cost_center;
DROP INDEX IF EXISTS idx_journal_lines_department;
ALTER TABLE journal_lines DROP COLUMN IF EXISTS cost_center_id, DROP COLUMN IF EXISTS department_id;
DROP TABLE IF EXISTS cost_centers;
DROP TABLE IF EXISTS departments;
