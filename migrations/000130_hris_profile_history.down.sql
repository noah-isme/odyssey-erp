-- WARNING: this is an irreversible PII loss. Dropping the hr_employees profile
-- columns removes date of birth, national id, tax id, address, contact data and
-- employment dates. Job history and emergency contacts are also dropped.

DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions WHERE name IN (
        'hr.compensation.view', 'hr.compensation.admin', 'hr.employee.sensitive'
    )
);

DELETE FROM permissions
WHERE name IN (
    'hr.compensation.view', 'hr.compensation.admin', 'hr.employee.sensitive'
);

ALTER TABLE payroll_compensation_assignments
    DROP COLUMN created_by,
    DROP COLUMN reason;

DROP TABLE hr_employee_job_history;
DROP TABLE hr_employee_emergency_contacts;

ALTER TABLE hr_employees
    DROP CONSTRAINT hr_employees_contract_dates_check,
    DROP COLUMN termination_reason,
    DROP COLUMN termination_date,
    DROP COLUMN probation_end_date,
    DROP COLUMN contract_end_date,
    DROP COLUMN contract_start_date,
    DROP COLUMN employment_type,
    DROP COLUMN country,
    DROP COLUMN postal_code,
    DROP COLUMN province,
    DROP COLUMN city,
    DROP COLUMN address_line2,
    DROP COLUMN address_line1,
    DROP COLUMN phone,
    DROP COLUMN dependents_count,
    DROP COLUMN marital_status,
    DROP COLUMN tax_id,
    DROP COLUMN national_id,
    DROP COLUMN gender,
    DROP COLUMN date_of_birth;
