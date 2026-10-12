//go:build integration

package e2e

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/odyssey-erp/odyssey-erp/internal/ap"
	"github.com/odyssey-erp/odyssey-erp/internal/ar"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/tax"
)

// newRC11StagingSeededDB creates an empty database on the PG_DSN server,
// migrates it and runs the staging seeder (scripts/seed/staging, base
// fixtures) against it, exactly as the staging rebuild does. The database is
// dropped when the test ends. It returns a pool on the new database.
func newRC11StagingSeededDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("PG_DSN")
	if dsn == "" {
		t.Skip("PG_DSN is required for the RC11 integration suites")
	}
	ctx := context.Background()
	cfg, err := pgx.ParseConfig(dsn)
	require.NoError(t, err)
	maintenance := cfg.Copy()
	maintenance.Database = "postgres"
	conn, err := pgx.ConnectConfig(ctx, maintenance)
	require.NoError(t, err)
	defer func() { _ = conn.Close(ctx) }()

	name := fmt.Sprintf("rc11_taxseed_%d", time.Now().UnixNano())
	_, err = conn.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize())
	require.NoError(t, err)
	t.Cleanup(func() {
		c, err := pgx.ConnectConfig(context.Background(), maintenance)
		if err != nil {
			t.Logf("cleanup connect: %v", err)
			return
		}
		defer func() { _ = c.Close(context.Background()) }()
		if _, err := c.Exec(context.Background(), "DROP DATABASE IF EXISTS "+pgx.Identifier{name}.Sanitize()+" WITH (FORCE)"); err != nil {
			t.Logf("drop %s: %v", name, err)
		}
	})

	u, err := url.Parse(dsn)
	require.NoError(t, err)
	u.Path = "/" + name
	dbDSN := u.String()
	applyAllMigrations(t, dbDSN)

	_, file, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(file), "..", "..")
	bin := filepath.Join(t.TempDir(), "seed-staging")
	build := exec.Command("go", "build", "-o", bin, "./scripts/seed/staging")
	build.Dir = root
	out, err := build.CombinedOutput()
	require.NoError(t, err, "build the staging seeder: %s", out)
	seed := exec.Command(bin, "--dsn", dbDSN, "--confirm-staging", name+"@"+cfg.Host)
	seed.Env = append(os.Environ(), "APP_ENV=staging")
	out, err = seed.CombinedOutput()
	require.NoError(t, err, "run the staging seeder: %s", out)
	t.Logf("staging seeder:\n%s", out)

	p, err := pgxpool.New(ctx, dbDSN)
	require.NoError(t, err)
	t.Cleanup(p.Close)
	return p
}

// TestRC11StagingSeedLetsARAndAPCaptureTax (FIND-028): on a database rebuilt
// from migrations and seeded with scripts/seed/staging only, posting an AR
// invoice and an AP invoice through the production service wiring (ledger
// hooks, audit and the tax service) completes without
// tax.ErrConfiguration and records the tax document of each. Without the
// seeded reviewed tax configuration the post commits and the tax capture
// then fails with "tax: reviewed configuration is incomplete".
func TestRC11StagingSeedLetsARAndAPCaptureTax(t *testing.T) {
	p := newRC11StagingSeededDB(t)
	ctx := context.Background()
	id := func(sql string, args ...any) int64 {
		t.Helper()
		var v int64
		require.NoError(t, p.QueryRow(ctx, sql, args...).Scan(&v), sql)
		return v
	}
	adminID := id(`SELECT id FROM users WHERE email = $1`, "admin@staging.odyssey.local")
	companyID := id(`SELECT id FROM companies WHERE code = 'ODY-01'`)
	customerID := id(`SELECT id FROM customers WHERE company_id = $1 AND code = 'CUST-STG-01'`, companyID)
	grnID := id(`SELECT g.id FROM grns g WHERE g.company_id = $1 AND g.status = 'POSTED' AND g.note = 'v0.10-core staging certification GRN'
		AND NOT EXISTS (SELECT 1 FROM ap_invoices a WHERE a.grn_id = g.id) ORDER BY g.id DESC LIMIT 1`, companyID)
	taxService := tax.NewService(tax.NewRepository(p), tax.ReviewedSchemaValidator{})

	// AR, wired as cmd/odyssey wires it.
	arService := ar.NewService(ar.NewRepository(p))
	arService.SetAccountingService(rc11LedgerHooks(p))
	arService.SetAuditLogger(shared.NewAuditLogger(p))
	arService.SetTaxService(taxService)
	arInvoice, err := arService.CreateARInvoice(ctx, ar.CreateARInvoiceInput{
		CustomerID: customerID, Number: "RC11-TAX-AR-1", Currency: "IDR", Total: 100000,
		DueDate: time.Now().AddDate(0, 0, 30), CreatedBy: adminID,
	})
	require.NoError(t, err)
	err = arService.PostARInvoice(ctx, ar.PostARInvoiceInput{InvoiceID: arInvoice.ID, PostedBy: adminID})
	require.NoError(t, err, "AR posting must complete, including the tax capture")
	require.NotErrorIs(t, err, tax.ErrConfiguration)
	var arDocs int
	var faktur string
	require.NoError(t, p.QueryRow(ctx, `SELECT COUNT(*) FROM tax_documents WHERE source_type = 'AR_INVOICE' AND source_id = $1 AND company_id = $2`, arInvoice.ID, companyID).Scan(&arDocs))
	require.Equal(t, 1, arDocs, "the AR invoice must have its tax document")
	require.NoError(t, p.QueryRow(ctx, `SELECT COALESCE(faktur_status, '') FROM ar_invoices WHERE id = $1`, arInvoice.ID).Scan(&faktur))
	require.Equal(t, "ISSUED", faktur)

	// AP, wired as cmd/odyssey wires it: the GRN invoice is matched by the
	// orchestrator and posted.
	apService, orchestrator := rc11APServer(p)
	apInvoice, err := apService.CreateAPInvoiceFromGRN(ctx, ap.CreateAPInvoiceFromGRNInput{GRNID: grnID, DueDate: time.Now().AddDate(0, 0, 30), CreatedBy: adminID, Number: "RC11-TAX-AP-1"})
	require.NoError(t, err)
	require.NoError(t, orchestrator.ProcessInvoice(ctx, apInvoice.ID, adminID))
	var status string
	require.NoError(t, p.QueryRow(ctx, `SELECT status FROM ap_invoices WHERE id = $1`, apInvoice.ID).Scan(&status))
	if status == string(ap.APStatusDraft) {
		err = apService.PostAPInvoice(ctx, ap.PostAPInvoiceInput{InvoiceID: apInvoice.ID, PostedBy: adminID})
		require.NoError(t, err, "AP posting must complete, including the tax capture")
		require.NoError(t, p.QueryRow(ctx, `SELECT status FROM ap_invoices WHERE id = $1`, apInvoice.ID).Scan(&status))
	}
	run, exceptions := rc11APMatchingState(t, p, apInvoice.ID)
	require.Equal(t, string(ap.APStatusPosted), status, "matching run=%q exceptions=%v", run, exceptions)
	require.Empty(t, exceptions, "posting must leave no exception (a failed tax capture is recorded as one)")
	var apDocs int
	require.NoError(t, p.QueryRow(ctx, `SELECT COUNT(*) FROM tax_documents WHERE source_type = 'AP_INVOICE' AND source_id = $1 AND company_id = $2`, apInvoice.ID, companyID).Scan(&apDocs))
	require.Equal(t, 1, apDocs, "the AP invoice must have its tax document")
}
