//go:build integration

package e2e

import (
	"context"
	"log/slog"
	"math"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/odyssey-erp/odyssey-erp/internal/accounting/journals"
	"github.com/odyssey-erp/odyssey-erp/internal/accounting/mappings"
	"github.com/odyssey-erp/odyssey-erp/internal/accounting/periods"
	"github.com/odyssey-erp/odyssey-erp/internal/ap"
	"github.com/odyssey-erp/odyssey-erp/internal/integration"
	"github.com/odyssey-erp/odyssey-erp/internal/procurement"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/tax"
)

// The TestRC11AP* suites replay the J-ARAP-001 AP leg of
// tests/staging/certification_test.go on a clean database: a PO-backed,
// POSTED GRN is invoiced, matched against the fixture's supplier-scoped
// matching policy (never a company-wide one) and posted. They compile against
// tag v0.10.0-rc.10 so the same file shows the failure there.

// rc11APServer builds the AP service and orchestrator the way cmd/odyssey
// wires them: procurement for GRN/PO lookups, ledger hooks, audit and tax.
func rc11APServer(p *pgxpool.Pool) (*ap.Service, *ap.Orchestrator) {
	hooks := integration.NewHooks(journals.NewService(journals.NewRepository(p), nil, nil), periods.NewRepository(p), mappings.NewRepository(p))
	procSvc := procurement.NewService(slog.Default(), procurement.NewRepository(p), nil, nil, shared.NewAuditLogger(p), nil, hooks)
	repo := ap.NewRepository(p)
	svc := ap.NewService(repo, procSvc)
	svc.SetIntegrationHandler(hooks)
	svc.SetAuditLogger(shared.NewAuditLogger(p))
	svc.SetTaxService(tax.NewService(tax.NewRepository(p), tax.ReviewedSchemaValidator{}))
	return svc, ap.NewOrchestrator(ap.NewMatchingService(repo), ap.NewExceptionService(repo), svc, repo)
}

// rc11AssertAPJournal asserts the posted invoice has a balanced
// PROCUREMENT.AP_INVOICE journal with the certification memo and an
// ap_invoice audit row.
func rc11AssertAPJournal(t *testing.T, p *pgxpool.Pool, invoiceID int64, number string) {
	t.Helper()
	ctx := context.Background()
	var journalID int64
	require.NoError(t, p.QueryRow(ctx, `SELECT id FROM journal_entries WHERE source_module='PROCUREMENT.AP_INVOICE' AND memo=$1 AND status='POSTED' ORDER BY id DESC LIMIT 1`, "AP Invoice "+number).Scan(&journalID), "no PROCUREMENT.AP_INVOICE journal for %s", number)
	var debit, credit float64
	require.NoError(t, p.QueryRow(ctx, `SELECT COALESCE(SUM(debit),0)::float8, COALESCE(SUM(credit),0)::float8 FROM journal_lines WHERE je_id=$1`, journalID).Scan(&debit, &credit))
	require.Greater(t, debit, 0.0)
	require.LessOrEqual(t, math.Abs(debit-credit), 0.005, "journal %d is not balanced: debit=%.2f credit=%.2f", journalID, debit, credit)
	var audits int
	require.NoError(t, p.QueryRow(ctx, `SELECT COUNT(*) FROM audit_logs WHERE entity='ap_invoice' AND entity_id=$1`, strconv.FormatInt(invoiceID, 10)).Scan(&audits))
	require.GreaterOrEqual(t, audits, 1, "posting must write an ap_invoice audit row")
}

// rc11APMatchingState reports the latest matching run status and the open
// exception types, for failure messages.
func rc11APMatchingState(t *testing.T, p *pgxpool.Pool, invoiceID int64) (run string, exceptions []string) {
	t.Helper()
	ctx := context.Background()
	_ = p.QueryRow(ctx, `SELECT status FROM ap_matching_runs WHERE ap_invoice_id=$1 ORDER BY id DESC LIMIT 1`, invoiceID).Scan(&run)
	rows, err := p.Query(ctx, `SELECT exception_type || ': ' || COALESCE(reason,'') FROM ap_exceptions WHERE ap_invoice_id=$1 ORDER BY id`, invoiceID)
	require.NoError(t, err)
	defer rows.Close()
	for rows.Next() {
		var s string
		require.NoError(t, rows.Scan(&s))
		exceptions = append(exceptions, s)
	}
	return run, exceptions
}

