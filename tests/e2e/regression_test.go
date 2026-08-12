package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// These regression tests use only net/http and run against a deployed local,
// staging, or CI instance. Set ODYSSEY_E2E_URL to enable them.
// Optionally set GOTENBERG_URL to capture screenshots.
func TestRegressionFlow(t *testing.T) {
	baseURL := strings.TrimRight(os.Getenv("ODYSSEY_E2E_URL"), "/")
	if baseURL == "" {
		t.Skip("set ODYSSEY_E2E_URL to run HTTP E2E regression")
	}
	email := envOr("ODYSSEY_E2E_EMAIL", "admin@odyssey.local")
	password := envOr("ODYSSEY_E2E_PASSWORD", "admin123")
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar, CheckRedirect: func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }}

	csrf := fetchCSRF(t, client, baseURL+"/auth/login")
	form := url.Values{"email": {email}, "password": {password}, "csrf_token": {csrf}}
	response := postForm(t, client, baseURL+"/auth/login", form)
	if response.StatusCode != http.StatusSeeOther {
		t.Fatalf("login status = %d, want %d", response.StatusCode, http.StatusSeeOther)
	}
	_ = response.Body.Close()

	gotenbergURL := os.Getenv("GOTENBERG_URL")
	screenshotDir := os.Getenv("ODYSSEY_E2E_SCREENSHOT_DIR")
	if screenshotDir == "" && gotenbergURL != "" {
		screenshotDir = "test-screenshots"
	}
	// URL for Gotenberg to reach the app (may differ from baseURL if Gotenberg runs in container)
	gotenbergTargetURL := os.Getenv("GOTENBERG_TARGET_URL")
	if gotenbergTargetURL == "" {
		gotenbergTargetURL = baseURL
	}

	// The shell's company switcher and user menu depend on this endpoint.
	assertWorkspaceAPI(t, client, baseURL)

	pages, detailRoutes, reservedPaths, mutationRoutes := pagesUnderTest(t)
	t.Logf("checking %d page routes", len(pages))
	// Links seen while sweeping the listings supply real identifiers for the
	// parameterised detail and edit routes checked afterwards.
	discoveredLinks := make(map[string]struct{})

	// Start from a clean directory so a run never mixes its output with stale
	// captures from an earlier run against a different page list.
	if screenshotDir != "" {
		if err := os.RemoveAll(screenshotDir); err != nil {
			t.Fatalf("clear screenshot dir: %v", err)
		}
		if err := os.MkdirAll(screenshotDir, 0o755); err != nil {
			t.Fatalf("create screenshot dir: %v", err)
		}
	}

	for _, page := range pages {
		t.Run(page.description, func(t *testing.T) {
			pageURL := baseURL + page.path
			finalURL, status, body := fetchPage(t, client, pageURL)
			if status != http.StatusOK {
				t.Fatalf("GET %s status = %d, want 200", page.path, status)
			}
			assertRenderedPage(t, page.path, body)
			assertSeededListingHasRows(t, page.path, body)
			harvestLinks(body, discoveredLinks)
			if finalURL != pageURL {
				t.Logf("GET %s redirected to %s", page.path, finalURL)
			}

			if gotenbergURL == "" || screenshotDir == "" {
				return
			}
			fileName := strings.ReplaceAll(strings.Trim(page.path, "/"), "/", "-")
			if fileName == "" {
				fileName = "home"
			}
			outputFile := filepath.Join(screenshotDir, fileName+".png")
			if err := TakePageScreenshot(t, client, pageURL, gotenbergTargetURL+page.path, gotenbergURL, outputFile); err != nil {
				t.Fatalf("screenshot %s: %v", page.path, err)
			}
			assertPNG(t, outputFile)
		})
	}

	// Detail and edit views, reached with identifiers harvested above. These
	// carry the row-level templates that listing pages never exercise when a
	// table happens to be empty.
	//
	// Each round harvests links from the pages it visits, so detail pages feed
	// the next round with the edit URLs that only they link to. The loop ends
	// when a round can resolve nothing further.
	remaining := detailRoutes
	for round := 1; len(remaining) > 0; round++ {
		batch, unresolved := resolveDetailPages(remaining, discoveredLinks, reservedPaths)
		if len(batch) == 0 {
			t.Logf("no link found for %d parameterised route(s), not covered: %s",
				len(unresolved), strings.Join(unresolved, ", "))
			break
		}
		t.Logf("detail round %d: checking %d parameterised route(s) from %d discovered links",
			round, len(batch), len(discoveredLinks))
		for _, page := range batch {
			t.Run(page.description, func(t *testing.T) {
				pageURL := baseURL + page.path
				_, status, body := fetchPage(t, client, pageURL)
				if status != http.StatusOK {
					t.Fatalf("GET %s (as %s) status = %d, want 200", page.path, page.description, status)
				}
				assertRenderedPage(t, page.path, body)
				harvestLinks(body, discoveredLinks)
			})
		}
		remaining = unresolved
	}

	// One real write, so the guard sweep below cannot pass against an
	// application that rejects every mutation for the wrong reason.
	t.Run("guarded mutation succeeds with a token", func(t *testing.T) {
		assertGuardedMutationSucceedsWithToken(t, client, baseURL)
	})

	// Export endpoints
	for _, path := range []string{"/accounting/pnl/export.xlsx", "/accounting/budget/export.xlsx"} {
		response = get(t, client, baseURL+path)
		payload, _ := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if response.StatusCode != http.StatusOK {
			t.Fatalf("GET %s status = %d, want 200", path, response.StatusCode)
		}
		if len(payload) < 4 || string(payload[:2]) != "PK" {
			t.Fatalf("GET %s did not return XLSX data", path)
		}
	}

	// Last: one of these routes is POST /auth/logout, so an unguarded mutation
	// would end the session. Running the sweep here keeps that from cascading
	// into unrelated failures above.
	if len(mutationRoutes) > 0 {
		t.Logf("checking %d mutating routes reject requests without a CSRF token", len(mutationRoutes))
		assertMutationsRequireCSRF(t, client, baseURL, mutationRoutes)

		// The guard should have kept the session intact throughout.
		if _, status, _ := fetchPage(t, client, baseURL+"/accounting/pnl"); status != http.StatusOK {
			t.Errorf("session no longer usable after the mutation sweep (status %d); "+
				"a mutating route ran when it should have been rejected", status)
		}
	}

	t.Run("bank-feed webhook uses provider boundary instead of browser CSRF", func(t *testing.T) {
		assertBankFeedWebhookContract(t, client, baseURL)
	})
}

