DELETE FROM role_permissions
WHERE permission_id = (SELECT id FROM permissions WHERE name = 'finance.payment.view');

DELETE FROM permissions WHERE name = 'finance.payment.view';
