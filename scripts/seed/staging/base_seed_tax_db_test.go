package main

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

// baseTaxTables are the tables of the reviewed tax configuration.
var baseTaxTables = []string{"tax_rule_versions", "tax_codes", "company_tax_identities", "tax_account_mappings", "tax_invoice_number_ranges"}

func (d *baseTestDB) taxRows(t *testing.T) string {
	t.Helper()
	var out []string
	for _, table := range baseTaxTables {
		out = append(out, table+"="+d.scalar(t, "SELECT COUNT(*)::text FROM "+table))
	}
	return strings.Join(out, ",")
}

func taxSteps(res *baseResult) string {
	var out []string
	for _, s := range res.Steps {
		if strings.HasPrefix(s.Table, "tax_") || s.Table == "company_tax_identities" {
			out = append(out, s.Name+"="+s.Status)
		}
	}
	return strings.Join(out, ",")
}

// TestBaseSeedDBTaxConfigurationCreatedOnceAndChecked: a clean run creates
// exactly one row of each tax prerequisite, a rerun reuses every one of them
// and --check then reports none missing.
func TestBaseSeedDBTaxConfigurationCreatedOnceAndChecked(t *testing.T) {
	d := newBaseTestDB(t)
	ctx := context.Background()

	check := d.opts()
	check.Check = true
	res, err := runBaseSeed(ctx, check)
	if err != nil {
		t.Fatalf("check on unseeded db: %v", err)
	}
	const all = "tax_rule_version_vat=%[1]s,tax_code VAT_OUTPUT=%[1]s,tax_code VAT_INPUT=%[1]s,company_tax_identity_primary=%[1]s,tax_account_mapping VAT_OUTPUT=%[1]s,tax_account_mapping VAT_INPUT=%[1]s,tax_invoice_number_range_primary=%[1]s"
	if got, want := taxSteps(res), fmt.Sprintf(all, "created"); got != want {
		t.Errorf("--check on an unseeded db: tax steps %q, want %q (reported missing)", got, want)
	}

	first, err := runBaseSeed(ctx, d.opts())
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	if got, want := taxSteps(first), fmt.Sprintf(all, "created"); got != want {
		t.Errorf("first run: tax steps %q, want %q", got, want)
	}
	want := "tax_rule_versions=1,tax_codes=2,company_tax_identities=1,tax_account_mappings=2,tax_invoice_number_ranges=1"
	if got := d.taxRows(t); got != want {
		t.Fatalf("after first run: %s, want %s", got, want)
	}

	second, err := runBaseSeed(ctx, d.opts())
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if got, want := taxSteps(second), fmt.Sprintf(all, "reused"); got != want {
		t.Errorf("second run: tax steps %q, want %q", got, want)
	}
	if got := d.taxRows(t); got != want {
		t.Fatalf("after second run: %s, want %s (exactly one of each)", got, want)
	}

	res, err = runBaseSeed(ctx, check)
	if err != nil {
		t.Fatalf("check after seed: %v", err)
	}
	if got, want := taxSteps(res), fmt.Sprintf(all, "reused"); got != want {
		t.Errorf("--check after seed: tax steps %q, want %q (reported present)", got, want)
	}
}

