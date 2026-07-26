package e2e

import (
	"net/http"
	"strings"
	"testing"
)

// isPageRoute decides the entire coverage set of the sweep, so its exclusions
// are worth pinning down. These run without a deployed instance.
func TestIsPageRoute(t *testing.T) {
	pages := []string{
		"/",
		"/accounting/pnl",
		"/accounting/balance-sheet",
		"/finance/banking/accounts",
		"/masterdata/products/new",
		"/jobs",
	}
	for _, pattern := range pages {
		if !isPageRoute(pattern) {
			t.Errorf("isPageRoute(%q) = false, want true", pattern)
		}
	}

	nonPages := []string{
		"",
		"relative/path",
		"/finance/banking/accounts/{id}",     // needs a real identifier
		"/static/*",                          // asset handler
		"/auth/login",                        // public, no app shell
		"/welcome",                           // public landing page
		"/metrics",                           // operational
		"/metrics/prometheus",                // operational
		"/accounting/pnl/export.xlsx",        // file payload
		"/accounting/budget/export.xlsx",     // file payload
		"/report/ping",                       // Gotenberg diagnostic
		"/finance/reports/trial-balance/pdf", // file payload
	}
	for _, pattern := range nonPages {
		if isPageRoute(pattern) {
			t.Errorf("isPageRoute(%q) = true, want false", pattern)
		}
	}
}

// A prefix match must respect path segments: /apirouter is not under /api.
func TestIsPageRouteMatchesWholeSegments(t *testing.T) {
	if !isPageRoute("/apiary/hives") {
		t.Error(`isPageRoute("/apiary/hives") = false, want true: "/api" must not match a partial segment`)
	}
	if isPageRoute("/api/v1/accounts") {
		t.Error(`isPageRoute("/api/v1/accounts") = true, want false`)
	}
}

