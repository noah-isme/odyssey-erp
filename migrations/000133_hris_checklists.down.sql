-- WARNING: this drops checklist templates, employee checklists and tasks, and
-- the fixed_assets.custodian_employee_id column. Data in them is lost.

DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions WHERE name IN (
        'hr.checklist.admin'
    )
);

DELETE FROM permissions
WHERE name IN (
    'hr.checklist.admin'
);

DROP INDEX idx_fixed_assets_custodian;
ALTER TABLE fixed_assets DROP COLUMN custodian_employee_id;

DROP TABLE hr_employee_checklist_tasks;
DROP TABLE hr_employee_checklists;
DROP TABLE hr_checklist_template_items;
DROP TABLE hr_checklist_templates;
