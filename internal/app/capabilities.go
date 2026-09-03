package app

import (
	"errors"
	"net/http"
	pathpkg "path"
	"sort"
	"strings"
)

// ReleaseProfile identifies the bounded route surface enabled by a deployment.
type ReleaseProfile string

const (
	ReleaseProfileV010Core    ReleaseProfile = "v0.10-core"
	ReleaseProfileV011Finance ReleaseProfile = "v0.11-finance"
	ReleaseProfileFull        ReleaseProfile = "full"
)

var ErrInvalidReleaseProfile = errors.New("invalid release profile")

// ParseReleaseProfile validates and normalizes RELEASE_PROFILE.
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

// Capability names the release-scoped workflow families.
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

// These prefixes cover the bounded v0.10-core journeys and their shared support
// surfaces. More specialized routes remain unavailable until separately certified.
var coreRoutePrefixes = []string{
	"/auth",
	"/finance/ar",
	"/finance/ap",
	"/finance/banking",
	"/finance/consol",
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
	"/legal",
	"/static",
	"/hr",
	"/payroll",
	"/procurement",
	"/logistics",
	"/freight",
	"/distribution",
	"/consol",
	"/eliminations",
	"/variance",
	"/crm",
	"/pos",
	"/mrp",
	"/qms",
	"/wms",
	"/projects",
	"/tax",
	"/analytics",
	"/insights",
	"/board-packs",
	"/jobs",
	"/metrics",
	"/portal",
	"/report",
	"/settings/integrations",
}

// v0.11-finance is cumulative: it adds the bounded treasury/P2P automation
// surfaces without widening the rest of the product route set.
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

var coreRouteBlockedPrefixes = []string{
	"/documents/search",
	"/cmms/iot",
	"/cmms/predictive",
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

// AllowedPathPrefixes returns a sorted copy suitable for route manifests.
func (p ReleaseProfile) AllowedPathPrefixes() []string {
	if p == ReleaseProfileFull {
		return []string{"/"}
	}
	result := p.boundedRoutePrefixes()
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

// AllowsPath reports whether a request path belongs to the selected profile.
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

// RequiresScopedAccess reports whether the profile is a bounded release profile.
func (p ReleaseProfile) RequiresScopedAccess() bool {
	return p == ReleaseProfileV010Core || p == ReleaseProfileV011Finance
}

// ReleaseProfileMiddleware turns unsupported preview routes into ordinary 404s.
func ReleaseProfileMiddleware(profile string) func(http.Handler) http.Handler {
	parsed, err := ParseReleaseProfile(profile)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if err == nil && parsed.AllowsPath(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}
			http.NotFound(w, r)
		})
	}
}
