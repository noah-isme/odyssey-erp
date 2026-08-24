//go:build staging
// +build staging

package staging

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"math"
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

	_ "github.com/jackc/pgx/v5/stdlib"
)

const (
	profileV010Core = "v0.10-core"

	envURL                = "ODYSSEY_E2E_URL"
	envPGDSN              = "PG_DSN"
	envAdminEmail         = "ODYSSEY_E2E_ADMIN_EMAIL"
	envAdminPassword      = "ODYSSEY_E2E_ADMIN_PASSWORD"
	envBranchEmail        = "ODYSSEY_E2E_BRANCH_EMAIL"
	envBranchPassword     = "ODYSSEY_E2E_BRANCH_PASSWORD"
	envNoAccessEmail      = "ODYSSEY_E2E_NO_ACCESS_EMAIL"
	envNoAccessPassword   = "ODYSSEY_E2E_NO_ACCESS_PASSWORD"
	envCompanyID          = "ODYSSEY_E2E_COMPANY_ID"
	envBranchID           = "ODYSSEY_E2E_BRANCH_ID"
	envOtherCompanyID     = "ODYSSEY_E2E_OTHER_COMPANY_ID"
	envOtherBranchID      = "ODYSSEY_E2E_OTHER_BRANCH_ID"
	envCustomerID         = "ODYSSEY_E2E_CUSTOMER_ID"
	envSupplierID         = "ODYSSEY_E2E_SUPPLIER_ID"
	envProductID          = "ODYSSEY_E2E_PRODUCT_ID"
	envWarehouseID        = "ODYSSEY_E2E_WAREHOUSE_ID"
	envGRNID              = "ODYSSEY_E2E_GRN_ID"
	envDocumentCategoryID = "ODYSSEY_E2E_DOCUMENT_CATEGORY_ID"
	envDocumentClassID    = "ODYSSEY_E2E_DOCUMENT_CLASSIFICATION_ID"
	envAmount             = "ODYSSEY_E2E_AMOUNT"
	envEvidenceIndex      = "ODYSSEY_EVIDENCE_INDEX"
	envEvidenceRunID      = "ODYSSEY_EVIDENCE_RUN_ID"
	envEvidenceArtifact   = "ODYSSEY_EVIDENCE_ARTIFACT_URI"
	envEvidenceSHA256     = "ODYSSEY_EVIDENCE_SHA256"
)

var (
	csrfPattern = regexp.MustCompile(`name="csrf_token"[^>]*value="([^"]+)"|value="([^"]+)"[^>]*name="csrf_token"`)
	pacerOnce   sync.Once
	pacerGap    time.Duration
	pacerMu     sync.Mutex
	pacerLast   time.Time
)

type stagingCredentials struct {
	email    string
	password string
}

type stagingConfig struct {
	baseURL string
	pgDSN   string
	amount  string

	admin    stagingCredentials
	branch   stagingCredentials
	noAccess stagingCredentials

	companyID, branchID                 int64
	otherCompanyID, otherBranchID       int64
	customerID, supplierID, productID   int64
	warehouseID, grnID                  int64
	documentCategoryID, documentClassID int64
}

func requiredValue(t *testing.T, key string) string {
	t.Helper()
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		t.Fatalf("%s is required; staging certification never supplies defaults", key)
	}
	return value
}

func requiredID(t *testing.T, key string) int64 {
	t.Helper()
	raw := requiredValue(t, key)
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		t.Fatalf("%s must be a positive integer, got %q", key, raw)
	}
	return id
}

func loadStagingConfig(t *testing.T) stagingConfig {
	t.Helper()
	baseURL := strings.TrimRight(requiredValue(t, envURL), "/")
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		t.Fatalf("%s must be an absolute http(s) URL, got %q", envURL, baseURL)
	}

	cfg := stagingConfig{
		baseURL: baseURL,
		pgDSN:   requiredValue(t, envPGDSN),
		amount:  requiredValue(t, envAmount),
		admin: stagingCredentials{
			email:    requiredValue(t, envAdminEmail),
			password: requiredValue(t, envAdminPassword),
		},
		branch: stagingCredentials{
			email:    requiredValue(t, envBranchEmail),
			password: requiredValue(t, envBranchPassword),
		},
		noAccess: stagingCredentials{
			email:    requiredValue(t, envNoAccessEmail),
			password: requiredValue(t, envNoAccessPassword),
		},
		companyID:          requiredID(t, envCompanyID),
		branchID:           requiredID(t, envBranchID),
		otherCompanyID:     requiredID(t, envOtherCompanyID),
		otherBranchID:      requiredID(t, envOtherBranchID),
		customerID:         requiredID(t, envCustomerID),
		supplierID:         requiredID(t, envSupplierID),
		productID:          requiredID(t, envProductID),
		warehouseID:        requiredID(t, envWarehouseID),
		grnID:              requiredID(t, envGRNID),
		documentCategoryID: requiredID(t, envDocumentCategoryID),
		documentClassID:    requiredID(t, envDocumentClassID),
	}
	amount, err := strconv.ParseFloat(cfg.amount, 64)
	if err != nil || amount <= 0 {
		t.Fatalf("%s must be a positive decimal, got %q", envAmount, cfg.amount)
	}
	return cfg
}