func TestDetailPatternsSelectsParameterisedPages(t *testing.T) {
	entries := []routeEntry{
		{Method: http.MethodGet, Pattern: "/masterdata/units/{id}"},
		{Method: http.MethodGet, Pattern: "/masterdata/units/{id}/edit"},
		{Method: http.MethodGet, Pattern: "/masterdata/units"},          // not parameterised
		{Method: http.MethodGet, Pattern: "/board-packs/{id}/download"}, // file payload
		{Method: http.MethodGet, Pattern: "/static/*"},                  // wildcard
		{Method: http.MethodPost, Pattern: "/masterdata/units/{id}"},    // not a GET
	}

	got := detailPatterns(entries)
	want := []string{"/masterdata/units/{id}", "/masterdata/units/{id}/edit"}
	if len(got) != len(want) {
		t.Fatalf("detailPatterns() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("detailPatterns()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestPatternMatcherBindsOneSegmentPerParam(t *testing.T) {
	matcher := patternMatcher("/masterdata/units/{id}")
	for _, path := range []string{"/masterdata/units/7", "/masterdata/units/abc"} {
		if !matcher.MatchString(path) {
			t.Errorf("%q should match /masterdata/units/{id}", path)
		}
	}
	// A parameter must not swallow further segments, or an edit URL would be
	// mistaken for a detail URL and the edit route would go unchecked.
	for _, path := range []string{"/masterdata/units", "/masterdata/units/7/edit", "/masterdata/unitsX/7"} {
		if matcher.MatchString(path) {
			t.Errorf("%q should not match /masterdata/units/{id}", path)
		}
	}
}

func TestHarvestLinksCollectsRowNavigationAttributes(t *testing.T) {
	body := `<tr data-row-id="3" data-href="/masterdata/units/3" data-edit-href="/masterdata/units/3/edit">
	          <a href="/sales/orders/12">SO-12</a>
	          <a href="/accounting/pnl/export.xlsx?period=2026-07">Export</a>
	          <a href="https://example.com/external">External</a>
	          <a href="#top">Top</a></tr>`

	links := make(map[string]struct{})
	harvestLinks(body, links)

	for _, want := range []string{
		"/masterdata/units/3",
		"/masterdata/units/3/edit",
		"/sales/orders/12",
		"/accounting/pnl/export.xlsx", // query stripped
	} {
		if _, ok := links[want]; !ok {
			t.Errorf("harvestLinks did not collect %q (got %v)", want, links)
		}
	}
	// Off-site and fragment-only links are not in-app paths.
	for _, unwanted := range []string{"https://example.com/external", "#top"} {
		if _, ok := links[unwanted]; ok {
			t.Errorf("harvestLinks should not collect %q", unwanted)
		}
	}
}

func TestResolveDetailPagesPairsPatternsWithRealPaths(t *testing.T) {
	links := map[string]struct{}{
		"/masterdata/units/3":      {},
		"/masterdata/units/3/edit": {},
		"/masterdata/units":        {},
	}
	patterns := []string{
		"/masterdata/units/{id}",
		"/masterdata/units/{id}/edit",
		"/sales/orders/{id}", // nothing discovered for this one
	}

	pages, unresolved := resolveDetailPages(patterns, links, nil)
	if len(pages) != 2 {
		t.Fatalf("resolveDetailPages() returned %d pages, want 2: %+v", len(pages), pages)
	}
	byPattern := map[string]string{}
	for _, page := range pages {
		byPattern[page.description] = page.path
	}
	if byPattern["/masterdata/units/{id}"] != "/masterdata/units/3" {
		t.Errorf("detail path = %q, want /masterdata/units/3", byPattern["/masterdata/units/{id}"])
	}
	if byPattern["/masterdata/units/{id}/edit"] != "/masterdata/units/3/edit" {
		t.Errorf("edit path = %q, want /masterdata/units/3/edit", byPattern["/masterdata/units/{id}/edit"])
	}
	if len(unresolved) != 1 || unresolved[0] != "/sales/orders/{id}" {
		t.Errorf("unresolved = %v, want [/sales/orders/{id}]", unresolved)
	}
}

// Edit routes are linked only from their own detail page, so resolution has to
// improve as later rounds harvest more links. This models that progression.
func TestResolveDetailPagesImprovesAsLinksAccumulate(t *testing.T) {
	patterns := []string{"/sales/orders/{id}", "/sales/orders/{id}/edit"}

	// Round one: only the listing has been seen, which links detail but not edit.
	links := map[string]struct{}{"/sales/orders/12": {}}
	batch, unresolved := resolveDetailPages(patterns, links, nil)
	if len(batch) != 1 || batch[0].path != "/sales/orders/12" {
		t.Fatalf("round 1 batch = %+v, want just the detail page", batch)
	}
	if len(unresolved) != 1 || unresolved[0] != "/sales/orders/{id}/edit" {
		t.Fatalf("round 1 unresolved = %v, want the edit route", unresolved)
	}

	// Round two: visiting the detail page contributed its edit link.
	harvestLinks(`<a href="/sales/orders/12/edit">Edit</a>`, links)
	batch, unresolved = resolveDetailPages(unresolved, links, nil)
	if len(unresolved) != 0 {
		t.Fatalf("round 2 unresolved = %v, want none", unresolved)
	}
	if len(batch) != 1 || batch[0].path != "/sales/orders/12/edit" {
		t.Fatalf("round 2 batch = %+v, want the edit page", batch)
	}
}

// A route parameter matches any single segment, so a concrete sibling route
// such as /masterdata/units/new would otherwise satisfy /masterdata/units/{id}
// and report coverage for a detail page that was never fetched. That is only
// visible when the entity has no rows, since a numeric id sorts first.
func TestResolveDetailPagesIgnoresConcreteSiblingRoutes(t *testing.T) {
	patterns := []string{"/masterdata/units/{id}"}
	reserved := map[string]struct{}{"/masterdata/units/new": {}}

	// No rows seeded: the only candidate link is the creation form.
	links := map[string]struct{}{"/masterdata/units/new": {}}
	pages, unresolved := resolveDetailPages(patterns, links, reserved)
	if len(pages) != 0 {
		t.Fatalf("resolved %+v from a creation form, want no coverage", pages)
	}
	if len(unresolved) != 1 {
		t.Fatalf("unresolved = %v, want the pattern reported as uncovered", unresolved)
	}

	// With a real row present the detail page resolves normally.
	links["/masterdata/units/4"] = struct{}{}
	pages, unresolved = resolveDetailPages(patterns, links, reserved)
	if len(unresolved) != 0 {
		t.Fatalf("unresolved = %v, want none", unresolved)
	}
	if len(pages) != 1 || pages[0].path != "/masterdata/units/4" {
		t.Fatalf("pages = %+v, want /masterdata/units/4", pages)
	}
}

func TestMutatingRoutesSelectsStateChangingMethods(t *testing.T) {
	entries := []routeEntry{
		{Method: http.MethodPost, Pattern: "/masterdata/units"},
		{Method: http.MethodPost, Pattern: "/masterdata/units/{id}/delete"},
		{Method: http.MethodGet, Pattern: "/masterdata/units"},     // reads
		{Method: http.MethodHead, Pattern: "/masterdata/units"},    // CSRF skips
		{Method: http.MethodOptions, Pattern: "/masterdata/units"}, // CSRF skips
		{Method: http.MethodDelete, Pattern: "/static/*"},          // file server
		{Method: http.MethodPut, Pattern: "/static/*"},             // file server
	}

	got := mutatingRoutes(entries)
	if len(got) != 2 {
		t.Fatalf("mutatingRoutes() = %+v, want the two POST routes", got)
	}
	for i, want := range []string{"/masterdata/units", "/masterdata/units/{id}/delete"} {
		if got[i].Pattern != want || got[i].Method != http.MethodPost {
			t.Errorf("route %d = %s %s, want POST %s", i, got[i].Method, got[i].Pattern, want)
		}
	}
}

// Probing a mutation must never address a real record: if a route turns out to
// be unguarded, the request has to land on nothing rather than delete data.
func TestConcreteMutationPathAvoidsRealRecords(t *testing.T) {
	for _, tc := range []struct{ pattern, want string }{
		{"/masterdata/units", "/masterdata/units"},
		{"/masterdata/units/{id}/delete", "/masterdata/units/" + unreachableID + "/delete"},
		{"/accounting/journals/{id}/reverse", "/accounting/journals/" + unreachableID + "/reverse"},
	} {
		if got := concreteMutationPath(tc.pattern); got != tc.want {
			t.Errorf("concreteMutationPath(%q) = %q, want %q", tc.pattern, got, tc.want)
		}
	}
	if strings.Contains(concreteMutationPath("/masterdata/units/{id}/delete"), "{") {
		t.Error("route parameters must be substituted, not passed through literally")
	}
}
