//go:build integration

package e2e

import (
	"context"
	"math"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/odyssey-erp/odyssey-erp/internal/ar"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
)

// TestRC11ARFormInvoicePostsBalanced replays the J-AR-001 create/post/duplicate
// sequence of tests/staging/certification_test.go at the service layer: the
// form posts only number and total, so the invoice must post a balanced
// journal, keep the given number and refuse a second invoice with that number.
func TestRC11ARFormInvoicePostsBalanced(t *testing.T) {
	p, f := newRC11Fixture(t)
	ctx := context.Background()

	svc := ar.NewService(ar.NewRepository(p))
	svc.SetAccountingService(rc11LedgerHooks(p))
	svc.SetAuditLogger(shared.NewAuditLogger(p))

	number := "RC11-AR-" + f.Suffix
	input := ar.CreateARInvoiceInput{
		CustomerID: f.CustomerID,
		Number:     number,
		Currency:   "IDR",
		Total:      1500,
		DueDate:    time.Now().AddDate(0, 0, 30),
		CreatedBy:  f.AdminID,
	}
	inv, err := svc.CreateARInvoice(ctx, input)
	require.NoError(t, err)
	require.Equal(t, number, inv.Number)

	var subtotal, taxAmount, total float64
	require.NoError(t, p.QueryRow(ctx, `SELECT subtotal::float8, tax_amount::float8, total::float8 FROM ar_invoices WHERE id=$1`, inv.ID).Scan(&subtotal, &taxAmount, &total))
	require.InDelta(t, 1500, total, 0.005)
	assert.InDelta(t, total, subtotal+taxAmount, 0.005, "a total-only invoice must store subtotal = total")

	require.NoError(t, svc.PostARInvoice(ctx, ar.PostARInvoiceInput{InvoiceID: inv.ID, PostedBy: f.AdminID}))

	var journalID int64
	require.NoError(t, p.QueryRow(ctx, `SELECT id FROM journal_entries WHERE source_module='AR.INVOICE' AND memo=$1 AND status='POSTED' ORDER BY id DESC LIMIT 1`, "AR Invoice "+number).Scan(&journalID))
	var debit, credit float64
	require.NoError(t, p.QueryRow(ctx, `SELECT COALESCE(SUM(debit),0)::float8, COALESCE(SUM(credit),0)::float8 FROM journal_lines WHERE je_id=$1`, journalID).Scan(&debit, &credit))
	require.Greater(t, debit, 0.0)
	require.LessOrEqual(t, math.Abs(debit-credit), 0.005, "journal %d is not balanced: debit=%.2f credit=%.2f", journalID, debit, credit)
	rc11AssertJournalActor(t, p, journalID, f.AdminID)

	var audits int
	require.NoError(t, p.QueryRow(ctx, `SELECT COUNT(*) FROM audit_logs WHERE entity='ar_invoice' AND entity_id=$1`, strconv.FormatInt(inv.ID, 10)).Scan(&audits))
	require.GreaterOrEqual(t, audits, 1, "posting must write an ar_invoice audit row")

	// The same number again: refused with a user-facing message, no second row.
	_, err = svc.CreateARInvoice(ctx, input)
	require.ErrorIs(t, err, ar.ErrDuplicateInvoiceNumber)
	require.Equal(t, "Invoice number already exists.", shared.UserSafeMessage(err))
	var rows int
	require.NoError(t, p.QueryRow(ctx, `SELECT COUNT(*) FROM ar_invoices WHERE number=$1`, number).Scan(&rows))
	require.Equal(t, 1, rows)
}

// TestRC11ARPostJournalAuditActor: posting a balanced AR invoice through the
// production ledger wiring (journal audit inside the posting transaction)
// records the posting user on the journal. Subtotal is given explicitly, so
// this isolates the journal actor from the totals fix.
func TestRC11ARPostJournalAuditActor(t *testing.T) {
	p, f := newRC11Fixture(t)
	ctx := context.Background()
	svc := ar.NewService(ar.NewRepository(p))
	svc.SetAccountingService(rc11LedgerHooks(p))
	svc.SetAuditLogger(shared.NewAuditLogger(p))

	number := "RC11-AR-ACT-" + f.Suffix
	inv, err := svc.CreateARInvoice(ctx, ar.CreateARInvoiceInput{CustomerID: f.CustomerID, Number: number, Currency: "IDR", Subtotal: 1500, Total: 1500, DueDate: time.Now().AddDate(0, 0, 30), CreatedBy: f.AdminID})
	require.NoError(t, err)
	require.NoError(t, svc.PostARInvoice(ctx, ar.PostARInvoiceInput{InvoiceID: inv.ID, PostedBy: f.AdminID}))

	var journalID int64
	require.NoError(t, p.QueryRow(ctx, `SELECT id FROM journal_entries WHERE source_module='AR.INVOICE' AND memo=$1 AND status='POSTED'`, "AR Invoice "+number).Scan(&journalID))
	rc11AssertJournalActor(t, p, journalID, f.AdminID)
}
