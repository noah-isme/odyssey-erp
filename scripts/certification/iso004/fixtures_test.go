package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testdata/fixtures.json has the exact shape printed by
// `go run ./scripts/seed/staging --iso004 --json` (key 9000000001).
const sampleFixtures = "testdata/fixtures.json"

func TestLoadFixturesFromSeedJSON(t *testing.T) {
	fx, src, err := loadFixtures(sampleFixtures, envMap(nil))
	require.NoError(t, err)
	assert.Equal(t, "9000000001", fx.Key)
	assert.Equal(t, int64(3), fx.CompanyA)
	assert.Equal(t, int64(4), fx.CompanyB)
	assert.Equal(t, int64(6), fx.ActorUserB)
	assert.Equal(t, int64(4), fx.APInvoiceBMatched)
	assert.Equal(t, "iso004-payslip-9000000001@staging.invalid", fx.PayslipBEmail)
	assert.Len(t, src, len(fixtureFields()), "every key comes from the file")
	for _, s := range src {
		assert.Equal(t, "file", s)
	}
	assert.Empty(t, validateFixtures(fx, "9000000001"))
}

// TestFixtureFieldsMatchSeedOutput keeps the loader aligned with the seed's
// exported variable names (scripts/seed/staging/iso004_output.go).
func TestFixtureFieldsMatchSeedOutput(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "..", "seed", "staging", "iso004_output.go"))
	require.NoError(t, err)
	seedNames := map[string]bool{}
	for _, m := range regexp.MustCompile(`"(STAGING_CERT_ISO004_[A-Z0-9_]+)"`).FindAllStringSubmatch(string(src), -1) {
		seedNames[m[1]] = true
	}
	require.NotEmpty(t, seedNames)
	ours := map[string]bool{}
	for _, f := range fixtureFields() {
		ours[f.Name] = true
	}
	assert.Equal(t, seedNames, ours)
}

func TestLoadFixturesEnvFallback(t *testing.T) {
	env := map[string]string{}
	fx, _, err := loadFixtures(sampleFixtures, envMap(nil))
	require.NoError(t, err)
	// Export every value as the seed's shell exports would.
	for _, f := range fixtureFields() {
		if f.Numeric {
			env[f.Name] = itoa(*f.int(fx))
		} else {
			env[f.Name] = *f.str(fx)
		}
	}
	fromEnv, src, err := loadFixtures("", envMap(env))
	require.NoError(t, err)
	assert.Equal(t, fx, fromEnv)
	assert.Equal(t, "env:STAGING_CERT_ISO004_COMPANY_A_ID", src["STAGING_CERT_ISO004_COMPANY_A_ID"])

	// Base-seeder names are the fallback for the company IDs only.
	delete(env, "STAGING_CERT_ISO004_COMPANY_A_ID")
	env["STAGING_CERT_COMPANY_ID"] = "3"
	fromEnv, src, err = loadFixtures("", envMap(env))
	require.NoError(t, err)
	assert.Equal(t, int64(3), fromEnv.CompanyA)
	assert.Equal(t, "env:STAGING_CERT_COMPANY_ID", src["STAGING_CERT_ISO004_COMPANY_A_ID"])
}

func TestLoadFixturesFileWinsAndMissingKeysFallBack(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "partial.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"STAGING_CERT_ISO004_KEY":"k1","STAGING_CERT_ISO004_COMPANY_A_ID":"11"}`), 0o600))
	fx, src, err := loadFixtures(path, envMap(map[string]string{
		"STAGING_CERT_ISO004_COMPANY_A_ID": "99",
		"STAGING_CERT_ISO004_COMPANY_B_ID": "12",
	}))
	require.NoError(t, err)
	assert.Equal(t, int64(11), fx.CompanyA, "string numbers are accepted and the file wins")
	assert.Equal(t, int64(12), fx.CompanyB)
	assert.Equal(t, "file", src["STAGING_CERT_ISO004_COMPANY_A_ID"])
	assert.Equal(t, "env:STAGING_CERT_ISO004_COMPANY_B_ID", src["STAGING_CERT_ISO004_COMPANY_B_ID"])
	problems := strings.Join(validateFixtures(fx, "k1"), "\n")
	assert.Contains(t, problems, "STAGING_CERT_ISO004_PAYSLIP_B_ID is missing")
	assert.NotContains(t, problems, "COMPANY_A_ID")
}

