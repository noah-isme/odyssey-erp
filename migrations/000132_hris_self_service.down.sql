-- WARNING: this drops expense claims and profile change requests.
-- Data in those tables is lost.

DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions WHERE name IN (
        'hr.expense.admin', 'hr.profile_change.admin'
    )
);

DELETE FROM permissions
WHERE name IN (
    'hr.expense.admin', 'hr.profile_change.admin'
);

DROP TABLE hr_profile_change_requests;
DROP TABLE hr_expense_claims;
