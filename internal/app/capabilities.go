package app

import (
	"errors"
	"net/http"
	pathpkg "path"
	"sort"
	"strings"
)

// ReleaseProfile identifies a bounded set of routes that may be advertised
// and served by a deployment.
type ReleaseProfile string

const (
	ReleaseProfileV010Core    ReleaseProfile = "v0.10-core"
	ReleaseProfileV011Finance ReleaseProfile = "v0.11-finance"
	ReleaseProfileFull        ReleaseProfile = "full"
)

var ErrInvalidReleaseProfile = errors.New("invalid release profile")

// ParseReleaseProfile validates and normalizes the RELEASE_PROFILE value.
func ParseReleaseProfile(value string) (ReleaseProfile, error) {
	switch ReleaseProfile(strings.ToLower(strings.TrimSpace(value))) {
	case ReleaseProfileV010Core:
		return ReleaseProfileV010Core, nil
	case ReleaseProfileV011Finance:
		return ReleaseProfileV011Finance, nil
	case ReleaseProfileFull:
		return ReleaseProfileFull, nil
	default:
		return "", ErrInvalidReleaseProfile
	}
}

type Capability string

const (
	CapabilityARAP          Capability = "ar-ap"
	CapabilitySalesDelivery Capability = "sales-delivery"
	CapabilityInventory     Capability = "inventory"
	CapabilityDocuments     Capability = "documents"
	CapabilityCMMS          Capability = "cmms"
)

var coreCapabilities = []Capability{
	CapabilityARAP,
	CapabilitySalesDelivery,
	CapabilityInventory,
	CapabilityDocuments,
	CapabilityCMMS,
}

// CoreCapabilities returns a copy so callers cannot mutate the registry.
func CoreCapabilities() []Capability {
	return append([]Capability(nil), coreCapabilities...)
}

// CoreRoutePolicy exposes only the core workflow prefixes and the supporting
// platform endpoints required to operate them. Exact paths are kept separate
// so /settings/integrations and other preview surfaces cannot be admitted by a
// broad /settings rule.
var coreRoutePrefixes = []string{
	"/auth",
	"/finance/ar",
	"/finance/ap",
	"/sales",
	"/delivery",
	"/inventory",
	"/documents",
	"/cmms",
	"/accounting",
	"/masterdata",
	"/approvals",
	"/users",
	"/roles",
	"/permissions",
	"/audit",
	"/api/dashboard",
	"/api/notifications",
	"/api/search",
	"/static",
}

// financeAutomationRoutePrefixes are the finance workflows included by the
// cumulative v0.11-finance profile. The profile deliberately does not widen
// access to the other finance surfaces that remain outside its release claim.
var financeAutomationRoutePrefixes = []string{
	"/finance/bankfeeds",
	"/finance/forecasting",
	"/finance/treasury",
}

var coreRouteExact = []string{
	"/",
	"/healthz",
	"/welcome",
	"/profile",
	"/settings",
	"/settings/security/password",
	"/company/select",
	"/api/me",
}

// These routes are implemented foundations but remain outside the bounded
// v0.10-core claim until provider/realtime certification is complete.
var coreRouteBlockedPrefixes = []string{
	"/documents/search",
	"/cmms/iot",
	"/cmms/predictive",
}

// AllowedPathPrefixes returns a sorted copy suitable for API responses and
// shell navigation filtering.
func (p ReleaseProfile) AllowedPathPrefixes() []string {
	if p == ReleaseProfileFull {
		return []string{"/"}
	}
	result := p.boundedRoutePrefixes()
	if len(result) == 0 {
		return nil
	}
	sort.Strings(result)
	return result
}

// AllowedExactPaths returns exact paths admitted by a bounded profile.
func (p ReleaseProfile) AllowedExactPaths() []string {
	if p == ReleaseProfileFull {
		return []string{"/"}
	}
	if len(p.boundedRoutePrefixes()) == 0 {
		return nil
	}
	result := append([]string(nil), coreRouteExact...)
	sort.Strings(result)
	return result
}

func (p ReleaseProfile) boundedRoutePrefixes() []string {
	switch p {
	case ReleaseProfileV010Core:
		return append([]string(nil), coreRoutePrefixes...)
	case ReleaseProfileV011Finance:
		result := append([]string(nil), coreRoutePrefixes...)
		return append(result, financeAutomationRoutePrefixes...)
	default:
		return nil
	}
}

// AllowsPath reports whether a request path belongs to the selected profile.
// Full releases leave the route surface unchanged.
func (p ReleaseProfile) AllowsPath(path string) bool {
	if p == ReleaseProfileFull {
		return true
	}
	allowedPrefixes := p.boundedRoutePrefixes()
	if len(allowedPrefixes) == 0 {
		return false
	}
	path = pathpkg.Clean(strings.TrimSpace(path))
	if path == "." {
		path = "/"
	}
	path = strings.TrimSuffix(path, "/")
	if path == "" {
		path = "/"
	}
	for _, blocked := range coreRouteBlockedPrefixes {
		if path == blocked || strings.HasPrefix(path, blocked+"/") {
			return false
		}
	}
	if strings.HasSuffix(path, "/ocr") {
		return false
	}
	for _, exact := range coreRouteExact {
		if path == exact {
			return true
		}
	}
	for _, prefix := range allowedPrefixes {
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return true
		}
	}
	return false
}

// RequiresScopedAccess reports whether a profile is a bounded release profile
// whose routes must enforce company/branch scope checks.
func (p ReleaseProfile) RequiresScopedAccess() bool {
	return p == ReleaseProfileV010Core || p == ReleaseProfileV011Finance
}

// ReleaseProfileMiddleware turns unsupported preview routes into ordinary 404
// responses. Route registration remains unchanged, which keeps route dumps and
// local development behavior stable while making the production profile a hard
// boundary.
func ReleaseProfileMiddleware(profile string) func(http.Handler) http.Handler {
	parsed, err := ParseReleaseProfile(profile)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// An invalid profile must fail closed. Config validation catches this
			// during startup, but keeping the middleware defensive prevents a
			// miswired router from silently exposing the full route surface.
			if err == nil && parsed.AllowsPath(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}
			http.NotFound(w, r)
		})
	}
}