// pageRoute is one HTML page the suite asserts on.
type pageRoute struct {
	path        string
	description string
}

// routeEntry mirrors app.RouteEntry as emitted by ODYSSEY_DUMP_ROUTES.
type routeEntry struct {
	Method  string `json:"method"`
	Pattern string `json:"pattern"`
}

// fallbackPages is used when no route dump is supplied, so the suite still
// runs standalone against an arbitrary instance.
var fallbackPages = []pageRoute{
	{"/", "Home/Dashboard"},
	{"/accounting/pnl", "Accounting P&L"},
	{"/accounting/balance-sheet", "Accounting Balance Sheet"},
	{"/accounting/cash-flow", "Accounting Cash Flow"},
	{"/accounting/trial-balance", "Accounting Trial Balance"},
	{"/accounting/gl", "Accounting General Ledger"},
	{"/accounting/budget", "Accounting Budget"},
	{"/finance/banking/accounts", "Banking Accounts"},
	{"/jobs", "Background Jobs"},
	{"/roles", "Roles"},
	{"/users", "Users"},
	{"/permissions", "Permissions"},
}

// nonPagePrefixes are routes that never render the authenticated app shell:
// public pages, portal pages, static assets, and operational endpoints.
var nonPagePrefixes = []string{
	"/static",
	"/auth",
	"/portal",
	"/welcome",
	"/metrics",
	"/health",
	"/healthz",
	"/readyz",
	"/debug",
	"/api",
}

// nonPageExactPaths are authenticated endpoints whose route names are rooted
// in the browser application but whose contract is JSON (or a separate
// terminal application). They are covered by their API/handler tests rather
// than the app-shell assertions below.
var nonPageExactPaths = map[string]struct{}{
	"/distribution/loads":              {},
	"/distribution/planning/horizons":  {},
	"/distribution/routes":             {},
	"/distribution/transfers":          {},
	"/finance/forecasting/runs/latest": {},
	"/mrp/boms/revisions":              {},
	"/mrp/decisions/audit":             {},
	"/mrp/decisions/form":              {},
	"/mrp/dispatch":                    {},
	"/mrp/genealogy":                   {},
	"/mrp/wip-locations":               {},
	"/permissions/access-reviews":      {},
	"/permissions/scoped-assignments":  {},
	"/procurement/contracts":           {},
	"/procurement/variances":           {},
	"/pos/terminal":                    {},
}