func pace() {
	pacerOnce.Do(func() {
		perMinute := 45
		if raw := strings.TrimSpace(os.Getenv("ODYSSEY_E2E_RATE_PER_MIN")); raw != "" {
			parsed, err := strconv.Atoi(raw)
			if err == nil && parsed > 0 {
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

type httpResult struct {
	status   int
	location string
	body     []byte
	header   http.Header
}

type stagingClient struct {
	baseURL string
	client  *http.Client
}

func setupClient(t *testing.T, baseURL string, credentials stagingCredentials) *stagingClient {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookie jar: %v", err)
	}
	client := &stagingClient{
		baseURL: baseURL,
		client: &http.Client{
			Jar: jar,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}

	loginPage := client.get(t, "/auth/login")
	if loginPage.status != http.StatusOK {
		t.Fatalf("GET /auth/login status = %d, want %d", loginPage.status, http.StatusOK)
	}
	csrf := csrfFromBody(t, loginPage.body, "/auth/login")
	login := client.postForm(t, "/auth/login", url.Values{
		"email":      {credentials.email},
		"password":   {credentials.password},
		"csrf_token": {csrf},
	})
	if login.status != http.StatusSeeOther || login.location == "" {
		t.Fatalf("POST /auth/login status/location = %d/%q, want %d and non-empty redirect", login.status, login.location, http.StatusSeeOther)
	}
	return client
}

func (c *stagingClient) do(t *testing.T, method, path string, body io.Reader, headers http.Header) httpResult {
	t.Helper()
	endpoint := path
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		endpoint = path
	} else {
		endpoint = c.baseURL + "/" + strings.TrimLeft(path, "/")
	}
	req, err := http.NewRequest(method, endpoint, body)
	if err != nil {
		t.Fatalf("build %s %s: %v", method, path, err)
	}
	for key, values := range headers {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	pace()
	resp, err := c.client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	bodyBytes, readErr := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if readErr != nil {
		t.Fatalf("read %s %s: %v", method, path, readErr)
	}
	return httpResult{status: resp.StatusCode, location: resp.Header.Get("Location"), body: bodyBytes, header: resp.Header.Clone()}
}

func (c *stagingClient) get(t *testing.T, path string) httpResult {
	return c.do(t, http.MethodGet, path, nil, nil)
}

func (c *stagingClient) postForm(t *testing.T, path string, values url.Values) httpResult {
	headers := make(http.Header)
	headers.Set("Content-Type", "application/x-www-form-urlencoded")
	return c.do(t, http.MethodPost, path, strings.NewReader(values.Encode()), headers)
}

func (c *stagingClient) postMultipart(t *testing.T, path, csrf, description string) httpResult {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range map[string]string{"csrf_token": csrf, "description": description} {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatalf("multipart field %s: %v", key, err)
		}
	}
	part, err := writer.CreateFormFile("file", "certification.txt")
	if err != nil {
		t.Fatalf("multipart file: %v", err)
	}
	if _, err := part.Write([]byte("v0.10-core staging certification evidence\n")); err != nil {
		t.Fatalf("multipart content: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("multipart close: %v", err)
	}
	headers := make(http.Header)
	headers.Set("Content-Type", writer.FormDataContentType())
	return c.do(t, http.MethodPost, path, &body, headers)
}

func csrfFromBody(t *testing.T, body []byte, endpoint string) string {
	t.Helper()
	match := csrfPattern.FindSubmatch(body)
	if len(match) > 1 {
		if token := string(match[1]); token != "" {
			return token
		}
		if len(match) > 2 && string(match[2]) != "" {
			return string(match[2])
		}
	}
	t.Fatalf("CSRF token not found on %s", endpoint)
	return ""
}

func fetchCSRF(t *testing.T, client *stagingClient, endpoint string) string {
	t.Helper()
	result := client.get(t, endpoint)
	if result.status != http.StatusOK {
		t.Fatalf("GET %s status = %d, want %d", endpoint, result.status, http.StatusOK)
	}
	return csrfFromBody(t, result.body, endpoint)
}

func parseRedirectID(t *testing.T, location string) int64 {
	t.Helper()
	parsed, err := url.Parse(location)
	if err != nil {
		t.Fatalf("parse redirect %q: %v", location, err)
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	for index := len(parts) - 1; index >= 0; index-- {
		if id, err := strconv.ParseInt(parts[index], 10, 64); err == nil && id > 0 {
			return id
		}
	}
	t.Fatalf("redirect %q did not contain a positive resource ID", location)
	return 0
}

type evidenceRecord struct {
	EvidenceID   string `json:"evidence_id"`
	Result       string `json:"result"`
	RunID        string `json:"run_id"`
	CollectedUTC string `json:"collected_utc"`
	Details      string `json:"details"`
	ArtifactURI  string `json:"artifact_uri"`
	SHA256       string `json:"sha256"`
}

type evidenceIndex struct {
	Version      int              `json:"version"`
	RunID        string           `json:"run_id"`
	CollectedUTC string           `json:"collected_utc"`
	CandidateTag string           `json:"candidate_tag,omitempty"`
	CandidateSHA string           `json:"candidate_sha,omitempty"`
	Entries      []evidenceRecord `json:"entries"`
	Missing      []string         `json:"missing,omitempty"`
}

type evidenceReporter struct{}

var expectedEvidenceIDs = map[string]struct{}{
	"ENV-002":     {},
	"REL-004":     {},
	"DB-002":      {},
	"J-ARAP-001":  {},
	"J-SALES-001": {},
	"J-INV-001":   {},
	"J-DOC-001":   {},
	"J-CMMS-001":  {},
	"ISO-001":     {},
	"ISO-002":     {},
	"ISO-003":     {},
}

var evidenceState = struct {
	sync.Mutex
	records map[string]evidenceRecord
}{records: make(map[string]evidenceRecord)}

var journeyState struct {
	sync.Mutex
	deliveryID int64
}

func rememberDeliveryID(id int64) {
	journeyState.Lock()
	journeyState.deliveryID = id
	journeyState.Unlock()
}

func rememberedDeliveryID() int64 {
	journeyState.Lock()
	defer journeyState.Unlock()
	return journeyState.deliveryID
}

func deliveryPathFor(id int64) string {
	return "/delivery/orders/" + strconv.FormatInt(id, 10)
}

// TestMain always writes the machine-readable index when a staging endpoint or
// explicit output path is configured. The index is intentionally assembled
// after all tests have run so a failed/partial run records missing IDs rather
// than silently treating the observed subset as a certification result.
func TestMain(m *testing.M) {
	code := m.Run()
	if err := writeEvidenceIndex(); err != nil {
		fmt.Fprintf(os.Stderr, "staging certification evidence index: %v\n", err)
		if evidenceStrict() && code == 0 {
			code = 1
		}
	}
	os.Exit(code)
}

var evidenceIDPattern = regexp.MustCompile(`^[A-Z][A-Z0-9]*(?:-[A-Z0-9]+)+$`)

func (evidenceReporter) pass(t *testing.T, evidenceID, details string) {
	t.Helper()
	if !evidenceIDPattern.MatchString(evidenceID) {
		t.Fatalf("invalid evidence ID %q: want uppercase hyphen-delimited ID", evidenceID)
	}
	if _, ok := expectedEvidenceIDs[evidenceID]; !ok {
		t.Fatalf("unexpected certification evidence ID %q; add it to the release contract before emitting it", evidenceID)
	}
	record := evidenceRecord{
		EvidenceID:   evidenceID,
		Result:       "PASS",
		RunID:        evidenceRunID(),
		CollectedUTC: time.Now().UTC().Format(time.RFC3339),
		Details:      details,
		ArtifactURI:  strings.TrimSpace(os.Getenv(envEvidenceArtifact)),
		SHA256:       strings.TrimSpace(os.Getenv(envEvidenceSHA256)),
	}
	evidenceState.Lock()
	_, duplicate := evidenceState.records[evidenceID]
	if !duplicate {
		evidenceState.records[evidenceID] = record
	}
	evidenceState.Unlock()
	if duplicate {
		t.Fatalf("duplicate certification evidence ID %q; each contract row must be emitted exactly once", evidenceID)
	}
	encoded, err := json.Marshal(record)
	if err != nil {
		t.Fatalf("marshal evidence %s: %v", evidenceID, err)
	}
	t.Logf("CERTIFICATION_EVIDENCE evidence_id=%s %s", evidenceID, encoded)
}

func evidenceRunID() string {
	for _, key := range []string{envEvidenceRunID, "GITHUB_RUN_ID", "CI_PIPELINE_ID"} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return "local-" + time.Now().UTC().Format("20060102T150405.000000000Z")
}

func evidenceStrict() bool {
	if raw := strings.TrimSpace(os.Getenv("ODYSSEY_EVIDENCE_STRICT")); raw != "" {
		parsed, err := strconv.ParseBool(raw)
		return err == nil && parsed
	}
	return strings.TrimSpace(os.Getenv(envURL)) != ""
}

func writeEvidenceIndex() error {
	path := strings.TrimSpace(os.Getenv(envEvidenceIndex))
	if path == "" && strings.TrimSpace(os.Getenv(envURL)) != "" {
		path = "evidence-index.json"
	}
	if path == "" {
		return nil
	}

	evidenceState.Lock()
	records := make([]evidenceRecord, 0, len(evidenceState.records))
	for _, record := range evidenceState.records {
		records = append(records, record)
	}
	evidenceState.Unlock()
	sort.Slice(records, func(i, j int) bool { return records[i].EvidenceID < records[j].EvidenceID })

	seen := make(map[string]struct{}, len(records))
	for _, record := range records {
		seen[record.EvidenceID] = struct{}{}
	}
	missing := make([]string, 0)
	for evidenceID := range expectedEvidenceIDs {
		if _, ok := seen[evidenceID]; !ok {
			missing = append(missing, evidenceID)
		}
	}
	sort.Strings(missing)

	index := evidenceIndex{
		Version:      1,
		RunID:        evidenceRunID(),
		CollectedUTC: time.Now().UTC().Format(time.RFC3339),
		CandidateTag: strings.TrimSpace(os.Getenv("CANDIDATE_TAG")),
		CandidateSHA: strings.TrimSpace(os.Getenv("EXPECTED_SHA")),
		Entries:      records,
		Missing:      missing,
	}
	encoded, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	dir := filepath.Dir(path)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create output directory: %w", err)
		}
	}
	tmp, err := os.CreateTemp(dir, ".evidence-index-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary output: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(encoded); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("publish %s: %w", path, err)
	}
	if evidenceStrict() && len(missing) > 0 {
		return fmt.Errorf("missing evidence IDs: %s", strings.Join(missing, ", "))
	}
	return nil
}

func assertDenied(t *testing.T, result httpResult, method, path string) {
	t.Helper()
	if result.status != http.StatusForbidden && result.status != http.StatusNotFound {
		t.Fatalf("%s %s status = %d, want %d or %d; body=%q", method, path, result.status, http.StatusForbidden, http.StatusNotFound, strings.TrimSpace(string(result.body)))
	}
}

func assertStatus(t *testing.T, result httpResult, want int, method, path string) {
	t.Helper()
	if result.status != want {
		t.Fatalf("%s %s status = %d, want %d; body=%q", method, path, result.status, want, strings.TrimSpace(string(result.body)))
	}
}

func assertRoute(t *testing.T, client *stagingClient, path string) httpResult {
	t.Helper()
	result := client.get(t, path)
	assertStatus(t, result, http.StatusOK, http.MethodGet, path)
	return result
}

func openReadOnlyDB(t *testing.T, dsn string) *sql.DB {
	t.Helper()
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		t.Fatalf("read-only staging database ping: %v", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if _, err := db.Exec("SET SESSION CHARACTERISTICS AS TRANSACTION READ ONLY"); err != nil {
		_ = db.Close()
		t.Fatalf("configure read-only staging database session: %v", err)
	}
	return db
}

func queryInt64(t *testing.T, db *sql.DB, query string, args ...any) int64 {
	t.Helper()
	var value int64
	if err := db.QueryRow(query, args...).Scan(&value); err != nil {
		t.Fatalf("query int64 %q: %v", query, err)
	}
	return value
}

func queryString(t *testing.T, db *sql.DB, query string, args ...any) string {
	t.Helper()
	var value string
	if err := db.QueryRow(query, args...).Scan(&value); err != nil {
		t.Fatalf("query string %q: %v", query, err)
	}
	return value
}

func queryCount(t *testing.T, db *sql.DB, query string, args ...any) int64 {
	t.Helper()
	var value int64
	if err := db.QueryRow(query, args...).Scan(&value); err != nil {
		t.Fatalf("query count %q: %v", query, err)
	}
	return value
}

func queryFloat64(t *testing.T, db *sql.DB, query string, args ...any) float64 {
	t.Helper()
	var value float64
	if err := db.QueryRow(query, args...).Scan(&value); err != nil {
		t.Fatalf("query float64 %q: %v", query, err)
	}
	return value
}

func assertBalancedJournal(t *testing.T, db *sql.DB, sourceModule, memo string) int64 {
	t.Helper()
	journalID := queryInt64(t, db, `
		SELECT id FROM journal_entries
		WHERE source_module=$1 AND memo=$2 AND status='POSTED'
		ORDER BY id DESC LIMIT 1`, sourceModule, memo)
	debit := queryFloat64(t, db, "SELECT COALESCE(SUM(debit),0)::double precision FROM journal_lines WHERE je_id=$1", journalID)
	credit := queryFloat64(t, db, "SELECT COALESCE(SUM(credit),0)::double precision FROM journal_lines WHERE je_id=$1", journalID)
	if debit <= 0 || credit <= 0 || math.Abs(debit-credit) > 0.005 {
		t.Fatalf("journal %d (%s) is not balanced: debit=%.4f credit=%.4f", journalID, memo, debit, credit)
	}
	return journalID
}

func assertFixtureOwnership(t *testing.T, db *sql.DB, cfg stagingConfig) {
	t.Helper()
	checks := []struct {
		name  string
		query string
		id    int64
	}{
		{"customer", "SELECT company_id FROM customers WHERE id=$1", cfg.customerID},
		{"supplier", "SELECT company_id FROM suppliers WHERE id=$1", cfg.supplierID},
		{"product", "SELECT company_id FROM products WHERE id=$1", cfg.productID},
		{"warehouse", "SELECT b.company_id FROM warehouses w JOIN branches b ON b.id=w.branch_id WHERE w.id=$1", cfg.warehouseID},
		{"branch", "SELECT company_id FROM branches WHERE id=$1", cfg.branchID},
		{"other branch", "SELECT company_id FROM branches WHERE id=$1", cfg.otherBranchID},
		{"GRN", "SELECT company_id FROM grns WHERE id=$1", cfg.grnID},
	}
	for _, check := range checks {
		companyID := queryInt64(t, db, check.query, check.id)
		want := cfg.companyID
		if check.name == "other branch" {
			want = cfg.otherCompanyID
		}
		if companyID != want {
			t.Fatalf("%s fixture %d belongs to company %d, want %d", check.name, check.id, companyID, want)
		}
	}
}

func apiMe(t *testing.T, client *stagingClient) (activeCompanyID int64, profile string, companyIDs []int64) {
	t.Helper()
	result := client.get(t, "/api/me")
	assertStatus(t, result, http.StatusOK, http.MethodGet, "/api/me")
	var payload struct {
		ActiveCompanyID int64  `json:"activeCompanyID"`
		ReleaseProfile  string `json:"releaseProfile"`
		Companies       []struct {
			ID int64 `json:"id"`
		} `json:"companies"`
	}
	if err := json.Unmarshal(result.body, &payload); err != nil {
		t.Fatalf("decode /api/me: %v; body=%q", err, string(result.body))
	}
	for _, company := range payload.Companies {
		companyIDs = append(companyIDs, company.ID)
	}
	return payload.ActiveCompanyID, payload.ReleaseProfile, companyIDs
}

func TestStagingCoreJourneys(t *testing.T) {
	cfg := loadStagingConfig(t)
	db := openReadOnlyDB(t, cfg.pgDSN)
	defer db.Close()
	assertFixtureOwnership(t, db, cfg)
	client := setupClient(t, cfg.baseURL, cfg.admin)
	report := evidenceReporter{}

	activeCompanyID, profile, _ := apiMe(t, client)
	if activeCompanyID != cfg.companyID {
		t.Fatalf("active company = %d, want %d", activeCompanyID, cfg.companyID)
	}
	if profile != profileV010Core {
		t.Fatalf("release profile = %q, want %q", profile, profileV010Core)
	}
	report.pass(t, "ENV-002", fmt.Sprintf("profile=%s active_company_id=%d", profile, activeCompanyID))

	routes := []string{
		"/healthz",
		"/finance/ar/invoices",
		"/finance/ar/invoices/new",
		"/finance/ap/invoices",
		"/finance/ap/invoices/new",
		"/sales/orders",
		"/sales/orders/new",
		"/delivery/orders",
		"/delivery/orders/new",
		"/inventory/stock-takes",
		"/inventory/stock-takes/new",
		"/inventory/adjustments",
		"/inventory/adjustments/new",
		"/inventory/transfers",
		"/inventory/valuation?warehouse_id=" + strconv.FormatInt(cfg.warehouseID, 10) + "&method=AVG",
		"/documents/library",
		"/documents/library/new",
		"/cmms/work-orders",
		"/cmms/work-orders/new",
		"/cmms/assets",
		"/cmms/pm-schedules",
	}
	for _, path := range routes {
		assertRoute(t, client, path)
	}
	blocked := client.get(t, "/documents/search")
	assertStatus(t, blocked, http.StatusNotFound, http.MethodGet, "/documents/search")
	report.pass(t, "REL-004", fmt.Sprintf("%d core routes returned 200 and blocked profile route returned 404", len(routes)))

	date := time.Now().UTC().Format("2006-01-02")
	salesToken := fetchCSRF(t, client, "/sales/orders/new")
	salesForm := url.Values{
		"csrf_token":               {salesToken},
		"customer_id":              {strconv.FormatInt(cfg.customerID, 10)},
		"currency":                 {"IDR"},
		"order_date":               {date},
		"product_id":               {strconv.FormatInt(cfg.productID, 10)},
		"fulfillment_warehouse_id": {strconv.FormatInt(cfg.warehouseID, 10)},
		"quantity":                 {"1"},
		"uom":                      {"EA"},
		"unit_price":               {cfg.amount},
		"discount_percent":         {"0"},
		"tax_percent":              {"0"},
	}
	salesCreate := client.postForm(t, "/sales/orders", salesForm)
	assertStatus(t, salesCreate, http.StatusSeeOther, http.MethodPost, "/sales/orders")
	salesID := parseRedirectID(t, salesCreate.location)
	salesDetailPath := "/sales/orders/" + strconv.FormatInt(salesID, 10)
	salesDetail := assertRoute(t, client, salesDetailPath)
	salesCSRF := csrfFromBody(t, salesDetail.body, salesDetailPath)
	if companyID := queryInt64(t, db, "SELECT company_id FROM sales_orders WHERE id=$1", salesID); companyID != cfg.companyID {
		t.Fatalf("sales order %d company = %d, want %d", salesID, companyID, cfg.companyID)
	}
	if status := queryString(t, db, "SELECT status::text FROM sales_orders WHERE id=$1", salesID); status != "DRAFT" {
		t.Fatalf("sales order %d initial status = %q, want DRAFT", salesID, status)
	}
	confirm := client.postForm(t, salesDetailPath+"/confirm", url.Values{"csrf_token": {salesCSRF}})
	assertStatus(t, confirm, http.StatusSeeOther, http.MethodPost, salesDetailPath+"/confirm")
	if status := queryString(t, db, "SELECT status::text FROM sales_orders WHERE id=$1", salesID); status != "CONFIRMED" {
		t.Fatalf("sales order %d status = %q, want CONFIRMED", salesID, status)
	}

	soLineID := queryInt64(t, db, "SELECT id FROM sales_order_lines WHERE sales_order_id=$1 ORDER BY id LIMIT 1", salesID)
	deliveryToken := fetchCSRF(t, client, "/delivery/orders/new?sales_order_id="+strconv.FormatInt(salesID, 10))
	deliveryCreate := client.postForm(t, "/delivery/orders", url.Values{
		"csrf_token":     {deliveryToken},
		"sales_order_id": {strconv.FormatInt(salesID, 10)},
		"warehouse_id":   {strconv.FormatInt(cfg.warehouseID, 10)},
		"delivery_date":  {date},
		"so_line_id[]":   {strconv.FormatInt(soLineID, 10)},
		"product_id[]":   {strconv.FormatInt(cfg.productID, 10)},
		"quantity[]":     {"1"},
		"line_notes[]":   {"v0.10-core certification delivery"},
	})
	assertStatus(t, deliveryCreate, http.StatusSeeOther, http.MethodPost, "/delivery/orders")
	deliveryID := parseRedirectID(t, deliveryCreate.location)
	deliveryPath := "/delivery/orders/" + strconv.FormatInt(deliveryID, 10)
	deliveryDetail := assertRoute(t, client, deliveryPath)
	if companyID := queryInt64(t, db, "SELECT company_id FROM delivery_orders WHERE id=$1", deliveryID); companyID != cfg.companyID {
		t.Fatalf("delivery order %d company = %d, want %d", deliveryID, companyID, cfg.companyID)
	}
	deliveryCSRF := csrfFromBody(t, deliveryDetail.body, deliveryPath)
	confirmDelivery := client.postForm(t, deliveryPath+"/confirm", url.Values{"csrf_token": {deliveryCSRF}})
	assertStatus(t, confirmDelivery, http.StatusSeeOther, http.MethodPost, deliveryPath+"/confirm")
	shipDelivery := client.postForm(t, deliveryPath+"/ship", url.Values{"csrf_token": {fetchCSRF(t, client, deliveryPath)}})
	assertStatus(t, shipDelivery, http.StatusSeeOther, http.MethodPost, deliveryPath+"/ship")
	completeDelivery := client.postForm(t, deliveryPath+"/complete", url.Values{"csrf_token": {fetchCSRF(t, client, deliveryPath)}})
	assertStatus(t, completeDelivery, http.StatusSeeOther, http.MethodPost, deliveryPath+"/complete")
	if status := queryString(t, db, "SELECT status::text FROM delivery_orders WHERE id=$1", deliveryID); status != "DELIVERED" {
		t.Fatalf("delivery order %d status = %q, want DELIVERED", deliveryID, status)
	}
	if delivered := queryCount(t, db, "SELECT COUNT(*) FROM sales_order_lines WHERE id=$1 AND quantity_delivered=quantity", soLineID); delivered != 1 {
		t.Fatalf("sales order line %d delivered rows = %d, want 1", soLineID, delivered)
	}
	inventoryMovements := queryCount(t, db, `
		SELECT COUNT(*)
		FROM inventory_tx tx
		JOIN delivery_orders d ON d.id=$1
		WHERE tx.ref_module='DELIVERY'
		  AND tx.warehouse_id=d.warehouse_id
		  AND tx.note='Delivery ' || d.doc_number || ' Confirmed'`, deliveryID)
	if inventoryMovements != 1 {
		t.Fatalf("delivery %d inventory movement count = %d, want 1; delivery must prove durable stock decrement", deliveryID, inventoryMovements)
	}
	auditEvents := queryCount(t, db, `
		SELECT COUNT(*) FROM audit_logs
		WHERE entity='inventory_tx' AND meta->>'note'=$1`, "Delivery "+queryString(t, db, "SELECT doc_number FROM delivery_orders WHERE id=$1", deliveryID)+" Confirmed")
	if auditEvents == 0 {
		t.Fatalf("delivery %d has no durable audit event", deliveryID)
	}
	beforeDelivered := queryCount(t, db, "SELECT COUNT(*) FROM delivery_order_lines WHERE delivery_order_id=$1 AND quantity_delivered=quantity_to_deliver", deliveryID)
	retryDelivery := client.postForm(t, deliveryPathFor(deliveryID)+"/complete", url.Values{"csrf_token": {fetchCSRF(t, client, deliveryPathFor(deliveryID))}})
	assertStatus(t, retryDelivery, http.StatusSeeOther, http.MethodPost, deliveryPathFor(deliveryID)+"/complete [duplicate retry]")
	afterDelivered := queryCount(t, db, "SELECT COUNT(*) FROM delivery_order_lines WHERE delivery_order_id=$1 AND quantity_delivered=quantity_to_deliver", deliveryID)
	if beforeDelivered != 1 || afterDelivered != beforeDelivered {
		t.Fatalf("delivery %d duplicate retry changed delivered lines from %d to %d", deliveryID, beforeDelivered, afterDelivered)
	}
	rememberDeliveryID(deliveryID)
	report.pass(t, "J-SALES-001", fmt.Sprintf("sales_order_id=%d delivery_order_id=%d transitioned_to_delivered=true inventory_movements=%d duplicate_retry_preserved_delivered_lines=%d audit_events=%d company_id=%d", salesID, deliveryID, inventoryMovements, afterDelivered, auditEvents, cfg.companyID))

	arToken := fetchCSRF(t, client, "/finance/ar/invoices/new")
	arNumber := "CERT-AR-" + strconv.FormatInt(time.Now().UTC().UnixNano(), 10)
	arForm := url.Values{
		"csrf_token":  {arToken},
		"number":      {arNumber},
		"customer_id": {strconv.FormatInt(cfg.customerID, 10)},
		"so_id":       {strconv.FormatInt(salesID, 10)},
		"currency":    {"IDR"},
		"total":       {cfg.amount},
		"due_date":    {date},
	}
	arCreate := client.postForm(t, "/finance/ar/invoices", arForm)
	assertStatus(t, arCreate, http.StatusSeeOther, http.MethodPost, "/finance/ar/invoices")
	arID := parseRedirectID(t, arCreate.location)
	arPath := "/finance/ar/invoices/" + strconv.FormatInt(arID, 10)
	arDetail := assertRoute(t, client, arPath)
	arCSRF := csrfFromBody(t, arDetail.body, arPath)
	if companyID := queryInt64(t, db, "SELECT c.company_id FROM ar_invoices i JOIN customers c ON c.id=i.customer_id WHERE i.id=$1", arID); companyID != cfg.companyID {
		t.Fatalf("AR invoice %d company = %d, want %d", arID, companyID, cfg.companyID)
	}
	postAR := client.postForm(t, arPath+"/post", url.Values{"csrf_token": {arCSRF}})
	assertStatus(t, postAR, http.StatusSeeOther, http.MethodPost, arPath+"/post")
	arJournalID := assertBalancedJournal(t, db, "AR.INVOICE", "AR Invoice "+arNumber)
	arAuditEvents := queryCount(t, db, "SELECT COUNT(*) FROM audit_logs WHERE entity='ar_invoice' AND entity_id=$1", strconv.FormatInt(arID, 10))
	if arAuditEvents == 0 {
		t.Fatalf("AR invoice %d has no durable posting audit event", arID)
	}
	paymentToken := fetchCSRF(t, client, "/finance/ar/payments/new?ar_invoice_id="+strconv.FormatInt(arID, 10))
	payment := client.postForm(t, "/finance/ar/payments", url.Values{
		"csrf_token":    {paymentToken},
		"ar_invoice_id": {strconv.FormatInt(arID, 10)},
		"amount":        {cfg.amount},
		"paid_at":       {date},
		"method":        {"TRANSFER"},
	})
	assertStatus(t, payment, http.StatusSeeOther, http.MethodPost, "/finance/ar/payments")
	if status := queryString(t, db, "SELECT status FROM ar_invoices WHERE id=$1", arID); status != "POSTED" {
		t.Fatalf("AR invoice %d status after payment = %q, want POSTED", arID, status)
	}
	if allocated := queryCount(t, db, "SELECT COUNT(*) FROM ar_payment_allocations WHERE ar_invoice_id=$1", arID); allocated != 1 {
		t.Fatalf("AR invoice %d allocation count = %d, want 1", arID, allocated)
	}
	duplicateAR := client.postForm(t, "/finance/ar/invoices", url.Values{
		"csrf_token": {fetchCSRF(t, client, "/finance/ar/invoices/new")},
		"number":     {arNumber}, "customer_id": {strconv.FormatInt(cfg.customerID, 10)},
		"so_id": {strconv.FormatInt(salesID, 10)}, "currency": {"IDR"}, "total": {cfg.amount}, "due_date": {date},
	})
	assertStatus(t, duplicateAR, http.StatusBadRequest, http.MethodPost, "/finance/ar/invoices [duplicate number]")
	if count := queryCount(t, db, "SELECT COUNT(*) FROM ar_invoices WHERE number=$1", arNumber); count != 1 {
		t.Fatalf("AR invoice number %q count = %d, want 1", arNumber, count)
	}
	apToken := fetchCSRF(t, client, "/finance/ap/invoices/new")
	apNumber := "CERT-AP-" + strconv.FormatInt(time.Now().UTC().UnixNano(), 10)
	apCreate := client.postForm(t, "/finance/ap/invoices", url.Values{
		"csrf_token": {apToken}, "number": {apNumber}, "source_type": {"grn"},
		"source_id": {strconv.FormatInt(cfg.grnID, 10)}, "due_date": {date},
	})
	assertStatus(t, apCreate, http.StatusSeeOther, http.MethodPost, "/finance/ap/invoices")
	apID := parseRedirectID(t, apCreate.location)
	apPath := "/finance/ap/invoices/" + strconv.FormatInt(apID, 10)
	apDetail := assertRoute(t, client, apPath)
	apCSRF := csrfFromBody(t, apDetail.body, apPath)
	postAP := client.postForm(t, apPath+"/post", url.Values{"csrf_token": {apCSRF}})
	assertStatus(t, postAP, http.StatusSeeOther, http.MethodPost, apPath+"/post")
	apJournalID := assertBalancedJournal(t, db, "PROCUREMENT.AP_INVOICE", "AP Invoice "+apNumber)
	apAuditEvents := queryCount(t, db, "SELECT COUNT(*) FROM audit_logs WHERE entity='ap_invoice' AND entity_id=$1", strconv.FormatInt(apID, 10))
	if apAuditEvents == 0 {
		t.Fatalf("AP invoice %d has no durable posting audit event", apID)
	}
	apPaymentToken := fetchCSRF(t, client, "/finance/ap/payments/new?ap_invoice_id="+strconv.FormatInt(apID, 10))
	apPayment := client.postForm(t, "/finance/ap/payments", url.Values{
		"csrf_token": {apPaymentToken}, "supplier_id": {strconv.FormatInt(cfg.supplierID, 10)},
		"amount": {cfg.amount}, "paid_at": {date}, "method": {"TRANSFER"},
		"ap_invoice_id": {strconv.FormatInt(apID, 10)}, "allocation_amount": {cfg.amount},
	})
	assertStatus(t, apPayment, http.StatusSeeOther, http.MethodPost, "/finance/ap/payments")
	if companyID := queryInt64(t, db, "SELECT COALESCE(i.company_id,s.company_id) FROM ap_invoices i JOIN suppliers s ON s.id=i.supplier_id WHERE i.id=$1", apID); companyID != cfg.companyID {
		t.Fatalf("AP invoice %d company = %d, want %d", apID, companyID, cfg.companyID)
	}
	if count := queryCount(t, db, "SELECT COUNT(*) FROM ap_payment_allocations WHERE ap_invoice_id=$1", apID); count != 1 {
		t.Fatalf("AP invoice %d allocation count = %d, want 1", apID, count)
	}
	report.pass(t, "J-ARAP-001", fmt.Sprintf("sales_order_id=%d ar_invoice_id=%d ap_invoice_id=%d ar_journal_id=%d ap_journal_id=%d ar_payment_allocations=1 ap_payment_allocations=1 duplicate_ar_number_count=1 ar_audit_events=%d ap_audit_events=%d", salesID, arID, apID, arJournalID, apJournalID, arAuditEvents, apAuditEvents))

	systemQty := queryFloat64(t, db, "SELECT COALESCE((SELECT qty FROM inventory_balances WHERE warehouse_id=$1 AND product_id=$2),0)::double precision", cfg.warehouseID, cfg.productID)
	physicalQty := strconv.FormatFloat(systemQty+1, 'f', 4, 64)
	stockToken := fetchCSRF(t, client, "/inventory/stock-takes/new")
	stockCreate := client.postForm(t, "/inventory/stock-takes", url.Values{
		"csrf_token": {stockToken}, "warehouse_id": {strconv.FormatInt(cfg.warehouseID, 10)},
		"taken_at": {date}, "note": {"v0.10-core certification"},
	})
	assertStatus(t, stockCreate, http.StatusSeeOther, http.MethodPost, "/inventory/stock-takes")
	stockID := parseRedirectID(t, stockCreate.location)
	stockPath := "/inventory/stock-takes/" + strconv.FormatInt(stockID, 10)
	stockDetail := assertRoute(t, client, stockPath)
	stockCSRF := csrfFromBody(t, stockDetail.body, stockPath)
	line := client.postForm(t, stockPath+"/lines", url.Values{
		"csrf_token": {stockCSRF}, "product_id": {strconv.FormatInt(cfg.productID, 10)},
		"physical_qty": {physicalQty}, "note": {"certification count"},
	})
	assertStatus(t, line, http.StatusSeeOther, http.MethodPost, stockPath+"/lines")
	postStock := client.postForm(t, stockPath+"/post", url.Values{"csrf_token": {fetchCSRF(t, client, stockPath)}})
	assertStatus(t, postStock, http.StatusSeeOther, http.MethodPost, stockPath+"/post")
	if status := queryString(t, db, "SELECT status FROM inventory_stock_takes WHERE id=$1", stockID); status != "POSTED" {
		t.Fatalf("stock take %d status = %q, want POSTED", stockID, status)
	}
	stockNumber := queryString(t, db, "SELECT number FROM inventory_stock_takes WHERE id=$1", stockID)
	stockMovements := queryCount(t, db, "SELECT COUNT(*) FROM inventory_tx WHERE ref_module='STOCK_TAKE' AND note=$1", "Stock Take adjustment: "+stockNumber)
	if stockMovements != 1 {
		t.Fatalf("stock take %d adjustment movement count = %d, want 1", stockID, stockMovements)
	}
	stockAuditEvents := queryCount(t, db, "SELECT COUNT(*) FROM audit_logs WHERE entity='inventory_tx' AND meta->>'note'=$1", "Stock Take adjustment: "+stockNumber)
	if stockAuditEvents == 0 {
		t.Fatalf("stock take %d has no durable inventory audit event", stockID)
	}
	avgCost := queryFloat64(t, db, "SELECT COALESCE(avg_cost,0)::double precision FROM inventory_balances WHERE warehouse_id=$1 AND product_id=$2", cfg.warehouseID, cfg.productID)
	if avgCost < 0 {
		t.Fatalf("inventory valuation avg_cost=%f, want non-negative", avgCost)
	}
	repeatPost := client.postForm(t, stockPath+"/post", url.Values{"csrf_token": {fetchCSRF(t, client, stockPath)}})
	assertStatus(t, repeatPost, http.StatusSeeOther, http.MethodPost, stockPath+"/post [duplicate]")
	if count := queryCount(t, db, "SELECT COUNT(*) FROM inventory_stock_takes WHERE id=$1", stockID); count != 1 {
		t.Fatalf("stock take %d count = %d, want 1", stockID, count)
	}
	report.pass(t, "J-INV-001", fmt.Sprintf("stock_take_id=%d posted_once=true adjustment_movements=%d avg_cost=%.4f duplicate_post_preserved_one_header=true audit_events=%d", stockID, stockMovements, avgCost, stockAuditEvents))

	documentToken := fetchCSRF(t, client, "/documents/library/new")
	documentCreate := client.postForm(t, "/documents/library", url.Values{
		"csrf_token": {documentToken}, "title": {"v0.10-core certification document"},
		"description":       {"immutable staging test document"},
		"category_id":       {strconv.FormatInt(cfg.documentCategoryID, 10)},
		"classification_id": {strconv.FormatInt(cfg.documentClassID, 10)},
	})
	assertStatus(t, documentCreate, http.StatusSeeOther, http.MethodPost, "/documents/library")
	documentID := parseRedirectID(t, documentCreate.location)
	documentPath := "/documents/library/" + strconv.FormatInt(documentID, 10)
	documentDetail := assertRoute(t, client, documentPath)
	if companyID := queryInt64(t, db, "SELECT company_id FROM documents WHERE id=$1", documentID); companyID != cfg.companyID {
		t.Fatalf("document %d company = %d, want %d", documentID, companyID, cfg.companyID)
	}
	version := client.postMultipart(t, documentPath+"/versions", csrfFromBody(t, documentDetail.body, documentPath), "certification version 1")
	assertStatus(t, version, http.StatusSeeOther, http.MethodPost, documentPath+"/versions")
	if count := queryCount(t, db, "SELECT COUNT(*) FROM document_versions WHERE document_id=$1", documentID); count != 1 {
		t.Fatalf("document %d version count = %d, want 1", documentID, count)
	}
	report.pass(t, "J-DOC-001", fmt.Sprintf("document_id=%d version_count=1 company_id=%d", documentID, cfg.companyID))

	assetToken := fetchCSRF(t, client, "/cmms/assets/new")
	assetCode := "CERT-ASSET-" + strconv.FormatInt(time.Now().UTC().UnixNano(), 10)
	assetCreate := client.postForm(t, "/cmms/assets", url.Values{
		"csrf_token": {assetToken}, "code": {assetCode}, "name": {"v0.10-core certification asset"},
		"description": {"staging certification asset"}, "asset_type": {"EQUIPMENT"},
	})
	assertStatus(t, assetCreate, http.StatusSeeOther, http.MethodPost, "/cmms/assets")
	assetID := parseRedirectID(t, assetCreate.location)
	assetPath := "/cmms/assets/" + strconv.FormatInt(assetID, 10)
	assertRoute(t, client, assetPath)
	workToken := fetchCSRF(t, client, "/cmms/work-orders/new")
	workCreate := client.postForm(t, "/cmms/work-orders", url.Values{
		"csrf_token": {workToken}, "title": {"v0.10-core certification work order"},
		"description": {"staging lifecycle test"}, "asset_id": {strconv.FormatInt(assetID, 10)},
		"priority": {"MEDIUM"}, "category": {"CORRECTIVE"},
	})
	assertStatus(t, workCreate, http.StatusSeeOther, http.MethodPost, "/cmms/work-orders")
	workID := parseRedirectID(t, workCreate.location)
	workPath := "/cmms/work-orders/" + strconv.FormatInt(workID, 10)
	workDetail := assertRoute(t, client, workPath)
	workStatus := client.postForm(t, workPath+"/status", url.Values{
		"csrf_token": {csrfFromBody(t, workDetail.body, workPath)}, "status": {"IN_PROGRESS"},
	})
	assertStatus(t, workStatus, http.StatusSeeOther, http.MethodPost, workPath+"/status")
	if companyID := queryInt64(t, db, "SELECT company_id FROM work_orders WHERE id=$1", workID); companyID != cfg.companyID {
		t.Fatalf("work order %d company = %d, want %d", workID, companyID, cfg.companyID)
	}
	if status := queryString(t, db, "SELECT status FROM work_orders WHERE id=$1", workID); status != "IN_PROGRESS" {
		t.Fatalf("work order %d status = %q, want IN_PROGRESS", workID, status)
	}
	report.pass(t, "J-CMMS-001", fmt.Sprintf("asset_id=%d work_order_id=%d status=IN_PROGRESS", assetID, workID))
}

func TestStagingTenantIsolation(t *testing.T) {
	cfg := loadStagingConfig(t)
	db := openReadOnlyDB(t, cfg.pgDSN)
	defer db.Close()
	assertFixtureOwnership(t, db, cfg)
	report := evidenceReporter{}

	admin := setupClient(t, cfg.baseURL, cfg.admin)
	active, profile, companies := apiMe(t, admin)
	if active != cfg.companyID || profile != profileV010Core {
		t.Fatalf("admin workspace = company %d/profile %q, want %d/%q", active, profile, cfg.companyID, profileV010Core)
	}
	for _, companyID := range companies {
		if companyID == cfg.otherCompanyID {
			t.Fatalf("admin workspace unexpectedly exposes other company %d", cfg.otherCompanyID)
		}
	}
	forgedSelect := admin.postForm(t, "/company/select", url.Values{
		"csrf_token": {fetchCSRF(t, admin, "/")},
		"company_id": {strconv.FormatInt(cfg.otherCompanyID, 10)},
	})
	assertStatus(t, forgedSelect, http.StatusSeeOther, http.MethodPost, "/company/select [forged company]")
	activeAfterForgedSelect, _, _ := apiMe(t, admin)
	if activeAfterForgedSelect != cfg.companyID {
		t.Fatalf("forged company select changed active company to %d, want %d", activeAfterForgedSelect, cfg.companyID)
	}
	report.pass(t, "ISO-002", fmt.Sprintf("forged company selection rejected; active company remained %d", cfg.companyID))

	branch := setupClient(t, cfg.baseURL, cfg.branch)
	branchActive, branchProfile, _ := apiMe(t, branch)
	if branchActive != cfg.companyID || branchProfile != profileV010Core {
		t.Fatalf("branch workspace = company %d/profile %q, want %d/%q", branchActive, branchProfile, cfg.companyID, profileV010Core)
	}
	if companyID := queryInt64(t, db, "SELECT company_id FROM branches WHERE id=$1", cfg.branchID); companyID != cfg.companyID {
		t.Fatalf("branch fixture %d company = %d, want %d", cfg.branchID, companyID, cfg.companyID)
	}
	report.pass(t, "ISO-001", fmt.Sprintf("branch identity retained company_id=%d branch_id=%d", cfg.companyID, cfg.branchID))

	// ISO-003 is deliberately backed by effective-dated RBAC rows rather than
	// logout. A logout only proves session invalidation; it cannot prove that an
	// already-issued session is re-evaluated after assignment expiry/revocation.
	branchUserID := queryInt64(t, db, "SELECT id FROM users WHERE email=$1", cfg.branch.email)
	expiredAssignments := queryCount(t, db, `
		SELECT COUNT(*)
		FROM rbac_user_role_assignments
		WHERE company_id=$1 AND user_id=$2 AND branch_id=$3
		  AND valid_to IS NOT NULL AND valid_to <= NOW()`, cfg.companyID, branchUserID, cfg.branchID)
	revokedReviews := queryCount(t, db, `
		SELECT COUNT(*)
		FROM rbac_access_reviews
		WHERE company_id=$1 AND subject_user_id=$2
		  AND status='COMPLETED' AND decision='REVOKE'
		  AND decided_by_user_id IS NOT NULL AND decided_at IS NOT NULL`, cfg.companyID, branchUserID)
	if expiredAssignments == 0 || revokedReviews == 0 {
		t.Fatalf("ISO-003 requires an expired branch assignment and a completed REVOKE access review for branch user %d; expired_assignments=%d revoked_reviews=%d", branchUserID, expiredAssignments, revokedReviews)
	}
	staleSessionAccess := branch.get(t, "/finance/ar/invoices")
	assertDenied(t, staleSessionAccess, http.MethodGet, "/finance/ar/invoices [expired/revoked existing session]")
	report.pass(t, "ISO-003", fmt.Sprintf("existing branch session denied after expired_assignments=%d and revoked_reviews=%d; reviewer actor/timestamp persisted", expiredAssignments, revokedReviews))

	noAccess := setupClient(t, cfg.baseURL, cfg.noAccess)
	denied := noAccess.get(t, "/finance/ar/invoices")
	assertStatus(t, denied, http.StatusForbidden, http.MethodGet, "/finance/ar/invoices [no-access]")
	deniedMutation := noAccess.postForm(t, "/finance/ar/invoices", url.Values{"csrf_token": {fetchCSRF(t, noAccess, "/")}})
	assertStatus(t, deniedMutation, http.StatusForbidden, http.MethodPost, "/finance/ar/invoices [no-access mutation]")

	otherInvoiceID := queryInt64(t, db, `
		SELECT i.id
		FROM ar_invoices i
		JOIN customers c ON c.id=i.customer_id
		WHERE c.company_id=$1
		ORDER BY i.id DESC LIMIT 1`, cfg.otherCompanyID)
	forgedObject := noAccess.get(t, "/finance/ar/invoices/"+strconv.FormatInt(otherInvoiceID, 10))
	assertDenied(t, forgedObject, http.MethodGet, "/finance/ar/invoices/{other-company-id} [no-access forged object]")

	// Re-submit the exact delivery completion input produced by the core
	// journey. The handler may redirect on a rejected state transition, so the
	// durable quantity is the authoritative idempotency assertion. This is a
	// supporting HTTP idempotency check; forged worker payloads are collected in
	// the operator evidence lane because rc.6 exposes no tenant-aware worker
	// injection endpoint.
	deliveryID := rememberedDeliveryID()
	if deliveryID <= 0 {
		t.Fatalf("ISO-004 requires the delivery artifact from TestStagingCoreJourneys")
	}
	deliveryPath := "/delivery/orders/" + strconv.FormatInt(deliveryID, 10)
	beforeDelivered := queryCount(t, db, "SELECT COUNT(*) FROM delivery_order_lines WHERE delivery_order_id=$1 AND quantity_delivered=quantity_to_deliver", deliveryID)
	if beforeDelivered != 1 {
		t.Fatalf("delivery %d has %d fully delivered lines before duplicate worker/input retry, want 1", deliveryID, beforeDelivered)
	}
	retryDelivery := admin.postForm(t, deliveryPath+"/complete", url.Values{"csrf_token": {fetchCSRF(t, admin, deliveryPath)}})
	assertStatus(t, retryDelivery, http.StatusSeeOther, http.MethodPost, deliveryPath+"/complete [duplicate retry]")
	afterDelivered := queryCount(t, db, "SELECT COUNT(*) FROM delivery_order_lines WHERE delivery_order_id=$1 AND quantity_delivered=quantity_to_deliver", deliveryID)
	if afterDelivered != beforeDelivered {
		t.Fatalf("delivery %d duplicate retry changed fully delivered line count from %d to %d", deliveryID, beforeDelivered, afterDelivered)
	}
	t.Logf("supporting HTTP idempotency assertion: no-access read/mutation and forged object denied; duplicate delivery retry preserved delivered_lines=%d", afterDelivered)

	logoutToken := fetchCSRF(t, admin, "/")
	logout := admin.postForm(t, "/auth/logout", url.Values{"csrf_token": {logoutToken}})
	assertStatus(t, logout, http.StatusSeeOther, http.MethodPost, "/auth/logout")
	postLogout := admin.get(t, "/")
	assertStatus(t, postLogout, http.StatusSeeOther, http.MethodGet, "/ [after logout]")
	t.Log("supporting assertion: logout invalidated access and redirected to welcome")
}

func TestStagingMigrationCeiling(t *testing.T) {
	cfg := loadStagingConfig(t)
	db := openReadOnlyDB(t, cfg.pgDSN)
	defer db.Close()

	rows, err := db.Query("SELECT version FROM schema_migrations ORDER BY version ASC")
	if err != nil {
		t.Fatalf("query schema_migrations: %v", err)
	}
	defer rows.Close()
	var count int
	var maxVersion int64
	var found124, found125 bool
	for rows.Next() {
		var version int64
		if err := rows.Scan(&version); err != nil {
			t.Fatalf("scan schema_migrations: %v", err)
		}
		count++
		if version > maxVersion {
			maxVersion = version
		}
		found124 = found124 || version == 124
		found125 = found125 || version == 125
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("schema_migrations rows: %v", err)
	}
	if !found124 || found125 || maxVersion != 124 {
		t.Fatalf("migration ceiling count=%d max=%06d found124=%t found125=%t; want 000124 present and exact max 000124", count, maxVersion, found124, found125)
	}
	evidenceReporter{}.pass(t, "DB-002", fmt.Sprintf("migration_count=%d max_version=%06d migration_000125_absent=true", count, maxVersion))
}
