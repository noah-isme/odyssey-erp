-- WARNING: this drops performance cycles, participants and goals, and the
-- hr_performance_reviews.company_id column. Data in them is lost.

DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions WHERE name IN (
        'hr.performance.view',
        'hr.performance.admin',
        'hr.reports.view'
    )
);

DELETE FROM permissions
WHERE name IN (
    'hr.performance.view',
    'hr.performance.admin',
    'hr.reports.view'
);

DROP INDEX idx_hr_performance_reviews_company;
ALTER TABLE hr_performance_reviews DROP COLUMN company_id;

DROP TABLE hr_performance_goals;
DROP TABLE hr_performance_participants;
DROP TABLE hr_performance_cycles;