// nonPageSuffixes are GET routes that return a file or data payload rather
// than a page. Exports are covered separately by their own assertions.
var nonPageSuffixes = []string{
	".xlsx",
	".csv",
	".json",
	".pdf",
	".png",
	"/export",
	"/pdf",
	"/download",
	"/stream",
	"/health",
	"/ping",
}

// pagesUnderTest derives coverage from the router's own table when
// ODYSSEY_E2E_ROUTES points at a dump produced by ODYSSEY_DUMP_ROUTES.
// A hand-maintained list drifts silently as routes are added; the router
// cannot.
func pagesUnderTest(t *testing.T) (pages []pageRoute, details []string, reserved map[string]struct{}, mutations []routeEntry) {
	t.Helper()
	reserved = make(map[string]struct{})
	routeFile := os.Getenv("ODYSSEY_E2E_ROUTES")
	if routeFile == "" {
		t.Log("ODYSSEY_E2E_ROUTES not set, using built-in page list")
		return fallbackPages, nil, reserved, nil
	}

	raw, err := os.ReadFile(routeFile)
	if err != nil {
		// go test runs from the package directory, so a relative path here
		// resolves under tests/e2e rather than the caller's directory.
		wd, _ := os.Getwd()
		t.Fatalf("read route dump %q (working directory %s; use an absolute path): %v",
			routeFile, wd, err)
	}
	var entries []routeEntry
	if err := json.Unmarshal(raw, &entries); err != nil {
		t.Fatalf("parse route dump %s: %v", routeFile, err)
	}

	pages = make([]pageRoute, 0, len(entries))
	skipped := 0
	for _, entry := range entries {
		if entry.Method != http.MethodGet {
			continue
		}
		// Every concrete GET path the router serves is reserved, so it can
		// never be mistaken for an identifier when resolving detail routes.
		if !strings.ContainsAny(entry.Pattern, "{*") {
			reserved[entry.Pattern] = struct{}{}
		}
		if !isPageRoute(entry.Pattern) {
			skipped++
			continue
		}
		pages = append(pages, pageRoute{path: entry.Pattern, description: entry.Pattern})
	}
	if len(pages) == 0 {
		t.Fatalf("route dump %s yielded no page routes", routeFile)
	}
	details = detailPatterns(entries)
	t.Logf("derived %d page routes, %d parameterised routes and %d mutating routes from %s (%d GET routes excluded)",
		len(pages), len(details), len(mutatingRoutes(entries)), routeFile, skipped)
	mutations = mutatingRoutes(entries)
	return pages, details, reserved, mutations
}

// isPageRoute reports whether a route pattern is a directly fetchable HTML
// page. Parameterised routes are excluded here because they need real
// identifiers; they are covered separately via detailPatterns.
func isPageRoute(pattern string) bool {
	if strings.ContainsAny(pattern, "{*") {
		return false
	}
	return servesPage(pattern)
}

// servesPage reports whether a path renders the authenticated app shell,
// ignoring whether it carries route parameters.
func servesPage(pattern string) bool {
	if pattern == "" || !strings.HasPrefix(pattern, "/") {
		return false
	}
	if pattern == "/" {
		return true
	}
	if _, ok := nonPageExactPaths[pattern]; ok {
		return false
	}
	for _, prefix := range nonPagePrefixes {
		if pattern == prefix || strings.HasPrefix(pattern, prefix+"/") {
			return false
		}
	}
	for _, suffix := range nonPageSuffixes {
		if strings.HasSuffix(pattern, suffix) {
			return false
		}
	}
	return true
}

// detailPatterns returns the parameterised page routes from a route dump, such
// as /masterdata/units/{id}. They are matched against links harvested from the
// list pages so that detail and edit views are exercised with identifiers that
// genuinely exist, without coupling the suite to seed internals.
func detailPatterns(entries []routeEntry) []string {
	patterns := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.Method != http.MethodGet {
			continue
		}
		if !strings.Contains(entry.Pattern, "{") || strings.Contains(entry.Pattern, "*") {
			continue
		}
		if !servesPage(entry.Pattern) {
			continue
		}
		patterns = append(patterns, entry.Pattern)
	}
	return patterns
}