func TestRC11APGRNInvoicePostsJournal(t *testing.T) {
	p, f := newRC11Fixture(t)
	ctx := context.Background()
	svc, orchestrator := rc11APServer(p)

	number := "RC11-AP-" + f.Suffix
	inv, err := svc.CreateAPInvoiceFromGRN(ctx, ap.CreateAPInvoiceFromGRNInput{GRNID: f.GRNID, DueDate: time.Now().AddDate(0, 0, 30), CreatedBy: f.AdminID, Number: number})
	require.NoError(t, err)

	var linked, lines int
	require.NoError(t, p.QueryRow(ctx, `SELECT COUNT(*) FILTER (WHERE po_line_id=$2), COUNT(*) FROM ap_invoice_lines WHERE ap_invoice_id=$1`, inv.ID, f.POLineID).Scan(&linked, &lines))
	assert.Equal(t, 1, lines)
	assert.Equal(t, lines, linked, "every GRN invoice line must carry the PO line it bills")

	// The matching run the worker (or, after rc.11, the create request) runs.
	require.NoError(t, orchestrator.ProcessInvoice(ctx, inv.ID, f.AdminID))
	run, exceptions := rc11APMatchingState(t, p, inv.ID)
	t.Logf("matching run=%q exceptions=%v", run, exceptions)

	// The user's "post" click: only needed when matching did not auto-post.
	var status string
	require.NoError(t, p.QueryRow(ctx, `SELECT status FROM ap_invoices WHERE id=$1`, inv.ID).Scan(&status))
	if status == string(ap.APStatusDraft) {
		require.NoError(t, svc.PostAPInvoice(ctx, ap.PostAPInvoiceInput{InvoiceID: inv.ID, PostedBy: f.AdminID}), "matching run=%q exceptions=%v", run, exceptions)
	}
	require.NoError(t, p.QueryRow(ctx, `SELECT status FROM ap_invoices WHERE id=$1`, inv.ID).Scan(&status))
	require.Equal(t, string(ap.APStatusPosted), status, "matching run=%q exceptions=%v", run, exceptions)

	rc11AssertAPJournal(t, p, inv.ID, number)
}

func TestRC11APGRNWithoutPOIsRefused(t *testing.T) {
	p, f := newRC11Fixture(t)
	ctx := context.Background()
	svc, _ := rc11APServer(p)

	var grnID int64
	require.NoError(t, p.QueryRow(ctx, `INSERT INTO grns(number,po_id,supplier_id,warehouse_id,status,company_id) VALUES($1,NULL,$2,$3,'POSTED',$4) RETURNING id`, "GRN-NOPO-"+f.Suffix, f.SupplierID, f.WarehouseID, f.CompanyID).Scan(&grnID))
	_, err := p.Exec(ctx, `INSERT INTO grn_lines(grn_id,product_id,qty,unit_cost) VALUES($1,$2,10,100)`, grnID, f.ProductID)
	require.NoError(t, err)

	_, err = svc.CreateAPInvoiceFromGRN(ctx, ap.CreateAPInvoiceFromGRNInput{GRNID: grnID, DueDate: time.Now().AddDate(0, 0, 30), CreatedBy: f.AdminID, Number: "RC11-AP-NOPO-" + f.Suffix})
	assert.Error(t, err, "a GRN without a purchase order must not be invoiced")

	var rows int
	require.NoError(t, p.QueryRow(ctx, `SELECT COUNT(*) FROM ap_invoices WHERE grn_id=$1`, grnID).Scan(&rows))
	require.Zero(t, rows, "no DRAFT invoice may be written for a PO-less GRN")
}
