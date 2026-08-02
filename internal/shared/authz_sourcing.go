package shared

const (
	PermProcurementRFQView        = "procurement.rfq.view"
	PermProcurementRFQManage      = "procurement.rfq.manage"
	PermProcurementRFQAward       = "procurement.rfq.award"
	PermProcurementContractView   = "procurement.contract.view"
	PermProcurementContractManage = "procurement.contract.manage"
	PermSupplierRatingView        = "procurement.supplier_rating.view"
	PermSupplierRatingManage      = "procurement.supplier_rating.manage"
	PermLogisticsCarrierView      = "logistics.carrier.view"
	PermLogisticsCarrierManage    = "logistics.carrier.manage"
	PermLogisticsFleetView        = "logistics.fleet.view"
	PermLogisticsFleetManage      = "logistics.fleet.manage"
	PermLogisticsPlanView         = "logistics.plan.view"
	PermLogisticsPlanManage       = "logistics.plan.manage"
	PermLogisticsDispatchManage   = "logistics.dispatch.manage"
	PermLogisticsFreightView      = "logistics.freight.view"
	PermLogisticsFreightManage    = "logistics.freight.manage"
)

// SourcingAndLogisticsScopes lists permissions introduced by the procurement
// sourcing foundation. Logistics permissions are intentionally seeded ahead of
// their feature modules so role provisioning remains stable across releases.
func SourcingAndLogisticsScopes() []string {
	return []string{
		PermProcurementRFQView, PermProcurementRFQManage, PermProcurementRFQAward,
		PermProcurementContractView, PermProcurementContractManage,
		PermSupplierRatingView, PermSupplierRatingManage,
		PermLogisticsCarrierView, PermLogisticsCarrierManage,
		PermLogisticsFleetView, PermLogisticsFleetManage,
		PermLogisticsPlanView, PermLogisticsPlanManage, PermLogisticsDispatchManage,
		PermLogisticsFreightView, PermLogisticsFreightManage,
	}
}
