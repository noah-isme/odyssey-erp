-- WARNING: this drops holidays, shifts, shift assignments, overtime requests,
-- and the leave half-day flags. Data in those tables/columns is lost.

DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions WHERE name IN (
        'hr.schedule.admin', 'hr.overtime.request', 'hr.overtime.admin'
    )
);

DELETE FROM permissions
WHERE name IN (
    'hr.schedule.admin', 'hr.overtime.request', 'hr.overtime.admin'
);

ALTER TABLE hr_leave_requests
    DROP COLUMN end_half_day,
    DROP COLUMN start_half_day;

DROP TABLE hr_overtime_requests;
DROP TABLE hr_employee_shifts;
DROP TABLE hr_shifts;
DROP TABLE hr_holidays;
