package integration

import (
	"context"
	"testing"
	"time"

	"github.com/odyssey-erp/odyssey-erp/internal/accounting/journals"
	"github.com/odyssey-erp/odyssey-erp/internal/accounting/mappings"
	"github.com/odyssey-erp/odyssey-erp/internal/accounting/periods"
	"github.com/odyssey-erp/odyssey-erp/internal/ar"
	"github.com/odyssey-erp/odyssey-erp/internal/inventory"
	"github.com/odyssey-erp/odyssey-erp/internal/procurement"
	"github.com/stretchr/testify/require"
)

type ledgerCapture struct{ postings []journals.PostingInput }

func (l *ledgerCapture) PostJournal(_ context.Context, input journals.PostingInput) (journals.JournalEntry, error) {
	l.postings = append(l.postings, input)
	return journals.JournalEntry{}, nil
}

type periodFake struct{ period periods.Period }

func (f periodFake) FindOpenPeriodByDate(context.Context, time.Time) (periods.Period, error) {
	return f.period, nil
}

type mappingFake map[string]int64

func (f mappingFake) Get(_ context.Context, _ string, key string) (mappings.AccountMapping, error) {
	return mappings.AccountMapping{Key: key, AccountID: f[key]}, nil
}

func TestARInvoiceAndCreditNotePostOppositeJournals(t *testing.T) {
	postedAt := time.Date(2026, 7, 30, 9, 0, 0, 0, time.UTC)
	ledger := &ledgerCapture{}
	hooks := NewHooks(ledger, periodFake{period: periods.Period{ID: 1, StartDate: postedAt, EndDate: postedAt, Status: periods.PeriodStatusOpen}}, mappingFake{
		"ar.invoice.ar": 10, "ar.invoice.revenue": 20, "ar.invoice.tax": 30,
	})

	require.NoError(t, hooks.CreateARPostingJournal(context.Background(), &ar.ARInvoice{ID: 8, Number: "INV-8", Subtotal: 100, TaxAmount: 10, Total: 110, PostedAt: &postedAt}))
	require.NoError(t, hooks.CreateARCreditNoteJournal(context.Background(), &ar.ARCreditNote{ID: 9, Number: "CN-9", Subtotal: 20, TaxAmount: 2, Total: 22, PostedAt: &postedAt}))
	require.Len(t, ledger.postings, 2)
	require.Equal(t, []journals.PostingLineInput{{AccountID: 10, Debit: 110}, {AccountID: 20, Credit: 100}, {AccountID: 30, Credit: 10}}, ledger.postings[0].Lines)
	require.Equal(t, []journals.PostingLineInput{{AccountID: 20, Debit: 20}, {AccountID: 10, Credit: 22}, {AccountID: 30, Debit: 2}}, ledger.postings[1].Lines)
	require.NotEqual(t, ledger.postings[0].SourceID, ledger.postings[1].SourceID)
}

func TestReturnInventoryReversesCOGS(t *testing.T) {
	postedAt := time.Date(2026, 7, 30, 9, 0, 0, 0, time.UTC)
	ledger := &ledgerCapture{}
	hooks := NewHooks(ledger, periodFake{period: periods.Period{ID: 1, StartDate: postedAt, EndDate: postedAt, Status: periods.PeriodStatusOpen}}, mappingFake{
		"inventory.adjustment.inventory": 40, "ar.return.cogs": 50,
	})

	require.NoError(t, hooks.HandleInventoryAdjustmentPosted(context.Background(), inventory.AdjustmentPostedEvent{Code: "RDO-1", ProductID: 7, Qty: 3, UnitCost: 25, PostedAt: postedAt, RefModule: "RETURN_DELIVERY", RefID: "4"}))
	require.Len(t, ledger.postings, 1)
	require.Equal(t, []journals.PostingLineInput{{AccountID: 40, Debit: 75}, {AccountID: 50, Credit: 75}}, ledger.postings[0].Lines)
}

func TestDebitNotePostsAPAgainstGRNInventory(t *testing.T) {
	postedAt := time.Date(2026, 7, 30, 9, 0, 0, 0, time.UTC)
	ledger := &ledgerCapture{}
	hooks := NewHooks(ledger, periodFake{period: periods.Period{ID: 1, StartDate: postedAt, EndDate: postedAt, Status: periods.PeriodStatusOpen}}, mappingFake{
		"ap.debit_note.ap": 10, "ap.debit_note.inventory": 20,
	})

	err := hooks.HandleDebitNotePosted(context.Background(), procurement.DebitNotePostedEvent{ID: 9, Number: "DN-9", GRNID: 8, Total: 110, PostedAt: postedAt})

	require.NoError(t, err)
	require.Equal(t, []journals.PostingLineInput{{AccountID: 10, Debit: 110}, {AccountID: 20, Credit: 110}}, ledger.postings[0].Lines)
}
