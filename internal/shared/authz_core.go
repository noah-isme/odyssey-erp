package shared

// Core platform permissions.
const (
	PermUsersView = "users.view"
	PermUsersEdit = "users.edit"

	PermRolesView = "roles.view"
	PermRolesEdit = "roles.edit"

	PermPermissionsView = "permissions.view"
)

// CoreScopes lists all permissions related to the core platform.
func CoreScopes() []string {
	return []string{
		PermUsersView,
		PermUsersEdit,
		PermRolesView,
		PermRolesEdit,
		PermPermissionsView,
	}
}
