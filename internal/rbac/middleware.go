package rbac

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"log/slog"

	"github.com/odyssey-erp/odyssey-erp/internal/shared"
)

// Middleware wires RBAC authorization helpers for HTTP handlers.
type Middleware struct {
	Service PermissionReader
	Logger  *slog.Logger
}

// PermissionReader is the minimal service contract required by authorization middleware.
type PermissionReader interface {
	EffectivePermissions(ctx context.Context, userID int64) ([]string, error)
}

// ScopedPermissionReader is the optional tenant-aware extension required by
// the scoped middleware helpers. Keeping it separate preserves compatibility
// with legacy readers that only implement global permission checks.
type ScopedPermissionReader interface {
	EffectivePermissionsInScope(ctx context.Context, userID int64, scope AccessScope, at time.Time) ([]string, error)
}

type scopedRouteContextKey struct{}

// ScopedRoute marks a route group as tenant-scoped. The existing RequireAny
// and RequireAll helpers keep their global compatibility behavior everywhere
// else, but enforce the exact permission in the active company/branch when
// this marker is present. This lets existing module route declarations adopt
// scoped checks without duplicating every permission string at the router.
func ScopedRoute(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), scopedRouteContextKey{}, true)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func isScopedRoute(ctx context.Context) bool {
	scoped, _ := ctx.Value(scopedRouteContextKey{}).(bool)
	return scoped
}

// RequireAny ensures the current user has at least one of the required permissions.
func (m Middleware) RequireAny(perms ...string) func(http.Handler) http.Handler {
	normalized := normalizePermissions(perms)
	return func(next http.Handler) http.Handler {
		scopedNext := m.RequireAnyInScope(perms...)(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if len(normalized) == 0 {
				next.ServeHTTP(w, r)
				return
			}
			if isScopedRoute(r.Context()) {
				scopedNext.ServeHTTP(w, r)
				return
			}
			userID, ok := m.currentUserID(r)
			if !ok {
				shared.WriteHTTPError(w, http.StatusForbidden, "")
				return
			}
			granted, err := m.Service.EffectivePermissions(r.Context(), userID)
			if err != nil {
				if m.Logger != nil {
					m.Logger.Error("rbac require any", slog.Any("error", err))
				}
				shared.WriteHTTPError(w, http.StatusInternalServerError, "")
				return
			}
			if hasAnyPermission(granted, normalized) {
				next.ServeHTTP(w, r)
				return
			}
			shared.WriteHTTPError(w, http.StatusForbidden, "")
		})
	}
}

// RequireAll ensures the current user has all required permissions.
func (m Middleware) RequireAll(perms ...string) func(http.Handler) http.Handler {
	normalized := normalizePermissions(perms)
	return func(next http.Handler) http.Handler {
		scopedNext := m.RequireAllInScope(perms...)(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if len(normalized) == 0 {
				next.ServeHTTP(w, r)
				return
			}
			if isScopedRoute(r.Context()) {
				scopedNext.ServeHTTP(w, r)
				return
			}
			userID, ok := m.currentUserID(r)
			if !ok {
				shared.WriteHTTPError(w, http.StatusForbidden, "")
				return
			}
			granted, err := m.Service.EffectivePermissions(r.Context(), userID)
			if err != nil {
				if m.Logger != nil {
					m.Logger.Error("rbac require all", slog.Any("error", err))
				}
				shared.WriteHTTPError(w, http.StatusInternalServerError, "")
				return
			}
			if hasAllPermissions(granted, normalized) {
				next.ServeHTTP(w, r)
				return
			}
			shared.WriteHTTPError(w, http.StatusForbidden, "")
		})
	}
}

// RequireAnyInScope ensures the current user has at least one of the required
// permissions in the active company and optional branch scope.
func (m Middleware) RequireAnyInScope(perms ...string) func(http.Handler) http.Handler {
	return m.requireInScope(normalizePermissions(perms), false)
}

// RequireAllInScope ensures the current user has all required permissions in
// the active company and optional branch scope.
func (m Middleware) RequireAllInScope(perms ...string) func(http.Handler) http.Handler {
	return m.requireInScope(normalizePermissions(perms), true)
}

