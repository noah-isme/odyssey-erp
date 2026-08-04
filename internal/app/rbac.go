package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/odyssey-erp/odyssey-erp/internal/shared"
)

// Permission represents an RBAC permission
type Permission string

// Define all system permissions across phases
const (
	// Phase 1-2: Core Permissions
	PermViewDashboard     Permission = "view:dashboard"
	PermViewGL            Permission = "view:gl"
	PermPostGL            Permission = "post:gl"
	PermViewAR            Permission = "view:ar"
	PermPostAR            Permission = "post:ar"
	PermViewAP            Permission = "view:ap"
	PermPostAP            Permission = "post:ap"
	PermViewSales         Permission = "view:sales"
	PermCreateSales       Permission = "create:sales"
	PermViewInventory     Permission = "view:inventory"
	PermPostInventory     Permission = "post:inventory"

	// Phase 2: Sourcing Permissions
	PermViewRFQ           Permission = "view:rfq"
	PermCreateRFQ         Permission = "create:rfq"
	PermApproveRFQ        Permission = "approve:rfq"
	PermViewAwards        Permission = "view:awards"
	PermCreateAwards      Permission = "create:awards"
	PermApproveAwards     Permission = "approve:awards"

	// Phase 3: Vendor Intelligence Permissions
	PermViewContracts     Permission = "view:contracts"
	PermCreateContracts   Permission = "create:contracts"
	PermEditContracts     Permission = "edit:contracts"
	PermApproveContracts  Permission = "approve:contracts"
	PermViewScorecards    Permission = "view:scorecards"
	PermCreateScorecards  Permission = "create:scorecards"
	PermViewVariances     Permission = "view:variances"
	PermApproveVariances  Permission = "approve:variances"
	PermViewPriceHistory  Permission = "view:price_history"

	// Phase 4: Transport Execution Permissions
	PermViewCarriers      Permission = "view:carriers"
	PermCreateCarriers    Permission = "create:carriers"
	PermEditCarriers      Permission = "edit:carriers"
	PermViewFleet         Permission = "view:fleet"
	PermCreateFleet       Permission = "create:fleet"
	PermEditFleet         Permission = "edit:fleet"
	PermViewShipments     Permission = "view:shipments"
	PermCreateShipments   Permission = "create:shipments"
	PermDispatchShipments Permission = "dispatch:shipments"
	PermViewTrips         Permission = "view:trips"
	PermCreateTrips       Permission = "create:trips"
	PermTrackShipments    Permission = "track:shipments"
	PermViewRates         Permission = "view:rates"
	PermCreateRates       Permission = "create:rates"

	// Phase 5: Distribution Planning Permissions
	PermViewLoads         Permission = "view:loads"
	PermCreateLoads       Permission = "create:loads"
	PermEditLoads         Permission = "edit:loads"
	PermDispatchLoads     Permission = "dispatch:loads"
	PermViewRoutes        Permission = "view:routes"
	PermCreateRoutes      Permission = "create:routes"
	PermOptimizeRoutes    Permission = "optimize:routes"
	PermApproveRoutes     Permission = "approve:routes"
	PermViewTransfers     Permission = "view:transfers"
	PermCreateTransfers   Permission = "create:transfers"
	PermApproveTransfers  Permission = "approve:transfers"
	PermDispatchTransfers Permission = "dispatch:transfers"
	PermViewPlanningHorizon Permission = "view:planning_horizon"
	PermCreatePlanningHorizon Permission = "create:planning_horizon"
)

// PermissionChecker validates if a user has required permissions
type PermissionChecker struct {
	rolePermissions map[string][]Permission
	logger          *slog.Logger
}

// NewPermissionChecker creates a new permission checker with default role mappings
func NewPermissionChecker(logger *slog.Logger) *PermissionChecker {
	pc := &PermissionChecker{
		rolePermissions: make(map[string][]Permission),
		logger:          logger,
	}
	pc.initializeDefaultRoles()
	return pc
}