// TestBaseSeedDBTaxReusesOperatorConfiguration: a complete reviewed operator
// configuration (rule version with VAT output and input codes, identity,
// mappings, number range) is reused as it is; the seeded rule version is not
// created and no operator row changes.
func TestBaseSeedDBTaxReusesOperatorConfiguration(t *testing.T) {
	d := newBaseTestDB(t)
	ctx := context.Background()
	d.exec(t, `INSERT INTO users (email, password_hash) VALUES ('operator@example.test', 'x')`)
	d.exec(t, `INSERT INTO companies (code, name) VALUES ($1, $2)`, baseCompanyCode, baseCompanyName)
	d.exec(t, `INSERT INTO accounts (code, name, type) VALUES ('OPS-VAT', 'Operator VAT', 'LIABILITY')`)
	d.exec(t, `INSERT INTO tax_rule_versions (rule_kind, version_code, effective_from, source_url, source_checksum, reviewed_by, reviewed_at, created_by)
		SELECT 'VAT', 'OPS-VAT', CURRENT_DATE - 30, 'https://example.test/vat', 'sha256:ops', id, NOW(), id FROM users WHERE email = 'operator@example.test'`)
	d.exec(t, `INSERT INTO tax_codes (rule_version_id, code, name, tax_kind)
		SELECT id, k, k, k FROM tax_rule_versions, unnest(ARRAY['VAT_OUTPUT','VAT_INPUT']) k WHERE version_code = 'OPS-VAT'`)
	d.exec(t, `INSERT INTO company_tax_identities (company_id, legal_name, npwp, nitku, registered_address, effective_from, created_by)
		SELECT c.id, 'Operator legal', '1', '2', 'Addr', CURRENT_DATE - 30, u.id FROM companies c, users u WHERE c.code = $1 AND u.email = 'operator@example.test'`, baseCompanyCode)
	d.exec(t, `INSERT INTO tax_account_mappings (company_id, category, account_id, effective_from, created_by)
		SELECT c.id, k, a.id, CURRENT_DATE - 30, u.id FROM companies c, accounts a, users u, unnest(ARRAY['VAT_OUTPUT','VAT_INPUT']) k
		WHERE c.code = $1 AND a.code = 'OPS-VAT' AND u.email = 'operator@example.test'`, baseCompanyCode)
	d.exec(t, `INSERT INTO tax_invoice_number_ranges (company_id, prefix, range_start, range_end, next_number, effective_from, created_by)
		SELECT c.id, 'OPS-', 1, 10, 3, CURRENT_DATE - 30, u.id FROM companies c, users u WHERE c.code = $1 AND u.email = 'operator@example.test'`, baseCompanyCode)
	before := map[string]string{}
	for _, table := range baseTaxTables {
		before[table] = d.scalar(t, "SELECT COALESCE(md5(string_agg(t::text, '|' ORDER BY t::text)), '') FROM "+table+" t")
	}

	res, err := runBaseSeed(ctx, d.opts())
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if got, want := taxSteps(res), "tax_code VAT_OUTPUT=reused,tax_code VAT_INPUT=reused,company_tax_identity_primary=reused,tax_account_mapping VAT_OUTPUT=reused,tax_account_mapping VAT_INPUT=reused,tax_invoice_number_range_primary=reused"; got != want {
		t.Errorf("tax steps %q, want %q", got, want)
	}
	for _, table := range baseTaxTables {
		if got := d.scalar(t, "SELECT COALESCE(md5(string_agg(t::text, '|' ORDER BY t::text)), '') FROM "+table+" t"); got != before[table] {
			t.Errorf("%s changed: the seeder must reuse the operator configuration as it is", table)
		}
	}
	if got := d.scalar(t, `SELECT COUNT(*)::text FROM accounts WHERE code IN ('1410', '2110')`); got != "0" {
		t.Errorf("VAT accounts created although the operator mappings exist: %s", got)
	}
}

