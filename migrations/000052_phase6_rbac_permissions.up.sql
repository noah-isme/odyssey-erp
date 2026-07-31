-- Repair the complete Phase 1–6 permission inventory and administrator grants.
-- This migration is additive and safe to run against partially upgraded databases.
INSERT INTO permissions (name, description) VALUES
    ('delivery.return.view', 'View return delivery orders'),
    ('delivery.return.create', 'Create return delivery orders'),
    ('delivery.return.post', 'Post return delivery orders'),
    ('delivery.return.void', 'Void return delivery orders'),
    ('finance.ar.credit_note.view', 'View AR credit notes'),
    ('finance.ar.credit_note.create', 'Create AR credit notes'),
    ('finance.ar.credit_note.post', 'Post AR credit notes'),
    ('finance.ar.credit_note.void', 'Void AR credit notes'),
    ('finance.ap.debit_note.view', 'View AP debit notes'),
    ('finance.ap.debit_note.create', 'Create AP debit notes'),
    ('finance.ap.debit_note.post', 'Post AP debit notes'),
    ('finance.ap.debit_note.void', 'Void AP debit notes'),
    ('procurement.return.view', 'View goods returns'),
    ('procurement.return.create', 'Create goods returns'),
    ('procurement.return.post', 'Post goods returns'),
    ('procurement.return.void', 'Void goods returns'),
    ('approvals.inbox', 'View and decide assigned approvals'),
    ('approvals.policy.admin', 'Create and manage approval policies'),
    ('approvals.delegate', 'Manage approval delegations'),
    ('hr.employee.view', 'View employee directory'),
    ('hr.employee.admin', 'Manage employee records'),
    ('hr.leave.request', 'Create and view own leave requests'),
    ('hr.leave.admin', 'Manage leave types and balances'),
    ('hr.attendance.import', 'Import attendance CSV files'),
    ('payroll.view', 'View payroll runs'),
    ('payroll.process', 'Create, calculate, and submit payroll runs'),
    ('payroll.post', 'Post approved payroll and export payments'),
    ('payroll.policy.admin', 'Manage payroll rules and account mappings'),
    ('payroll.payslip.own', 'View own payslips'),
    ('payroll.payslip.manager', 'View authorized reports payslips'),
    ('tax.view', 'View tax documents, ledgers, and recaps'),
    ('tax.config.manage', 'Manage reviewed tax configuration'),
    ('tax.period.lock', 'Lock tax reporting periods'),
    ('tax.document.correct', 'Cancel or replace tax documents'),
    ('tax.report.export', 'Generate tax authority exports'),
    ('crm.view', 'View owned CRM records'),
    ('crm.create', 'Create CRM leads, opportunities, and activities'),
    ('crm.edit', 'Update owned CRM records'),
    ('crm.convert', 'Convert won opportunities to customers and quotations'),
    ('crm.team.view', 'View all company CRM records'),
    ('crm.manage', 'Administer all company CRM records')
ON CONFLICT (name) DO UPDATE SET description = EXCLUDED.description;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
CROSS JOIN permissions p
WHERE LOWER(TRIM(r.name)) IN ('admin', 'administrator')
  AND p.name IN (
    'delivery.return.view', 'delivery.return.create', 'delivery.return.post', 'delivery.return.void',
    'finance.ar.credit_note.view', 'finance.ar.credit_note.create', 'finance.ar.credit_note.post', 'finance.ar.credit_note.void',
    'finance.ap.debit_note.view', 'finance.ap.debit_note.create', 'finance.ap.debit_note.post', 'finance.ap.debit_note.void',
    'procurement.return.view', 'procurement.return.create', 'procurement.return.post', 'procurement.return.void',
    'approvals.inbox', 'approvals.policy.admin', 'approvals.delegate',
    'hr.employee.view', 'hr.employee.admin', 'hr.leave.request', 'hr.leave.admin', 'hr.attendance.import',
    'payroll.view', 'payroll.process', 'payroll.post', 'payroll.policy.admin', 'payroll.payslip.own', 'payroll.payslip.manager',
    'tax.view', 'tax.config.manage', 'tax.period.lock', 'tax.document.correct', 'tax.report.export',
    'crm.view', 'crm.create', 'crm.edit', 'crm.convert', 'crm.team.view', 'crm.manage'
  )
ON CONFLICT DO NOTHING;
