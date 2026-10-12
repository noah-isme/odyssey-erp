//go:build integration

package e2e

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/odyssey-erp/odyssey-erp/internal/tax"
)

// rc11SeedTaxConfig gives fixture f a complete reviewed tax configuration:
// a reviewed VAT rule version with VAT_OUTPUT and VAT_INPUT codes, the
// company's tax identity, VAT account mappings, a tax invoice number range,
// an OPEN accounting period covering today and tax ids on the fixture
// customer and supplier. It returns the rule version and VAT_OUTPUT code.
// It runs only on a scratch database (the rule version is global).
func rc11SeedTaxConfig(t *testing.T, p *pgxpool.Pool, f rc11Fixture) (ruleID, outputCodeID int64) {
	t.Helper()
	ctx := context.Background()
	exec := func(sql string, args ...any) {
		t.Helper()
		_, err := p.Exec(ctx, sql, args...)
		require.NoError(t, err, sql)
	}
	require.NoError(t, p.QueryRow(ctx, `INSERT INTO tax_rule_versions (rule_kind, version_code, effective_from, source_url, source_checksum, reviewed_by, reviewed_at, created_by)
		VALUES ('VAT', 'RC11-VAT', CURRENT_DATE - 30, 'urn:test', 'test', $1, NOW(), $1) RETURNING id`, f.AdminID).Scan(&ruleID))
	require.NoError(t, p.QueryRow(ctx, `INSERT INTO tax_codes (rule_version_id, code, name, tax_kind) VALUES ($1, 'OUT', 'VAT out', 'VAT_OUTPUT') RETURNING id`, ruleID).Scan(&outputCodeID))
	exec(`INSERT INTO tax_codes (rule_version_id, code, name, tax_kind) VALUES ($1, 'IN', 'VAT in', 'VAT_INPUT')`, ruleID)
	exec(`INSERT INTO company_tax_identities (company_id, legal_name, npwp, nitku, registered_address, effective_from, created_by)
		VALUES ($1, 'RC11', '1', '2', 'Addr', CURRENT_DATE - 30, $2)`, f.CompanyID, f.AdminID)
	exec(`INSERT INTO tax_account_mappings (company_id, category, account_id, effective_from, created_by)
		SELECT $1, 'VAT_OUTPUT', id, CURRENT_DATE - 30, $2 FROM accounts WHERE code = '2100'`, f.CompanyID, f.AdminID)
	exec(`INSERT INTO tax_account_mappings (company_id, category, account_id, effective_from, created_by)
		SELECT $1, 'VAT_INPUT', id, CURRENT_DATE - 30, $2 FROM accounts WHERE code = '1300'`, f.CompanyID, f.AdminID)
	exec(`INSERT INTO tax_invoice_number_ranges (company_id, prefix, range_start, range_end, next_number, effective_from, created_by)
		VALUES ($1, 'RC11-', 1, 9999, 1, CURRENT_DATE - 30, $2)`, f.CompanyID, f.AdminID)
	exec(`INSERT INTO accounting_periods (period_id, company_id, name, start_date, end_date, status)
		SELECT id, $2, code, start_date, end_date, 'OPEN' FROM periods WHERE id = $1 ON CONFLICT DO NOTHING`, f.PeriodID, f.CompanyID)
	exec(`UPDATE customers SET tax_id = '0098765432101000' WHERE id = $1`, f.CustomerID)
	exec(`UPDATE suppliers SET tax_id = '0087654321012000' WHERE id = $1`, f.SupplierID)
	return ruleID, outputCodeID
}