// patternMatcher turns a chi route pattern into a matcher for concrete paths,
// where each {param} stands for exactly one path segment.
func patternMatcher(pattern string) *regexp.Regexp {
	segments := strings.Split(pattern, "/")
	for i, segment := range segments {
		if strings.HasPrefix(segment, "{") && strings.HasSuffix(segment, "}") {
			segments[i] = `[^/]+`
		} else {
			segments[i] = regexp.QuoteMeta(segment)
		}
	}
	return regexp.MustCompile("^" + strings.Join(segments, "/") + "$")
}

// linkAttr captures in-app paths from anchors and the data attributes the
// listing tables use for row navigation. Any query string or fragment is
// dropped so that one canonical path per target is collected.
var linkAttr = regexp.MustCompile(`(?:href|data-href|data-edit-href)="(/[^"#?]*)[^"]*"`)

// harvestLinks collects in-app paths from a rendered page.
func harvestLinks(body string, into map[string]struct{}) {
	for _, match := range linkAttr.FindAllStringSubmatch(body, -1) {
		into[match[1]] = struct{}{}
	}
}

// resolveDetailPages pairs each parameterised route with a concrete path drawn
// from the links harvested so far, and returns the patterns still without one.
// Callers re-run it as more links come in, since a route such as
// /sales/orders/{id}/edit is only ever linked from its own detail page.
//
// reserved holds the concrete route paths the router already serves in their
// own right. A parameter matches any single segment, so without this
// /masterdata/units/{id} would happily resolve to /masterdata/units/new and
// report coverage for a detail page that was never fetched.
func resolveDetailPages(patterns []string, links map[string]struct{}, reserved map[string]struct{}) (pages []pageRoute, unresolved []string) {
	ordered := make([]string, 0, len(links))
	for link := range links {
		if _, isReserved := reserved[link]; isReserved {
			continue
		}
		if hasZeroPathSegment(link) {
			continue
		}
		ordered = append(ordered, link)
	}
	sort.Strings(ordered)

	pages = make([]pageRoute, 0, len(patterns))
	for _, pattern := range patterns {
		matcher := patternMatcher(pattern)
		matched := ""
		for _, link := range ordered {
			if matcher.MatchString(link) {
				matched = link
				break
			}
		}
		if matched == "" {
			unresolved = append(unresolved, pattern)
			continue
		}
		pages = append(pages, pageRoute{path: matched, description: pattern})
	}
	return pages, unresolved
}

func hasZeroPathSegment(path string) bool {
	for _, segment := range strings.Split(strings.Trim(path, "/"), "/") {
		if segment == "0" {
			return true
		}
	}
	return false
}

// fetchPage retrieves a page, following in-app redirects. The shared client
// deliberately does not auto-follow, because the login assertions depend on
// seeing the 303 itself; alias routes such as /consol -> /finance/consol still
// need their destination checked, so redirects are resolved here instead.
func fetchPage(t *testing.T, client *http.Client, pageURL string) (finalURL string, status int, body string) {
	t.Helper()
	const maxHops = 5
	current := pageURL
	for hop := 0; ; hop++ {
		response := get(t, client, current)
		payload, err := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if err != nil {
			t.Fatalf("GET %s: read body: %v", current, err)
		}
		if response.StatusCode < 300 || response.StatusCode > 399 {
			return current, response.StatusCode, string(payload)
		}
		if hop == maxHops {
			t.Fatalf("GET %s: exceeded %d redirects", pageURL, maxHops)
		}
		location := response.Header.Get("Location")
		if location == "" {
			t.Fatalf("GET %s: status %d without a Location header", current, response.StatusCode)
		}
		base, err := url.Parse(current)
		if err != nil {
			t.Fatalf("parse %s: %v", current, err)
		}
		target, err := base.Parse(location)
		if err != nil {
			t.Fatalf("resolve redirect %q from %s: %v", location, current, err)
		}
		current = target.String()
	}
}