func (m Middleware) requireInScope(required []string, requireAll bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if len(required) == 0 {
				next.ServeHTTP(w, r)
				return
			}

			identity, ok := shared.IdentityFromContext(r.Context())
			if !ok {
				shared.WriteHTTPError(w, http.StatusForbidden, "")
				return
			}

			scope, ok := m.currentScope(r, identity.CompanyID)
			if !ok {
				shared.WriteHTTPError(w, http.StatusForbidden, "")
				return
			}

			reader, ok := m.scopedReader()
			if !ok {
				shared.WriteHTTPError(w, http.StatusForbidden, "")
				return
			}

			// Every permission check in this request uses one UTC evaluation
			// instant so effective-dated assignments cannot straddle a boundary.
			at := time.Now().UTC()
			granted, err := reader.EffectivePermissionsInScope(r.Context(), identity.UserID, scope, at)
			if err != nil {
				if errors.Is(err, ErrScopedRepositoryUnavailable) || errors.Is(err, ErrInvalidScope) || errors.Is(err, ErrInvalidEffectiveTime) {
					shared.WriteHTTPError(w, http.StatusForbidden, "")
					return
				}
				if m.Logger != nil {
					m.Logger.Error("rbac scoped permission lookup", slog.Any("error", err))
				}
				shared.WriteHTTPError(w, http.StatusInternalServerError, "")
				return
			}

			allowed := hasAllPermissions(granted, required)
			if !requireAll {
				allowed = hasAnyPermission(granted, required)
			}
			if allowed {
				next.ServeHTTP(w, r)
				return
			}
			shared.WriteHTTPError(w, http.StatusForbidden, "")
		})
	}
}

func (m Middleware) scopedReader() (ScopedPermissionReader, bool) {
	if m.Service == nil {
		return nil, false
	}
	reader, ok := m.Service.(ScopedPermissionReader)
	return reader, ok
}

func (m Middleware) currentScope(r *http.Request, companyID int64) (AccessScope, bool) {
	sess := shared.SessionFromContext(r.Context())
	if sess == nil {
		return AccessScope{}, false
	}

	rawBranchID := strings.TrimSpace(sess.Get("branch_id"))
	if rawBranchID == "" {
		return AccessScope{CompanyID: companyID}, true
	}
	branchID, err := strconv.ParseInt(rawBranchID, 10, 64)
	if err != nil || branchID <= 0 {
		if m.Logger != nil {
			m.Logger.Error("rbac parse branch id", slog.String("value", rawBranchID))
		}
		return AccessScope{}, false
	}
	return AccessScope{CompanyID: companyID, BranchID: &branchID}, true
}

func (m Middleware) currentUserID(r *http.Request) (int64, bool) {
	sess := shared.SessionFromContext(r.Context())
	if sess == nil {
		return 0, false
	}
	raw := strings.TrimSpace(sess.User())
	if raw == "" {
		return 0, false
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		if m.Logger != nil {
			m.Logger.Error("rbac parse user id", slog.String("value", raw))
		}
		return 0, false
	}
	return id, true
}

func normalizePermissions(perms []string) []string {
	unique := make(map[string]struct{}, len(perms))
	for _, p := range perms {
		p = strings.TrimSpace(strings.ToLower(p))
		if p == "" {
			continue
		}
		unique[p] = struct{}{}
	}
	normalized := make([]string, 0, len(unique))
	for p := range unique {
		normalized = append(normalized, p)
	}
	return normalized
}

func hasAnyPermission(granted []string, required []string) bool {
	if len(required) == 0 {
		return true
	}
	set := make(map[string]struct{}, len(granted))
	for _, p := range granted {
		set[strings.ToLower(p)] = struct{}{}
	}
	for _, r := range required {
		if _, ok := set[r]; ok {
			return true
		}
	}
	return false
}

func hasAllPermissions(granted []string, required []string) bool {
	if len(required) == 0 {
		return true
	}
	set := make(map[string]struct{}, len(granted))
	for _, p := range granted {
		set[strings.ToLower(p)] = struct{}{}
	}
	for _, r := range required {
		if _, ok := set[r]; !ok {
			return false
		}
	}
	return true
}