// initializeDefaultRoles sets up standard role hierarchies
func (pc *PermissionChecker) initializeDefaultRoles() {
	// Admin: All permissions
	adminPerms := []Permission{
		PermViewDashboard, PermViewGL, PermPostGL,
		PermViewAR, PermPostAR, PermViewAP, PermPostAP,
		PermViewSales, PermCreateSales,
		PermViewInventory, PermPostInventory,
		PermViewRFQ, PermCreateRFQ, PermApproveRFQ,
		PermViewAwards, PermCreateAwards, PermApproveAwards,
		PermViewContracts, PermCreateContracts, PermEditContracts, PermApproveContracts,
		PermViewScorecards, PermCreateScorecards,
		PermViewVariances, PermApproveVariances, PermViewPriceHistory,
		PermViewCarriers, PermCreateCarriers, PermEditCarriers,
		PermViewFleet, PermCreateFleet, PermEditFleet,
		PermViewShipments, PermCreateShipments, PermDispatchShipments,
		PermViewTrips, PermCreateTrips, PermTrackShipments,
		PermViewRates, PermCreateRates,
		PermViewLoads, PermCreateLoads, PermEditLoads, PermDispatchLoads,
		PermViewRoutes, PermCreateRoutes, PermOptimizeRoutes, PermApproveRoutes,
		PermViewTransfers, PermCreateTransfers, PermApproveTransfers, PermDispatchTransfers,
		PermViewPlanningHorizon, PermCreatePlanningHorizon,
	}
	pc.rolePermissions["admin"] = adminPerms

	// Procurement Officer
	procurementPerms := []Permission{
		PermViewDashboard, PermViewGL, PermViewAR, PermViewAP,
		PermViewSales, PermViewInventory,
		PermViewRFQ, PermCreateRFQ, PermApproveRFQ,
		PermViewAwards, PermCreateAwards, PermApproveAwards,
		PermViewContracts, PermCreateContracts, PermEditContracts, PermApproveContracts,
		PermViewScorecards, PermViewVariances, PermApproveVariances,
		PermViewPriceHistory,
	}
	pc.rolePermissions["procurement"] = procurementPerms

	// Logistics Manager
	logisticsPerms := []Permission{
		PermViewDashboard, PermViewGL,
		PermViewCarriers, PermCreateCarriers, PermEditCarriers,
		PermViewFleet, PermCreateFleet, PermEditFleet,
		PermViewShipments, PermCreateShipments, PermDispatchShipments,
		PermViewTrips, PermCreateTrips, PermTrackShipments,
		PermViewRates,
		PermViewLoads, PermCreateLoads, PermEditLoads, PermDispatchLoads,
		PermViewRoutes, PermCreateRoutes, PermOptimizeRoutes, PermApproveRoutes,
		PermViewTransfers, PermCreateTransfers, PermApproveTransfers, PermDispatchTransfers,
		PermViewPlanningHorizon, PermCreatePlanningHorizon,
	}
	pc.rolePermissions["logistics"] = logisticsPerms

	// Finance
	financePerms := []Permission{
		PermViewDashboard,
		PermViewGL, PermPostGL,
		PermViewAR, PermViewAP,
		PermViewSales, PermViewInventory,
		PermViewContracts, PermViewPriceHistory,
		PermViewShipments, PermViewRates,
		PermViewLoads, PermViewRoutes, PermViewTransfers,
	}
	pc.rolePermissions["finance"] = financePerms

	// Viewer (read-only)
	viewerPerms := []Permission{
		PermViewDashboard,
		PermViewGL, PermViewAR, PermViewAP,
		PermViewSales, PermViewInventory,
		PermViewRFQ, PermViewAwards,
		PermViewContracts, PermViewScorecards, PermViewVariances, PermViewPriceHistory,
		PermViewCarriers, PermViewFleet,
		PermViewShipments, PermViewTrips, PermViewRates,
		PermViewLoads, PermViewRoutes, PermViewTransfers,
		PermViewPlanningHorizon,
	}
	pc.rolePermissions["viewer"] = viewerPerms
}

// HasPermission checks if user has the required permission
func (pc *PermissionChecker) HasPermission(userRole string, permission Permission) bool {
	perms, exists := pc.rolePermissions[userRole]
	if !exists {
		return false
	}
	for _, p := range perms {
		if p == permission {
			return true
		}
	}
	return false
}

// HasAnyPermission checks if user has any of the required permissions
func (pc *PermissionChecker) HasAnyPermission(userRole string, permissions ...Permission) bool {
	for _, perm := range permissions {
		if pc.HasPermission(userRole, perm) {
			return true
		}
	}
	return false
}

// RequirePermission returns a middleware that enforces a permission
func RequirePermission(checker *PermissionChecker, permission Permission) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sess := shared.SessionFromContext(r.Context())
			if sess == nil {
				http.Redirect(w, r, "/login", http.StatusSeeOther)
				return
			}

			userRole := sess.Get("role")
			if userRole == "" {
				http.Error(w, "Forbidden: No role assigned", http.StatusForbidden)
				return
			}

			if !checker.HasPermission(userRole, permission) {
				checker.logger.Warn("permission_denied",
					slog.String("role", userRole),
					slog.String("permission", string(permission)),
					slog.String("path", r.URL.Path),
				)
				http.Error(w, fmt.Sprintf("Forbidden: Missing permission %s", permission), http.StatusForbidden)
				return
			}

			checker.logger.Debug("permission_granted",
				slog.String("role", userRole),
				slog.String("permission", string(permission)),
			)

			ctx := ContextWithPermissionChecker(r.Context(), checker)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireAnyPermission returns a middleware that enforces any of multiple permissions
func RequireAnyPermission(checker *PermissionChecker, permissions ...Permission) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sess := shared.SessionFromContext(r.Context())
			if sess == nil {
				http.Redirect(w, r, "/login", http.StatusSeeOther)
				return
			}

			userRole := sess.Get("role")
			if userRole == "" {
				http.Error(w, "Forbidden: No role assigned", http.StatusForbidden)
				return
			}

			if !checker.HasAnyPermission(userRole, permissions...) {
				checker.logger.Warn("permission_denied_any",
					slog.String("role", userRole),
					slog.String("path", r.URL.Path),
				)
				http.Error(w, "Forbidden: Missing required permissions", http.StatusForbidden)
				return
			}

			ctx := ContextWithPermissionChecker(r.Context(), checker)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

type permissionContextKey string
const permissionCheckerKey = permissionContextKey("permissionChecker")

// ContextWithPermissionChecker adds permission checker to context
func ContextWithPermissionChecker(ctx context.Context, checker *PermissionChecker) context.Context {
	return context.WithValue(ctx, permissionCheckerKey, checker)
}

// PermissionCheckerFromContext extracts permission checker from context
func PermissionCheckerFromContext(ctx context.Context) *PermissionChecker {
	checker, ok := ctx.Value(permissionCheckerKey).(*PermissionChecker)
	if !ok {
		return nil
	}
	return checker
}

// CheckPermission is a utility function to check permissions at handler level
func CheckPermission(ctx context.Context, permission Permission) bool {
	sess := shared.SessionFromContext(ctx)
	if sess == nil {
		return false
	}

	userRole := sess.Get("role")
	if userRole == "" {
		return false
	}

	checker := PermissionCheckerFromContext(ctx)
	if checker == nil {
		return false
	}

	return checker.HasPermission(userRole, permission)
}