// seededListings are pages the seed guarantees at least one row for. If one
// comes back empty the sweep silently loses coverage of the matching detail
// route, so an empty listing is reported as a failure rather than passing as a
// well-rendered but vacant page.
var seededListings = map[string]string{
	"/accounting/banks/statements": "/accounting/banks/statements/",
	"/board-packs":                 "/board-packs/",
	"/eliminations/runs":           "/eliminations/runs/",
	"/finance/ar/invoices":         "/finance/ar/invoices/",
	"/finance/banking/accounts":    "/finance/banking/accounts/",
	// The payments listing has no detail route of its own; each row links to
	// the invoice it settles.
	"/finance/ar/payments":   "/finance/ar/invoices/",
	"/inventory/adjustments": "/inventory/adjustments/",
	"/inventory/stock-takes": "/inventory/stock-takes/",
	"/variance/snapshots":    "/variance/snapshots/",
}

// assertWorkspaceAPI checks the endpoint behind the shell's company switcher
// and user menu.
//
// Those widgets are populated by JavaScript from /api/me, so this HTML-level
// suite cannot see them render. It can however verify the contract they depend
// on: if this endpoint breaks, main.js swallows the error and every
// authenticated page silently falls back to "Memuat perusahaan..." and
// "Pengguna" with nothing failing.
func assertWorkspaceAPI(t *testing.T, client *http.Client, baseURL string) {
	t.Helper()
	response := get(t, client, baseURL+"/api/me")
	payload, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatalf("GET /api/me: read body: %v", err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/me status = %d, want 200", response.StatusCode)
	}

	var workspace struct {
		User struct {
			ID    int64  `json:"id"`
			Email string `json:"email"`
		} `json:"user"`
		Companies []struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
		} `json:"companies"`
	}
	if err := json.Unmarshal(payload, &workspace); err != nil {
		t.Fatalf("GET /api/me returned invalid JSON: %v", err)
	}
	if workspace.User.ID == 0 || workspace.User.Email == "" {
		t.Errorf("GET /api/me returned no identifiable user; the user menu would show a placeholder")
	}
	if len(workspace.Companies) == 0 {
		t.Errorf("GET /api/me returned no companies; the company switcher would stay disabled")
	}
	for _, company := range workspace.Companies {
		if company.ID == 0 || company.Name == "" {
			t.Errorf("GET /api/me returned an unusable company entry %+v", company)
		}
	}
}

// assertSeededListingHasRows checks that a listing the seed populates actually
// rendered at least one row link.
func assertSeededListingHasRows(t *testing.T, path, body string) {
	t.Helper()
	prefix, ok := seededListings[path]
	if !ok {
		return
	}
	rowLink := regexp.MustCompile(regexp.QuoteMeta(prefix) + `\d+`)
	if !rowLink.MatchString(body) {
		t.Errorf("GET %s rendered no rows, but the seed guarantees at least one; "+
			"its detail route cannot be covered", path)
	}
}

// mutatingRoutes returns the routes that can change state: everything the CSRF
// middleware guards, which is every method except GET, HEAD and OPTIONS. The
// static file handler is registered for all methods and is not a mutation, so
// it is excluded along with any wildcard pattern.
func mutatingRoutes(entries []routeEntry) []routeEntry {
	routes := make([]routeEntry, 0, len(entries))
	for _, entry := range entries {
		switch entry.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			continue
		}
		if strings.Contains(entry.Pattern, "*") || strings.HasPrefix(entry.Pattern, "/static") {
			continue
		}
		if isBankFeedWebhookRoute(entry.Pattern) {
			continue
		}
		routes = append(routes, entry)
	}
	return routes
}

func isBankFeedWebhookRoute(pattern string) bool {
	return strings.HasPrefix(pattern, "/finance/bankfeeds/webhooks/")
}

