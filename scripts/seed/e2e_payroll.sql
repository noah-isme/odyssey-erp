-- Seed data: E2E test fixture for Payroll and Payslip PDF
-- Purpose: Provides test data for E2E visual and PDF testing of Indonesian payroll and payslips
-- Covers payroll periods, payroll runs, run lines with BPJS/PPh21/TER calculations, and payslips

BEGIN;

-- 1. Ensure HR Department and Position exist for company 1
INSERT INTO hr_departments (company_id, code, name, created_at, updated_at)
VALUES (1, 'ENG', 'Engineering', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
ON CONFLICT (company_id, code) DO NOTHING;

INSERT INTO hr_positions (company_id, code, name, created_at, updated_at)
VALUES (1, 'SE', 'Software Engineer', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
ON CONFLICT (company_id, code) DO NOTHING;

-- 2. Ensure HR Employee "Budi Santoso" exists for company 1
INSERT INTO hr_employees (
    company_id,
    employee_number,
    name,
    email,
    department_id,
    position_id,
    user_id,
    hire_date,
    status,
    created_at,
    updated_at
)
SELECT
    1,
    'EMP-001',
    'Budi Santoso',
    'budi.santoso@odyssey.local',
    (SELECT id FROM hr_departments WHERE company_id = 1 AND code = 'ENG' LIMIT 1),
    (SELECT id FROM hr_positions WHERE company_id = 1 AND code = 'SE' LIMIT 1),
    1, -- linked to admin user
    '2024-01-01'::date,
    'ACTIVE',
    CURRENT_TIMESTAMP,
    CURRENT_TIMESTAMP
ON CONFLICT (company_id, employee_number) DO UPDATE SET
    name = EXCLUDED.name,
    email = EXCLUDED.email,
    user_id = EXCLUDED.user_id,
    status = EXCLUDED.status,
    updated_at = CURRENT_TIMESTAMP;

-- 3. Ensure base payroll rule versions exist
INSERT INTO payroll_rule_versions (
    code,
    rule_type,
    effective_from,
    source_name,
    source_url,
    source_reference,
    reviewed_by,
    reviewed_at,
    created_at
)
VALUES
    ('DJP-PP58-2023', 'TAX', '2024-01-01', 'JDIH Kementerian Keuangan', 'https://jdih.kemenkeu.go.id/download/e47c3fc4-a912-4bf1-bcad-335fee3f71f8/2023pp058.pdf', 'PP 58 Tahun 2023; PMK 168 Tahun 2023', 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    ('BPJS-2026-03', 'BPJS', '2024-01-01', 'BPJS Ketenagakerjaan / BPJS Kesehatan', 'https://www.bpjsketenagakerjaan.go.id/penerima-upah.html', 'PP 44-46/2015; Perpres 64/2020', 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    ('COMP-POLICY-2024', 'COMPANY', '2024-01-01', 'Odyssey Internal HR', 'https://odyssey.local/policies', 'Company Policy 2024', 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
ON CONFLICT (code) DO NOTHING;

-- 4. Ensure Company Policy exists for company 1
INSERT INTO payroll_company_policies (
    rule_version_id,
    company_id,
    overtime_divisor,
    first_hour_multiplier_bps,
    subsequent_hour_multiplier_bps,
    currency,
    rounding_unit,
    jkk_risk_class,
    effective_from
)
SELECT
    rv.id,
    1,
    173,
    15000,
    20000,
    'IDR',
    1,
    'LOW',
    '2024-01-01'::date
FROM payroll_rule_versions rv
WHERE rv.rule_type = 'COMPANY'
LIMIT 1
ON CONFLICT (company_id, effective_from) DO NOTHING;

-- 5. Insert / Update Payroll Period 2024-12
INSERT INTO payroll_periods (
    company_id,
    code,
    starts_on,
    ends_on,
    pay_date,
    status,
    created_at,
    updated_at
)
VALUES (
    1,
    '2024-12',
    '2024-12-01'::date,
    '2024-12-31'::date,
    '2024-12-25'::date,
    'CLOSED',
    CURRENT_TIMESTAMP,
    CURRENT_TIMESTAMP
)
ON CONFLICT (company_id, code) DO UPDATE SET
    starts_on = EXCLUDED.starts_on,
    ends_on = EXCLUDED.ends_on,
    pay_date = EXCLUDED.pay_date,
    status = EXCLUDED.status,
    updated_at = CURRENT_TIMESTAMP;

-- 6. Ensure employee compensation assignment exists
INSERT INTO payroll_compensation_assignments (
    employee_id,
    base_salary,
    ptkp_code,
    bpjs_health,
    bpjs_employment,
    bank_code,
    bank_account_number,
    bank_account_name,
    effective_from,
    created_at
)
SELECT
    e.id,
    8000000.00,
    'TK/0',
    TRUE,
    TRUE,
    'BCA',
    '1234567890',
    'Budi Santoso',
    '2024-01-01'::date,
    CURRENT_TIMESTAMP
FROM hr_employees e
WHERE e.company_id = 1 AND e.employee_number = 'EMP-001'
ON CONFLICT (employee_id, effective_from) DO UPDATE SET
    base_salary = EXCLUDED.base_salary,
    ptkp_code = EXCLUDED.ptkp_code;

-- 7. Insert Payroll Run for Period 2024-12 (initially in DRAFT to allow line insertion)
INSERT INTO payroll_runs (
    run_uuid,
    company_id,
    period_id,
    run_type,
    tax_rule_version_id,
    bpjs_rule_version_id,
    company_policy_id,
    status,
    created_by,
    created_at,
    updated_at
)
SELECT
    'a0000000-0000-0000-0000-000000000001'::uuid,
    1,
    p.id,
    'REGULAR',
    tax.id,
    bpjs.id,
    pol.id,
    'DRAFT',
    1,
    CURRENT_TIMESTAMP,
    CURRENT_TIMESTAMP
FROM payroll_periods p
CROSS JOIN (SELECT id FROM payroll_rule_versions WHERE rule_type = 'TAX' ORDER BY effective_from DESC LIMIT 1) tax
CROSS JOIN (SELECT id FROM payroll_rule_versions WHERE rule_type = 'BPJS' ORDER BY effective_from DESC LIMIT 1) bpjs
CROSS JOIN (SELECT id FROM payroll_company_policies WHERE company_id = 1 ORDER BY effective_from DESC LIMIT 1) pol
WHERE p.company_id = 1 AND p.code = '2024-12'
  AND NOT EXISTS (
      SELECT 1 FROM payroll_runs r WHERE r.company_id = 1 AND r.period_id = p.id
  );

-- 8. Insert Payroll Run Line for "Budi Santoso"
-- Base Salary: 8,000,000 | Allowances: 1,500,000 | Gross: 9,500,000
-- Employee BPJS: 380,000 | PPh 21: 270,000 | Other Deductions: 1,000,000 | Net Pay: 7,850,000
INSERT INTO payroll_run_lines (
    run_id,
    employee_id,
    department_id,
    ptkp_code,
    ter_category,
    base_salary,
    allowances,
    overtime,
    thr,
    gross,
    employee_bpjs,
    employer_bpjs,
    pph21,
    other_deductions,
    net_pay,
    breakdown,
    created_at
)
SELECT
    r.id,
    e.id,
    e.department_id,
    'TK/0',
    'A',
    8000000.00,
    1500000.00,
    0.00,
    0.00,
    9500000.00,
    380000.00,
    972800.00,
    270000.00,
    1000000.00,
    7850000.00,
    jsonb_build_object(
        'EmployeeID', e.id,
        'TaxVersionID', r.tax_rule_version_id,
        'BPJSVersionID', r.bpjs_rule_version_id,
        'PolicyID', r.company_policy_id,
        'PTKPCode', 'TK/0',
        'TERCategory', 'A',
        'PTKPAnnual', 54000000,
        'BaseSalary', 8000000,
        'Allowances', 1500000,
        'Overtime', 0,
        'THR', 0,
        'Adjustments', 0,
        'Gross', 9500000,
        'TaxableGross', 9500000,
        'EmployeeBPJS', 380000,
        'EmployerBPJS', 972800,
        'PPh21', 270000,
        'OtherDeductions', 1000000,
        'NetPay', 7850000,
        'TERRateBPS', 284,
        'Contributions', jsonb_build_array(
            jsonb_build_object('Program', 'HEALTH', 'WageBase', 9500000, 'Employee', 95000, 'Employer', 380000, 'EmployerTaxable', true),
            jsonb_build_object('Program', 'JHT', 'WageBase', 9500000, 'Employee', 190000, 'Employer', 351500, 'EmployerTaxable', false),
            jsonb_build_object('Program', 'JP', 'WageBase', 9500000, 'Employee', 95000, 'Employer', 190000, 'EmployerTaxable', false),
            jsonb_build_object('Program', 'JKK', 'WageBase', 9500000, 'Employee', 0, 'Employer', 22800, 'EmployerTaxable', true),
            jsonb_build_object('Program', 'JKM', 'WageBase', 9500000, 'Employee', 0, 'Employer', 28500, 'EmployerTaxable', true)
        ),
        'AttendanceDays', 22,
        'LeaveDays', 0
    ),
    CURRENT_TIMESTAMP
FROM payroll_runs r
JOIN payroll_periods p ON p.id = r.period_id
CROSS JOIN hr_employees e
WHERE p.company_id = 1 AND p.code = '2024-12'
  AND e.company_id = 1 AND e.employee_number = 'EMP-001'
  AND NOT EXISTS (
      SELECT 1 FROM payroll_run_lines l WHERE l.run_id = r.id AND l.employee_id = e.id
  );

-- 9. Insert Payslip Record for the Run Line
INSERT INTO payroll_payslips (
    run_line_id,
    document_key,
    checksum,
    generated_at,
    delivered_at
)
SELECT
    l.id,
    'payroll/2024-12/' || l.employee_id || '.pdf',
    md5(l.breakdown::text),
    CURRENT_TIMESTAMP,
    CURRENT_TIMESTAMP
FROM payroll_run_lines l
JOIN payroll_runs r ON r.id = l.run_id
JOIN payroll_periods p ON p.id = r.period_id
WHERE p.company_id = 1 AND p.code = '2024-12'
ON CONFLICT (run_line_id) DO NOTHING;

-- 10. Update Payroll Run status to POSTED (immutability sealed)
UPDATE payroll_runs
SET status = 'POSTED',
    posted_at = CURRENT_TIMESTAMP,
    updated_at = CURRENT_TIMESTAMP
WHERE company_id = 1
  AND period_id = (SELECT id FROM payroll_periods WHERE company_id = 1 AND code = '2024-12')
  AND status = 'DRAFT';

COMMIT;
