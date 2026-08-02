DELETE FROM role_permissions rp
USING permissions p
WHERE rp.permission_id = p.id
  AND p.name IN (
      'finance.automation.manage', 'finance.bank_feed.manage',
      'finance.forecast.view', 'finance.forecast.manage',
      'finance.payment.propose', 'finance.payment.approve',
      'finance.payment.export', 'finance.payment.execute',
      'procurement.p2p_exception.view', 'procurement.p2p_exception.resolve',
      'fixedassets.location.manage', 'fixedassets.transfer.manage',
      'fixedassets.maintenance.manage', 'fixedassets.warranty.manage'
  );

DELETE FROM permissions
WHERE name IN (
    'finance.automation.manage', 'finance.bank_feed.manage',
    'finance.forecast.view', 'finance.forecast.manage',
    'finance.payment.propose', 'finance.payment.approve',
    'finance.payment.export', 'finance.payment.execute',
    'procurement.p2p_exception.view', 'procurement.p2p_exception.resolve',
    'fixedassets.location.manage', 'fixedassets.transfer.manage',
    'fixedassets.maintenance.manage', 'fixedassets.warranty.manage'
);

DROP TABLE IF EXISTS finance_automation_outbox;
DROP TRIGGER IF EXISTS trg_company_finance_automation_settings ON companies;
DROP FUNCTION IF EXISTS ensure_finance_automation_settings();
DROP TABLE IF EXISTS finance_automation_settings;