func assertBankFeedWebhookContract(t *testing.T, client *http.Client, baseURL string) {
	t.Helper()
	// Bounded release profiles may intentionally omit bank-feed routes. The
	// route manifest is the source of truth for the running profile; do not
	// turn an expected profile 404 into a false provider-boundary failure.
	if routeFile := strings.TrimSpace(os.Getenv("ODYSSEY_E2E_ROUTES")); routeFile != "" {
		raw, err := os.ReadFile(routeFile)
		if err != nil {
			t.Fatalf("read route dump %q for bank-feed contract: %v", routeFile, err)
		}
		var entries []routeEntry
		if err := json.Unmarshal(raw, &entries); err != nil {
			t.Fatalf("parse route dump %s for bank-feed contract: %v", routeFile, err)
		}
		admitted := false
		for _, entry := range entries {
			if entry.Method == http.MethodPost && entry.Pattern == "/finance/bankfeeds/webhooks/{provider}" {
				admitted = true
				break
			}
		}
		if !admitted {
			t.Skip("bank-feed webhook is outside the selected release profile")
		}
	}
	request, err := http.NewRequest(http.MethodPost, baseURL+"/finance/bankfeeds/webhooks/stripe", strings.NewReader(`{"type":"e2e.probe"}`))
	if err != nil {
		t.Fatalf("build bank-feed webhook request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("bank-feed webhook request: %v", err)
	}
	_, _ = io.Copy(io.Discard, response.Body)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusBadRequest {
		t.Errorf("bank-feed webhook without a connection id returned %d, want 400 from provider validation", response.StatusCode)
	}
}

// unreachableID stands in for route parameters. It is deliberately an
// identifier no record has, so that if a route turns out not to be guarded the
// request lands on nothing rather than mutating or deleting real data.
const unreachableID = "999999999"

// concreteMutationPath substitutes route parameters with an identifier that
// matches no record.
func concreteMutationPath(pattern string) string {
	segments := strings.Split(pattern, "/")
	for i, segment := range segments {
		if strings.HasPrefix(segment, "{") && strings.HasSuffix(segment, "}") {
			segments[i] = unreachableID
		}
	}
	return strings.Join(segments, "/")
}

// assertMutationsRequireCSRF checks that every state-changing route rejects a
// request without a CSRF token.
//
// This is the whole non-GET surface, and it is checked uniformly because the
// guard is a middleware invariant rather than per-handler logic: a route
// mounted outside the protected chain would silently accept forged
// cross-site writes. The requests carry no token, so a guarded route never
// reaches its handler and nothing is written.
//
// It deliberately says nothing about whether these routes work - only that
// they cannot be driven without a token. Exercising their behaviour needs
// per-route fixtures.
func assertMutationsRequireCSRF(t *testing.T, client *http.Client, baseURL string, routes []routeEntry) {
	t.Helper()
	for _, route := range routes {
		name := route.Method + " " + route.Pattern
		t.Run(name, func(t *testing.T) {
			target := baseURL + concreteMutationPath(route.Pattern)
			pace()
			request, err := http.NewRequest(route.Method, target,
				strings.NewReader("probe=csrf-guard"))
			if err != nil {
				t.Fatalf("build request for %s: %v", name, err)
			}
			request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

			response, err := client.Do(request)
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			_, _ = io.Copy(io.Discard, response.Body)
			_ = response.Body.Close()

			if response.StatusCode != http.StatusForbidden {
				t.Errorf("%s without a CSRF token returned %d, want 403; "+
					"an unguarded mutation accepts cross-site writes",
					name, response.StatusCode)
			}
		})
	}
}

// assertGuardedMutationSucceedsWithToken drives one real write end to end.
//
// Without it the guard sweep above would pass just as happily against an
// application that rejected every mutation for the wrong reason.
func assertGuardedMutationSucceedsWithToken(t *testing.T, client *http.Client, baseURL string) {
	t.Helper()
	const listPath = "/masterdata/units"

	token := fetchCSRF(t, client, baseURL+listPath+"/new")
	code := "E2E-" + strconv.FormatInt(time.Now().UnixNano()%1_000_000, 10)

	response := postForm(t, client, baseURL+listPath, url.Values{
		"code":       {code},
		"name":       {"E2E Probe Unit"},
		"csrf_token": {token},
	})
	_, _ = io.Copy(io.Discard, response.Body)
	_ = response.Body.Close()
	if response.StatusCode >= 400 {
		t.Fatalf("POST %s with a valid token returned %d, want a success or redirect",
			listPath, response.StatusCode)
	}

	_, status, body := fetchPage(t, client, baseURL+listPath)
	if status != http.StatusOK {
		t.Fatalf("GET %s after create status = %d, want 200", listPath, status)
	}
	if !strings.Contains(body, code) {
		t.Errorf("unit %s was accepted but does not appear in %s", code, listPath)
	}
}

// serverErrorMarkers are strings the accounting handlers emit through
// http.Error. Before Render buffered its output they could be appended to an
// already-committed 200 response, so a status check alone never saw them.
var serverErrorMarkers = []string{
	"Gagal memuat data laporan",
	"Internal Server Error",
}

// assertRenderedPage checks that an authenticated page actually rendered the
// application shell to completion, rather than the public landing page or HTML
// truncated by a mid-render template failure.
func assertRenderedPage(t *testing.T, path, body string) {
	t.Helper()
	if strings.Contains(body, `class="public-page"`) {
		t.Errorf("GET %s rendered the public landing page, want the authenticated app shell", path)
		return
	}
	if !strings.Contains(body, `class="app-shell"`) {
		t.Errorf("GET %s did not render the app shell", path)
		return
	}
	for _, marker := range serverErrorMarkers {
		if strings.Contains(body, marker) {
			t.Errorf("GET %s body contains server error %q", path, marker)
		}
	}
	if !strings.Contains(body, "</html>") {
		t.Errorf("GET %s body is truncated: no closing </html>", path)
	}
	assertCSRFFieldsArePopulated(t, path, body)
}

// emptyCSRFField matches a rendered token field with no value, whatever the
// attribute order.
var emptyCSRFField = regexp.MustCompile(`name="csrf_token"[^>]*value=""|value=""[^>]*name="csrf_token"`)

// assertCSRFFieldsArePopulated checks that a page carrying a CSRF field also
// carries a token.
//
// A handler that renders a form without putting CSRFToken in its TemplateData
// produces value="", and every submit of that form is then rejected by the
// CSRF middleware. The page itself still looks perfectly healthy, so nothing
// else here would notice.
func assertCSRFFieldsArePopulated(t *testing.T, path, body string) {
	t.Helper()
	if emptyCSRFField.MatchString(body) {
		t.Errorf("GET %s renders a csrf_token field with an empty value; "+
			"submitting that form is rejected as a CSRF failure", path)
	}
}

// assertPNG verifies a capture is a real, non-empty PNG rather than an error
// payload that happened to be written to disk.
func assertPNG(t *testing.T, path string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read screenshot %s: %v", path, err)
	}
	if len(data) < 8 || !bytes.Equal(data[:8], []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}) {
		t.Fatalf("screenshot %s is not a PNG", path)
	}
	if len(data) < 1024 {
		t.Fatalf("screenshot %s is only %d bytes, likely blank", path, len(data))
	}
}