// TestBaseSeedDBTaxRefusesConflicts: an existing tax configuration the seeded
// one would conflict with aborts the run, rolls it back and stays untouched.
func TestBaseSeedDBTaxRefusesConflicts(t *testing.T) {
	operator := func(t *testing.T, d *baseTestDB) {
		d.exec(t, `INSERT INTO users (email, password_hash) VALUES ('operator@example.test', 'x')`)
	}
	rule := func(t *testing.T, d *baseTestDB, code string, reviewed bool, kinds ...string) {
		reviewedAt := "NULL"
		reviewedBy := "NULL"
		if reviewed {
			reviewedAt, reviewedBy = "NOW()", "id"
		}
		d.exec(t, `INSERT INTO tax_rule_versions (rule_kind, version_code, effective_from, source_url, source_checksum, reviewed_by, reviewed_at, created_by)
			SELECT 'VAT', $1, CURRENT_DATE - 30, 'https://example.test/vat', 'sha256:ops', `+reviewedBy+`, `+reviewedAt+`, id FROM users WHERE email = 'operator@example.test'`, code)
		for _, k := range kinds {
			d.exec(t, `INSERT INTO tax_codes (rule_version_id, code, name, tax_kind) SELECT id, $2, $2, $2 FROM tax_rule_versions WHERE version_code = $1`, code, k)
		}
	}
	cases := []struct {
		name    string
		plant   func(t *testing.T, d *baseTestDB)
		wantErr string
	}{
		{
			name: "partial reviewed operator rule (VAT output only)",
			plant: func(t *testing.T, d *baseTestDB) {
				operator(t, d)
				rule(t, d, "OPS-VAT", true, "VAT_OUTPUT")
			},
			wantErr: "has tax codes for [VAT_OUTPUT] but not for [VAT_INPUT]",
		},
		{
			name: "reviewed operator VAT rule without VAT codes",
			plant: func(t *testing.T, d *baseTestDB) {
				operator(t, d)
				rule(t, d, "OPS-VAT", true)
			},
			wantErr: "reviewed VAT rule version(s) OPS-VAT have no VAT_OUTPUT/VAT_INPUT tax codes effective today and would overlap",
		},
		{
			name: "seeded rule version code that is not reviewed",
			plant: func(t *testing.T, d *baseTestDB) {
				operator(t, d)
				rule(t, d, baseTaxRuleVersionCode, false)
			},
			wantErr: `tax_rule_version_vat (tax_rule_versions) already exists as "not reviewed or not effective today"`,
		},
		{
			name: "company tax identity that has ended",
			plant: func(t *testing.T, d *baseTestDB) {
				operator(t, d)
				d.exec(t, `INSERT INTO companies (code, name) VALUES ($1, $2)`, baseCompanyCode, baseCompanyName)
				d.exec(t, `INSERT INTO company_tax_identities (company_id, legal_name, npwp, nitku, registered_address, effective_from, effective_to, created_by)
					SELECT c.id, 'Old', '1', '2', 'Addr', CURRENT_DATE - 400, CURRENT_DATE - 1, u.id FROM companies c, users u WHERE c.code = $1 AND u.email = 'operator@example.test'`, baseCompanyCode)
			},
			wantErr: "company_tax_identity_primary (company_tax_identities): 1 existing row(s) but none effective today",
		},
		{
			name: "base customer without a tax id",
			plant: func(t *testing.T, d *baseTestDB) {
				operator(t, d)
				d.exec(t, `INSERT INTO companies (code, name) VALUES ($1, $2)`, baseCompanyCode, baseCompanyName)
				d.exec(t, `INSERT INTO customers (code, name, company_id, created_by) SELECT $1, 'Operator customer', c.id, u.id FROM companies c, users u WHERE c.code = $2 AND u.email = 'operator@example.test'`, baseCustomerCode, baseCompanyCode)
			},
			wantErr: "without tax_id",
		},
		{
			name: "exhausted seeded tax invoice number range",
			plant: func(t *testing.T, d *baseTestDB) {
				if _, err := runBaseSeed(context.Background(), d.opts()); err != nil {
					t.Fatalf("seed: %v", err)
				}
				d.exec(t, `UPDATE tax_invoice_number_ranges SET next_number = range_end + 1`)
			},
			wantErr: `tax_invoice_number_range_primary (tax_invoice_number_ranges) already exists as "exhausted or not effective today"`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := newBaseTestDB(t)
			tc.plant(t, d)
			snap := d.snapshot(t)
			if _, err := runBaseSeed(context.Background(), d.opts()); err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want containing %q", err, tc.wantErr)
			}
			if got := d.snapshot(t); fmt.Sprint(got) != fmt.Sprint(snap) {
				t.Fatalf("refused run modified the database")
			}
		})
	}
}
