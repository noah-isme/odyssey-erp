-- Rollback for Phase 3: Vendor Intelligence

DROP TABLE IF EXISTS po_contract_variances;
DROP TABLE IF EXISTS supplier_scorecards;
DROP TABLE IF EXISTS price_history;
DROP TABLE IF EXISTS contract_price_lines;
DROP TABLE IF EXISTS supplier_contracts;

DELETE FROM permissions WHERE module IN (
    'procurement.contract.create',
    'procurement.contract.submit',
    'procurement.contract.approve',
    'procurement.contract.terminate',
    'procurement.supplier_rating.view',
    'procurement.supplier_rating.create',
    'procurement.supplier_rating.publish',
    'procurement.price_history.view',
    'procurement.variance.view',
    'procurement.variance.approve'
);
