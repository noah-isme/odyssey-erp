DELETE FROM role_permissions rp
USING permissions p
WHERE rp.permission_id = p.id AND p.name IN (
    'procurement.rfq.view', 'procurement.rfq.manage', 'procurement.rfq.award',
    'procurement.contract.view', 'procurement.contract.manage',
    'procurement.supplier_rating.view', 'procurement.supplier_rating.manage',
    'logistics.carrier.view', 'logistics.carrier.manage', 'logistics.fleet.view',
    'logistics.fleet.manage', 'logistics.plan.view', 'logistics.plan.manage',
    'logistics.dispatch.manage', 'logistics.freight.view', 'logistics.freight.manage'
);
DELETE FROM permissions WHERE name IN (
    'procurement.rfq.view', 'procurement.rfq.manage', 'procurement.rfq.award',
    'procurement.contract.view', 'procurement.contract.manage',
    'procurement.supplier_rating.view', 'procurement.supplier_rating.manage',
    'logistics.carrier.view', 'logistics.carrier.manage', 'logistics.fleet.view',
    'logistics.fleet.manage', 'logistics.plan.view', 'logistics.plan.manage',
    'logistics.dispatch.manage', 'logistics.freight.view', 'logistics.freight.manage'
);
DROP INDEX IF EXISTS idx_po_lines_rfq_award_line;
DROP INDEX IF EXISTS idx_pos_rfq_award;
ALTER TABLE po_lines DROP COLUMN IF EXISTS rfq_award_line_id;
ALTER TABLE pos DROP COLUMN IF EXISTS rfq_award_id;
DROP TABLE IF EXISTS rfq_award_lines;
DROP TABLE IF EXISTS rfq_awards;
DROP TABLE IF EXISTS rfq_comparison_snapshots;
DROP TABLE IF EXISTS rfq_bid_lines;
DROP TABLE IF EXISTS rfq_bids;
DROP TABLE IF EXISTS rfq_suppliers;
DROP TABLE IF EXISTS rfq_lines;
DROP TABLE IF EXISTS rfqs;
