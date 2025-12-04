package shared

// Sales & Delivery permissions declared for RBAC.
const (
	// Customer permissions
	PermCustomerView   = "sales.customer.view"
	PermCustomerCreate = "sales.customer.create"
	PermCustomerEdit   = "sales.customer.edit"
	PermCustomerDelete = "sales.customer.delete"

	// Quotation permissions
	PermQuotationView    = "sales.quotation.view"
	PermQuotationCreate  = "sales.quotation.create"
	PermQuotationEdit    = "sales.quotation.edit"
	PermQuotationApprove = "sales.quotation.approve"
	PermQuotationReject  = "sales.quotation.reject"
	PermQuotationConvert = "sales.quotation.convert"

	// Sales Order permissions
	PermSalesOrderView    = "sales.order.view"
	PermSalesOrderCreate  = "sales.order.create"
	PermSalesOrderEdit    = "sales.order.edit"
	PermSalesOrderConfirm = "sales.order.confirm"
	PermSalesOrderCancel  = "sales.order.cancel"

	// Delivery Order permissions
	PermDeliveryOrderView     = "delivery.order.view"
	PermDeliveryOrderCreate   = "delivery.order.create"
	PermDeliveryOrderEdit     = "delivery.order.edit"
	PermDeliveryOrderConfirm  = "delivery.order.confirm"
	PermDeliveryOrderShip     = "delivery.order.ship"
	PermDeliveryOrderComplete = "delivery.order.complete"
	PermDeliveryOrderCancel   = "delivery.order.cancel"
	PermDeliveryOrderPrint    = "delivery.order.print"
)

// SalesScopes lists all permissions related to the sales module.
func SalesScopes() []string {
	return []string{
		PermCustomerView,
		PermCustomerCreate,
		PermCustomerEdit,
		PermCustomerDelete,
		PermQuotationView,
		PermQuotationCreate,
		PermQuotationEdit,
		PermQuotationApprove,
		PermQuotationReject,
		PermQuotationConvert,
		PermSalesOrderView,
		PermSalesOrderCreate,
		PermSalesOrderEdit,
		PermSalesOrderConfirm,
		PermSalesOrderCancel,
	}
}

// DeliveryScopes lists all permissions related to the delivery module.
func DeliveryScopes() []string {
	return []string{
		PermDeliveryOrderView,
		PermDeliveryOrderCreate,
		PermDeliveryOrderEdit,
		PermDeliveryOrderConfirm,
		PermDeliveryOrderShip,
		PermDeliveryOrderComplete,
		PermDeliveryOrderCancel,
		PermDeliveryOrderPrint,
	}
}

// AllSalesDeliveryScopes returns all sales and delivery permissions.
func AllSalesDeliveryScopes() []string {
	return append(SalesScopes(), DeliveryScopes()...)
}