func fetchCSRF(t *testing.T, client *http.Client, endpoint string) string {
	t.Helper()
	response := get(t, client, endpoint)
	body, _ := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET login status = %d", response.StatusCode)
	}
	match := regexp.MustCompile(`name="csrf_token" value="([^"]+)"`).FindSubmatch(body)
	if len(match) != 2 {
		t.Fatal("CSRF token not found")
	}
	return string(match[1])
}

// The app rate-limits every non-static route to 60 requests per minute per IP
// (conditionalRateLimiter in internal/app/middleware.go). A router-derived
// sweep is much larger than that, so requests are paced to stay inside the
// window. Without pacing the tail of the sweep returns 429s that are
// indistinguishable from genuinely broken pages - which is why the original
// hand-written list carried "rate limited" comments next to working routes.
var (
	pacerOnce sync.Once
	pacerGap  time.Duration
	pacerMu   sync.Mutex
	pacerLast time.Time
)

func pace() {
	pacerOnce.Do(func() {
		perMinute := 45 // margin below the server's 60/min
		if raw := os.Getenv("ODYSSEY_E2E_RATE_PER_MIN"); raw != "" {
			if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
				perMinute = parsed
			}
		}
		pacerGap = time.Minute / time.Duration(perMinute)
	})

	pacerMu.Lock()
	defer pacerMu.Unlock()
	if wait := time.Until(pacerLast.Add(pacerGap)); wait > 0 {
		time.Sleep(wait)
	}
	pacerLast = time.Now()
}

// retryAfter reports how long to wait before retrying a throttled request.
func retryAfter(response *http.Response) time.Duration {
	if raw := response.Header.Get("Retry-After"); raw != "" {
		if seconds, err := strconv.Atoi(raw); err == nil && seconds > 0 {
			return time.Duration(seconds) * time.Second
		}
	}
	return time.Minute
}

