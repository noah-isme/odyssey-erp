package portal

import "context"

// Test-only access to unexported helpers for the external portal_test package
// (which imports internal/app to run requests through the real middleware
// stack; internal/app imports portal, so those tests cannot live in package
// portal).
var (
	InvitationTokenForTest = invitationToken
	PortalUserForTest      = portalUser
)

// SetEmployeeResolverForTest replaces the hr_employees lookup behind
// access(r, "EMPLOYEE").
func (h *Handler) SetEmployeeResolverForTest(f func(ctx context.Context, userID, companyID int64) (int64, error)) {
	h.resolveEmployee = f
}