// TestRC11TaxRepositoryWritesWithCompleteConfiguration (FIND-038): with a
// complete reviewed tax configuration, the tax repository's capture
// (RecordARInvoice, RecordAPInvoice), CancelDocument and ReplaceDocument
// must write their events, ledger entries and audit rows. Each of them sent
// several parameterised statements in one Exec, which PostgreSQL refuses
// ("cannot insert multiple commands into a prepared statement", SQLSTATE
// 42601), so none could succeed on a real database.
func TestRC11TaxRepositoryWritesWithCompleteConfiguration(t *testing.T) {
	dsn, _, _ := newRC11ScratchDB(t)
	t.Setenv("PG_DSN", dsn)
	p, f := newRC11Fixture(t)
	ctx := context.Background()
	ruleID, outputCodeID := rc11SeedTaxConfig(t, p, f)
	repo := tax.NewRepository(p)
	svc := tax.NewService(repo, nil)
	count := func(sql string, args ...any) int {
		t.Helper()
		var n int
		require.NoError(t, p.QueryRow(ctx, sql, args...).Scan(&n), sql)
		return n
	}
	id := func(sql string, args ...any) int64 {
		t.Helper()
		var v int64
		require.NoError(t, p.QueryRow(ctx, sql, args...).Scan(&v), sql)
		return v
	}
	arInvoice := func(number string) int64 {
		return id(`INSERT INTO ar_invoices (number, customer_id, currency, subtotal, tax_amount, total, status, due_at, posted_at, posted_by)
			VALUES ($1, $2, 'IDR', 1000, 110, 1110, 'POSTED', NOW() + INTERVAL '30 days', NOW(), $3) RETURNING id`, number+"-"+f.Suffix, f.CustomerID, f.AdminID)
	}
	apInvoice := func(number string) int64 {
		return id(`INSERT INTO ap_invoices (number, supplier_id, company_id, currency, subtotal, tax_amount, total, status, posted_at, posted_by)
			VALUES ($1, $2, $3, 'IDR', 1000, 110, 1110, 'POSTED', NOW(), $4) RETURNING id`, number+"-"+f.Suffix, f.SupplierID, f.CompanyID, f.AdminID)
	}

	// Capture (internal/tax/repository.go capture, the post-commit step of
	// AR and AP posting).
	ar1, ap1 := arInvoice("RC11-TAX-AR1"), apInvoice("RC11-TAX-AP1")
	assert.NoError(t, svc.RecordARInvoice(ctx, ar1, f.AdminID), "AR capture")
	assert.NoError(t, svc.RecordAPInvoice(ctx, ap1, f.AdminID), "AP capture")
	for _, src := range []struct {
		typ string
		id  int64
	}{{"AR_INVOICE", ar1}, {"AP_INVOICE", ap1}} {
		assert.Equal(t, 1, count(`SELECT COUNT(*) FROM tax_documents d
			JOIN tax_document_events e ON e.tax_document_id = d.id AND e.event_type = 'ISSUED'
			JOIN tax_ledger_entries l ON l.tax_document_id = d.id AND l.source_type = d.source_type
			JOIN tax_audit_events a ON a.entity_type = 'TAX_DOCUMENT' AND a.entity_id = d.id AND a.action = 'ISSUED'
			WHERE d.source_type = $1 AND d.source_id = $2`, src.typ, src.id), "%s %d: tax document with its ISSUED event, ledger entry and audit row", src.typ, src.id)
	}

	// Documents for cancel and replace are planted directly, so these two
	// paths are exercised whatever the capture does.
	taxPeriodID := id(`INSERT INTO tax_periods (company_id, accounting_period_id)
		SELECT $1, id FROM accounting_periods WHERE period_id = $2
		ON CONFLICT (company_id, accounting_period_id) DO UPDATE SET company_id = EXCLUDED.company_id RETURNING id`, f.CompanyID, f.PeriodID)
	accountID := id(`SELECT id FROM accounts WHERE code = '2100'`)
	plant := func(sourceType, direction string, sourceID int64, number string) int64 {
		t.Helper()
		doc := id(`INSERT INTO tax_documents (company_id, tax_period_id, source_type, source_id, source_number, source_posted_at, document_kind, direction, tax_number, issue_date,
				counterparty_name, counterparty_tax_id, taxable_base, vat_amount, gross_amount, sign, tax_code_id, rule_version_id, source_hash, created_by)
			VALUES ($1, $2, $3, $4, $5, NOW(), 'INVOICE', $6, $5, CURRENT_DATE, 'Counterparty', '0098765432101000', 1000, 110, 1110, 1, $7, $8, 'planted', $9) RETURNING id`,
			f.CompanyID, taxPeriodID, sourceType, sourceID, number+"-"+f.Suffix, direction, outputCodeID, ruleID, f.AdminID)
		_, err := p.Exec(ctx, `INSERT INTO tax_ledger_entries (company_id, tax_period_id, category, tax_document_id, account_id, source_type, source_id, source_date, taxable_base, tax_amount, sign)
			VALUES ($1, $2, 'VAT_OUTPUT', $3, $4, $5, $6, CURRENT_DATE, 1000, 110, 1)`, f.CompanyID, taxPeriodID, doc, accountID, sourceType, sourceID)
		require.NoError(t, err)
		return doc
	}

	// CancelDocument.
	ap2 := apInvoice("RC11-TAX-AP2")
	cancelled := plant("AP_INVOICE", "INPUT", ap2, "RC11-TAX-DOC-AP2")
	assert.NoError(t, repo.CancelDocument(ctx, cancelled, f.AdminID, "rc11 cancel"), "CancelDocument")
	assert.Equal(t, 1, count(`SELECT COUNT(*) FROM tax_document_events WHERE tax_document_id = $1 AND event_type = 'CANCELLED' AND reason = 'rc11 cancel'`, cancelled), "CANCELLED event")
	assert.Equal(t, 1, count(`SELECT COUNT(*) FROM tax_audit_events WHERE entity_type = 'TAX_DOCUMENT' AND entity_id = $1 AND action = 'CANCELLED'`, cancelled), "CANCELLED audit row")
	assert.Equal(t, 1, count(`SELECT COUNT(*) FROM tax_ledger_entries WHERE tax_document_id = $1 AND source_type = 'AP_INVOICE_CANCEL' AND sign = -1`, cancelled), "reversing ledger entry")

	// ReplaceDocument of an AR document (also updates both invoices).
	ar2, ar3 := arInvoice("RC11-TAX-AR2"), arInvoice("RC11-TAX-AR3")
	original := plant("AR_INVOICE", "OUTPUT", ar2, "RC11-TAX-DOC-AR2")
	replacement := plant("AR_INVOICE", "OUTPUT", ar3, "RC11-TAX-DOC-AR3")
	_, err := p.Exec(ctx, `UPDATE ar_invoices SET tax_document_id = CASE id WHEN $1 THEN $3::bigint ELSE $4::bigint END, faktur_status = 'ISSUED' WHERE id IN ($1, $2)`, ar2, ar3, original, replacement)
	require.NoError(t, err)
	assert.NoError(t, repo.ReplaceDocument(ctx, original, replacement, f.AdminID, "rc11 replace"), "ReplaceDocument")
	assert.Equal(t, 1, count(`SELECT COUNT(*) FROM tax_document_events WHERE tax_document_id = $1 AND event_type = 'REPLACED' AND replacement_document_id = $2`, original, replacement), "REPLACED event")
	assert.Equal(t, 1, count(`SELECT COUNT(*) FROM tax_audit_events WHERE entity_type = 'TAX_DOCUMENT' AND entity_id = $1 AND action = 'REPLACED'`, original), "REPLACED audit row")
	assert.Equal(t, 1, count(`SELECT COUNT(*) FROM tax_ledger_entries WHERE tax_document_id = $1 AND source_type = 'AR_INVOICE_REPLACE' AND sign = -1`, original), "reversing ledger entry")
	assert.Equal(t, 1, count(`SELECT COUNT(*) FROM ar_invoices WHERE id = $1 AND faktur_status = 'REPLACED'`, ar2), "original invoice marked REPLACED")
	assert.Equal(t, 1, count(`SELECT COUNT(*) FROM ar_invoices WHERE id = $1 AND replacement_of_tax_document_id = $2`, ar3, original), "replacement invoice records the document it replaces")
}
