package shared

// Core platform permissions.
const (
	PermUsersView = "users.view"
	PermUsersEdit = "users.edit"

	PermRolesView = "roles.view"
	PermRolesEdit = "roles.edit"

	PermPermissionsView = "permissions.view"
	// Scoped permission administration is intentionally separate from listing
	// the permission catalog. Assignment and review operations are tenant
	// scoped and are guarded by the corresponding permission.
	PermPermissionsAssign = "permissions.assign"
	PermPermissionsReview = "permissions.review"

	// Organization permissions
	PermOrgView = "org.view"
	PermOrgEdit = "org.edit"

	// Master data permissions
	PermMasterView   = "master.view"
	PermMasterEdit   = "master.edit"
	PermMasterImport = "master.import"

	// RBAC permissions
	PermRBACView = "rbac.view"
	PermRBACEdit = "rbac.edit"

	// Report permissions
	PermReportView = "report.view"

	// Inventory permissions
	PermInventoryView = "inventory.view"
	PermInventoryEdit = "inventory.edit"

	// Procurement permissions
	PermProcurementView = "procurement.view"
	PermProcurementEdit = "procurement.edit"

	// Finance AP permissions
	PermFinanceAPView    = "finance.ap.view"
	PermFinanceAPCreate  = "finance.ap.create"
	PermFinanceAPPost    = "finance.ap.post"
	PermFinanceAPVoid    = "finance.ap.void"
	PermFinanceAPPayment = "finance.ap.payment"
)

// CoreScopes lists all permissions related to the core platform.
func CoreScopes() []string {
	return []string{
		PermUsersView,
		PermUsersEdit,
		PermRolesView,
		PermRolesEdit,
		PermPermissionsView,
		PermPermissionsAssign,
		PermPermissionsReview,
		PermOrgView,
		PermOrgEdit,
		PermMasterView,
		PermMasterEdit,
		PermMasterImport,
		PermRBACView,
		PermRBACEdit,
		PermReportView,
		PermInventoryView,
		PermInventoryEdit,
		PermProcurementView,
		PermProcurementEdit,
		PermFinanceAPView,
		PermFinanceAPCreate,
		PermFinanceAPPost,
		PermFinanceAPVoid,
		PermFinanceAPPayment,
	}
}