func get(t *testing.T, client *http.Client, endpoint string) *http.Response {
	t.Helper()
	pace()
	response, err := client.Get(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	// Absorb a throttle once rather than reporting it as a page failure.
	if response.StatusCode == http.StatusTooManyRequests {
		wait := retryAfter(response)
		_ = response.Body.Close()
		t.Logf("rate limited on %s, retrying in %s", endpoint, wait)
		time.Sleep(wait)
		pace()
		response, err = client.Get(endpoint)
		if err != nil {
			t.Fatal(err)
		}
	}
	return response
}
func postForm(t *testing.T, client *http.Client, endpoint string, values url.Values) *http.Response {
	t.Helper()
	pace()
	response, err := client.PostForm(endpoint, values)
	if err != nil {
		t.Fatal(err)
	}
	return response
}
func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

// TakePageScreenshot fetches a screenshot of an authenticated page via Gotenberg
// by passing the session cookies to Gotenberg's URL-based screenshot endpoint.
//
// cookieURL is the page on the host the test client authenticated against;
// pageURL is the same page as Gotenberg must reach it, which differs whenever
// Gotenberg runs in its own container. The two are kept separate because the
// cookie jar is keyed by host: looking cookies up under Gotenberg's hostname
// silently returns none and captures a logged-out page.
func TakePageScreenshot(t *testing.T, client *http.Client, cookieURL, pageURL, gotenbergURL, outputFilePath string) error {
	t.Helper()

	// Use multipart form to send URL and cookies
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	_ = writer.WriteField("url", pageURL)
	_ = writer.WriteField("format", "png")
	_ = writer.WriteField("width", "1920")
	_ = writer.WriteField("height", "1080")

	// Extract and pass cookies from the client's jar
	if client.Jar != nil {
		cookieReq, err := http.NewRequest(http.MethodGet, cookieURL, nil)
		if err != nil {
			return fmt.Errorf("parse cookie URL: %w", err)
		}
		pageReq, err := http.NewRequest(http.MethodGet, pageURL, nil)
		if err != nil {
			return fmt.Errorf("parse page URL: %w", err)
		}
		cookies := client.Jar.Cookies(cookieReq.URL)
		if len(cookies) == 0 {
			return fmt.Errorf("no session cookies for %s; screenshot would capture a logged-out page", cookieURL)
		}
		{
			// Gotenberg expects cookies with specific fields; omit SameSite which uses Go's int enum
			type gotenbergCookie struct {
				Name     string `json:"name"`
				Value    string `json:"value"`
				Domain   string `json:"domain,omitempty"`
				Path     string `json:"path,omitempty"`
				Expires  string `json:"expires,omitempty"`
				HTTPOnly bool   `json:"httpOnly,omitempty"`
				Secure   bool   `json:"secure,omitempty"`
			}
			// Always scope cookies to the host Gotenberg fetches. A domain
			// carried over from the test client's host would not match, and
			// Chromium would drop the cookie and render a logged-out page.
			cookieDomain := pageReq.URL.Hostname()
			gCookies := make([]gotenbergCookie, len(cookies))
			for i, c := range cookies {
				var expires string
				if !c.Expires.IsZero() {
					expires = c.Expires.Format(time.RFC3339)
				}
				gCookies[i] = gotenbergCookie{
					Name:     c.Name,
					Value:    c.Value,
					Domain:   cookieDomain,
					Path:     c.Path,
					Expires:  expires,
					HTTPOnly: c.HttpOnly,
					Secure:   c.Secure,
				}
			}
			cookieJSON, _ := json.Marshal(gCookies)
			_ = writer.WriteField("cookies", string(cookieJSON))
		}
	}

	if err := writer.Close(); err != nil {
		return fmt.Errorf("close writer: %w", err)
	}

	endpoint := strings.TrimRight(gotenbergURL, "/") + "/forms/chromium/screenshot/url"
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, endpoint, body)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	httpClient := &http.Client{Timeout: 60 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("gotenberg request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("gotenberg returned status %d: %s", resp.StatusCode, string(respBody))
	}

	if err := os.MkdirAll(filepath.Dir(outputFilePath), 0755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}

	outFile, err := os.Create(outputFilePath)
	if err != nil {
		return fmt.Errorf("create output file: %w", err)
	}
	defer func() { _ = outFile.Close() }()

	if _, err := io.Copy(outFile, resp.Body); err != nil {
		return fmt.Errorf("save screenshot: %w", err)
	}

	t.Logf("Saved screenshot to %s", outputFilePath)
	return nil
}