func TestLoadFixturesRejectsBadInput(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(p, []byte(body), 0o600))
		return p
	}
	cases := map[string]struct {
		path string
		env  map[string]string
		want string
	}{
		"unknown key":      {write("u.json", `{"STAGING_CERT_ISO004_COMPANY_C_ID":1}`), nil, "unknown keys STAGING_CERT_ISO004_COMPANY_C_ID"},
		"not json":         {write("n.json", `not json`), nil, "parse fixtures file"},
		"float id":         {write("f.json", `{"STAGING_CERT_ISO004_COMPANY_A_ID":1.5}`), nil, "not an integer"},
		"object value":     {write("o.json", `{"STAGING_CERT_ISO004_COMPANY_A_ID":{}}`), nil, "expected a number or string"},
		"missing file":     {filepath.Join(dir, "absent.json"), nil, "read fixtures file"},
		"env not a number": {"", map[string]string{"STAGING_CERT_ISO004_PAYSLIP_B_ID": "abc"}, "STAGING_CERT_ISO004_PAYSLIP_B_ID"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, _, err := loadFixtures(tc.path, envMap(tc.env))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

func TestValidateFixtures(t *testing.T) {
	good := func() *Fixtures {
		fx, _, err := loadFixtures(sampleFixtures, envMap(nil))
		require.NoError(t, err)
		return fx
	}
	cases := map[string]struct {
		mutate func(*Fixtures)
		runID  string
		want   string
	}{
		"run id differs from seed key": {func(*Fixtures) {}, "123", "does not equal --run-id"},
		"same companies":               {func(f *Fixtures) { f.CompanyB = f.CompanyA }, "9000000001", "company A and company B are the same"},
		"zero id":                      {func(f *Fixtures) { f.OCRJobForged = 0 }, "9000000001", "STAGING_CERT_ISO004_OCR_JOB_FORGED_ID is missing"},
		"negative id":                  {func(f *Fixtures) { f.GRNB = -1 }, "9000000001", "STAGING_CERT_ISO004_AP_GRN_B_ID"},
		"duplicate invoices":           {func(f *Fixtures) { f.APInvoiceBMatched = f.APInvoiceBMissing }, "9000000001", "must be distinct"},
		"same suppliers":               {func(f *Fixtures) { f.SupplierBPolicy = f.SupplierBNoPolicy }, "9000000001", "AP suppliers"},
		"same forecast scenarios":      {func(f *Fixtures) { f.ForecastScenarioB = f.ForecastScenarioA }, "9000000001", "forecast scenarios"},
		"admin is invoice creator":     {func(f *Fixtures) { f.AdminUserA = f.APInvoiceBCreatedBy }, "9000000001", "forged-actor"},
		"creator is not actor B":       {func(f *Fixtures) { f.APInvoiceBCreatedBy = 99 }, "9000000001", "does not equal ACTOR_B_USER_ID"},
		"payslip email of another key": {func(f *Fixtures) { f.PayslipBEmail = "iso004-payslip-1@staging.invalid" }, "9000000001", "PAYSLIP_B_EMAIL"},
		"payslip timestamp":            {func(f *Fixtures) { f.PayslipBCreatedAt = "2026-10-04 04:41" }, "9000000001", "not RFC3339"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			fx := good()
			tc.mutate(fx)
			problems := validateFixtures(fx, tc.runID)
			require.NotEmpty(t, problems)
			assert.Contains(t, strings.Join(problems, "\n"), tc.want)
		})
	}
	assert.Equal(t, []string{"no fixtures loaded"}, validateFixtures(nil, "x"))
}

func TestFixtureChecksAreParameterizedSelects(t *testing.T) {
	fx, _, err := loadFixtures(sampleFixtures, envMap(nil))
	require.NoError(t, err)
	checks := fixtureChecks(fx)
	require.Len(t, checks, 20)
	names := map[string]bool{}
	for _, c := range checks {
		assert.False(t, names[c.Name], "duplicate check %s", c.Name)
		names[c.Name] = true
		assertSelectOnly(t, c.SQL)
		assert.Contains(t, c.SQL, "$1", "%s is parameterized", c.Name)
		assert.Len(t, c.Args, 1)
		assert.NotEmpty(t, c.Expect)
	}
	byName := map[string]fixtureCheck{}
	for _, c := range checks {
		byName[c.Name] = c
	}
	assert.Equal(t, "company:4/version:1/version-company:3/status:PENDING", byName["ocr_job_forged"].Expect)
	assert.Equal(t, "company:4/supplier:3/supplier-company:4/created_by:6/status:DRAFT", byName["ap_invoice_b_missing"].Expect)
	assert.Equal(t, "company:4/employee-company:4/email:iso004-payslip-9000000001@staging.invalid", byName["payslip_b"].Expect)
}

var writeKeywords = regexp.MustCompile(`(?i)\b(INSERT|UPDATE|DELETE|MERGE|UPSERT|TRUNCATE|DROP|ALTER|CREATE|GRANT|REVOKE|COPY|CALL|DO|LOCK|NEXTVAL|SETVAL|VACUUM|REINDEX|CLUSTER|REFRESH)\b`)

func assertSelectOnly(t *testing.T, sql string) {
	t.Helper()
	trimmed := strings.TrimSpace(sql)
	upper := strings.ToUpper(trimmed)
	assert.True(t, strings.HasPrefix(upper, "SELECT") || strings.HasPrefix(upper, "SHOW"), "statement must be SELECT/SHOW: %s", sql)
	assert.False(t, writeKeywords.MatchString(trimmed), "statement contains a write keyword: %s", sql)
	assert.NotContains(t, trimmed, ";", "single statement only: %s", sql)
}

// TestToolSourceIsSelectOnly scans every non-test source file of the tool for
// SQL string literals and requires them to be SELECT/SHOW statements.
func TestToolSourceIsSelectOnly(t *testing.T) {
	files, err := filepath.Glob("*.go")
	require.NoError(t, err)
	sqlLiteral := regexp.MustCompile("(?s)`\\s*((?:SELECT|SHOW|INSERT|UPDATE|DELETE|WITH|CREATE|ALTER|DROP|TRUNCATE|MERGE|COPY|SET)\\b[^`]*)`")
	found := 0
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		require.NoError(t, err)
		for _, m := range sqlLiteral.FindAllStringSubmatch(string(src), -1) {
			found++
			assertSelectOnly(t, m[1])
		}
	}
	assert.Greater(t, found, 5, "the scan must find the tool's SQL")
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
