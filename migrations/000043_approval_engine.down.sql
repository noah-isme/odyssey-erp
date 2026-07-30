DELETE FROM role_permissions WHERE permission_id IN (SELECT id FROM permissions WHERE name LIKE 'approvals.%');
DELETE FROM permissions WHERE name LIKE 'approvals.%';
DROP TABLE IF EXISTS approval_delegations;
DROP TABLE IF EXISTS approval_decisions;
DROP TABLE IF EXISTS approval_assignments;
DROP TABLE IF EXISTS approval_requests;
DROP TABLE IF EXISTS approval_policy_steps;
DROP TABLE IF EXISTS approval_policies;
