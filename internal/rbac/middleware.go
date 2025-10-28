package rbac

import (
	"net/http"
	"strconv"
	"strings"

	"log/slog"

	"github.com/odyssey-erp/odyssey-erp/internal/shared"
)

// Middleware wires RBAC authorization helpers for HTTP handlers.
type Middleware struct {
	Service *Service
	Logger  *slog.Logger
}

// RequireAny ensures the current user has at least one of the required permissions.
func (m Middleware) RequireAny(perms ...string) func(http.Handler) http.Handler {
	normalized := normalizePermissions(perms)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if len(normalized) == 0 {
				next.ServeHTTP(w, r)
				return
			}
			userID, ok := m.currentUserID(r)
			if !ok {
				http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
				return
			}
			granted, err := m.Service.EffectivePermissions(r.Context(), userID)
			if err != nil {
				if m.Logger != nil {
					m.Logger.Error("rbac require any", slog.Any("error", err))
				}
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
				return
			}
			if hasAnyPermission(granted, normalized) {
				next.ServeHTTP(w, r)
				return
			}
			http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
		})
	}
}

// RequireAll ensures the current user has all required permissions.
func (m Middleware) RequireAll(perms ...string) func(http.Handler) http.Handler {
	normalized := normalizePermissions(perms)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if len(normalized) == 0 {
				next.ServeHTTP(w, r)
				return
			}
			userID, ok := m.currentUserID(r)
			if !ok {
				http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
				return
			}
			granted, err := m.Service.EffectivePermissions(r.Context(), userID)
			if err != nil {
				if m.Logger != nil {
					m.Logger.Error("rbac require all", slog.Any("error", err))
				}
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
				return
			}
			if hasAllPermissions(granted, normalized) {
				next.ServeHTTP(w, r)
				return
			}
			http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
		})
	}
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
