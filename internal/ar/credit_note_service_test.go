package ar

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type creditNoteMemoryRepo struct {
	*memoryARRepo
	notes     map[int64]*ARCreditNote
	lines     map[int64][]ARCreditNoteLine
	available float64
	nextID    int64
}

func newCreditNoteMemoryRepo() *creditNoteMemoryRepo {
	return &creditNoteMemoryRepo{memoryARRepo: newMemoryARRepo(), notes: make(map[int64]*ARCreditNote), lines: make(map[int64][]ARCreditNoteLine)}
}

func (r *creditNoteMemoryRepo) CreateARCreditNote(_ context.Context, input CreateARCreditNoteInput) (*ARCreditNote, error) {
	r.nextID++
	note := &ARCreditNote{ID: r.nextID, Number: input.Number, CustomerID: input.CustomerID, ARInvoiceID: input.ARInvoiceID, ReturnDeliveryOrderID: input.ReturnDeliveryOrderID, Currency: input.Currency, Reason: input.Reason, Status: ARCreditNoteStatusDraft, CreatedBy: input.CreatedBy}
	for i, inputLine := range input.Lines {
		subtotal := inputLine.Quantity * inputLine.UnitPrice * (1 - inputLine.DiscountPct/100)
		tax := subtotal * inputLine.TaxPct / 100
		line := ARCreditNoteLine{ID: int64(i + 1), ARCreditNoteID: note.ID, ARInvoiceLineID: inputLine.ARInvoiceLineID, ReturnDeliveryOrderLineID: inputLine.ReturnDeliveryOrderLineID, ProductID: inputLine.ProductID, Description: inputLine.Description, Quantity: inputLine.Quantity, UnitPrice: inputLine.UnitPrice, DiscountPct: inputLine.DiscountPct, TaxPct: inputLine.TaxPct, Subtotal: subtotal, TaxAmount: tax, Total: subtotal + tax}
		r.lines[note.ID] = append(r.lines[note.ID], line)
		note.Subtotal += subtotal
		note.TaxAmount += tax
		note.Total += subtotal + tax
	}
	r.notes[note.ID] = note
	return note, nil
}

func (r *creditNoteMemoryRepo) GetARCreditNote(_ context.Context, id int64) (*ARCreditNote, error) {
	return r.notes[id], nil
}

func (r *creditNoteMemoryRepo) GetARCreditNoteWithDetails(_ context.Context, id int64) (*ARCreditNoteWithDetails, error) {
	return &ARCreditNoteWithDetails{ARCreditNote: *r.notes[id], Lines: r.lines[id]}, nil
}

func (r *creditNoteMemoryRepo) ListARCreditNotes(_ context.Context, _ ListARCreditNotesRequest) ([]ARCreditNote, error) {
	var notes []ARCreditNote
	for _, note := range r.notes {
		notes = append(notes, *note)
	}
	return notes, nil
}

func (r *creditNoteMemoryRepo) PostARCreditNote(_ context.Context, id, _ int64, postedBy int64, amount float64) error {
	note := r.notes[id]
	note.Status = ARCreditNoteStatusPosted
	note.PostedBy = &postedBy
	r.available -= amount
	return nil
}

func (r *creditNoteMemoryRepo) VoidARCreditNote(_ context.Context, id, voidedBy int64, reason string) error {
	note := r.notes[id]
	note.Status = ARCreditNoteStatusVoid
	note.VoidedBy = &voidedBy
	note.VoidReason = reason
	return nil
}

func (r *creditNoteMemoryRepo) GenerateCreditNoteNumber(context.Context) (string, error) {
	return "CN-TEST-001", nil
}

func (r *creditNoteMemoryRepo) GetInvoiceCreditAvailable(context.Context, int64) (float64, error) {
	return r.available, nil
}

type returnDeliveryFake struct{ info ReturnDeliveryInfo }

func (f returnDeliveryFake) GetReturnForCreditNote(context.Context, int64) (*ReturnDeliveryInfo, error) {
	return &f.info, nil
}

type accountingFake struct{ creditNotes []ARCreditNote }

func (f *accountingFake) CreateARPostingJournal(context.Context, *ARInvoice) error { return nil }
func (f *accountingFake) CreateARCreditNoteJournal(_ context.Context, note *ARCreditNote) error {
	f.creditNotes = append(f.creditNotes, *note)
	return nil
}

func TestCreateAndPostCreditNoteFromConfirmedReturn(t *testing.T) {
	ctx := context.Background()
	repo := newCreditNoteMemoryRepo()
	repo.available = 1100
	service := NewService(repo)
	invoice, err := service.CreateARInvoice(ctx, CreateARInvoiceInput{CustomerID: 7, DeliveryOrderID: 55, Number: "INV-55", Currency: "IDR", Subtotal: 1000, TaxAmount: 100, Total: 1100, Lines: []CreateARInvoiceLineInput{{DeliveryOrderLineID: 77, ProductID: 9, Description: "Returned item", Quantity: 10, UnitPrice: 100, TaxPct: 10}}})
	require.NoError(t, err)
	repo.invoiceLines[invoice.ID][0].DeliveryOrderLineID = 77
	require.NoError(t, repo.PostARInvoice(ctx, invoice.ID, 3))

	service.SetReturnDeliveryService(returnDeliveryFake{info: ReturnDeliveryInfo{ID: 5, OriginalDeliveryOrderID: 55, CustomerID: 7, Status: "CONFIRMED", Lines: []ReturnDeliveryLineInfo{{ID: 8, DeliveryOrderLineID: 77, ProductID: 9, Quantity: 2}}}})
	ledger := &accountingFake{}
	service.SetAccountingService(ledger)

	note, err := service.CreateARCreditNoteFromReturn(ctx, CreateARCreditNoteFromReturnInput{ReturnDeliveryOrderID: 5, Reason: "Damaged", CreatedBy: 4})
	require.NoError(t, err)
	require.Equal(t, 220.0, note.Total)
	require.NoError(t, service.PostARCreditNote(ctx, PostARCreditNoteInput{CreditNoteID: note.ID, PostedBy: 4}))
	require.Equal(t, ARCreditNoteStatusPosted, note.Status)
	require.Equal(t, 880.0, repo.available)
	require.Len(t, ledger.creditNotes, 1)
	require.Equal(t, 220.0, ledger.creditNotes[0].Total)
}

func TestPostCreditNoteRejectsAmountAboveRemainingCredit(t *testing.T) {
	ctx := context.Background()
	repo := newCreditNoteMemoryRepo()
	repo.available = 100
	repo.invoices[1] = &ARInvoice{ID: 1, Status: ARStatusPosted}
	repo.notes[1] = &ARCreditNote{ID: 1, ARInvoiceID: 1, Total: 120, Status: ARCreditNoteStatusDraft}
	service := NewService(repo)

	err := service.PostARCreditNote(ctx, PostARCreditNoteInput{CreditNoteID: 1, PostedBy: 4})
	require.ErrorIs(t, err, ErrCreditExceedsBalance)
	require.Equal(t, ARCreditNoteStatusDraft, repo.notes[1].Status)
}
