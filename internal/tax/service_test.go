package tax

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"
)

type fakeStore struct {
	recap     []RecapLine
	locked    bool
	schema    ExportSchema
	records   []ExportRecord
	exportID  int64
	sources   []PostedSource
	captured  []string
	pending   []PendingCapture
	completed []int64
	cancelled []int64
	replaced  [][2]int64
	built     bool
}

func (f *fakeStore) CaptureARInvoice(context.Context, int64, int64) (Document, error) {
	f.captured = append(f.captured, "AR_INVOICE")
	return Document{}, nil
}
func (f *fakeStore) CaptureARCreditNote(context.Context, int64, int64) (Document, error) {
	f.captured = append(f.captured, "AR_CREDIT_NOTE")
	return Document{}, nil
}
func (f *fakeStore) CaptureAPInvoice(context.Context, int64, int64) (Document, error) {
	f.captured = append(f.captured, "AP_INVOICE")
	return Document{}, nil
}
func (f *fakeStore) CaptureAPDebitNote(context.Context, int64, int64) (Document, error) {
	f.captured = append(f.captured, "AP_DEBIT_NOTE")
	return Document{}, nil
}
func (f *fakeStore) CaptureAPPayment(context.Context, int64, int64) ([]Withholding, error) {
	f.captured = append(f.captured, "AP_PAYMENT")
	return nil, nil
}
func (f *fakeStore) CancelDocument(_ context.Context, id, _ int64, _ string) error {
	f.cancelled = append(f.cancelled, id)
	return nil
}
func (*fakeStore) CancelSource(context.Context, string, int64, int64, string) error { return nil }
func (f *fakeStore) ReplaceDocument(_ context.Context, id, replacementID, _ int64, _ string) error {
	f.replaced = append(f.replaced, [2]int64{id, replacementID})
	return nil
}
func (*fakeStore) ListDocuments(context.Context, int64, int64) ([]Document, error) { return nil, nil }
func (*fakeStore) ListPeriods(context.Context, int64) ([]Period, error)            { return nil, nil }
func (f *fakeStore) ListPostedSources(context.Context, int64, int64) ([]PostedSource, error) {
	f.built = true
	return f.sources, nil
}
func (f *fakeStore) Recap(context.Context, int64, int64) ([]RecapLine, error) { return f.recap, nil }
func (f *fakeStore) LockPeriod(context.Context, int64, int64, int64) error {
	f.locked = true
	return nil
}
func (f *fakeStore) LoadExport(context.Context, int64, int64, string) (ExportSchema, []ExportRecord, error) {
	return f.schema, f.records, nil
}
func (f *fakeStore) RecordExport(_ context.Context, _, _, _ int64, _ string, _ int, _ Money, _ Money, _ int64) (int64, error) {
	return f.exportID, nil
}
func (f *fakeStore) PendingCaptures(context.Context, int) ([]PendingCapture, error) {
	return f.pending, nil
}
func (f *fakeStore) CompleteCapture(_ context.Context, id int64) error {
	f.completed = append(f.completed, id)
	return nil
}
func (*fakeStore) FailCapture(context.Context, int64, error) error { return nil }

func reviewedSchema() ExportSchema {
	body := "official-xsd-artifact"
	digest := sha256.Sum256([]byte(body))
	return ExportSchema{ID: 7, Kind: "CORETAX_OUTPUT_VAT", Version: "2026.01", MediaType: "application/xml", Body: body, OfficialChecksum: hex.EncodeToString(digest[:])}
}

func TestExportNetsCreditNotesAndRecordsExactTotals(t *testing.T) {
	f := &fakeStore{schema: reviewedSchema(), exportID: 9, records: []ExportRecord{
		{TaxNumber: "010", DocumentNumber: "INV-1", CounterpartyName: "Buyer", CounterpartyTaxID: "123", IssueDate: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), TaxableBase: 1000, TaxAmount: 110, Sign: 1},
		{TaxNumber: "011", DocumentNumber: "CN-1", CounterpartyName: "Buyer", CounterpartyTaxID: "123", IssueDate: time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC), TaxableBase: 250, TaxAmount: 28, Sign: -1},
	}}
	got, err := NewService(f, ReviewedSchemaValidator{}).Export(context.Background(), 1, 2, 3, "CORETAX_OUTPUT_VAT")
	if err != nil {
		t.Fatal(err)
	}
	if got.TaxableBase != 750 || got.TaxAmount != 82 || got.Count != 2 || got.ID != 9 {
		t.Fatalf("unexpected export totals: %+v", got)
	}
	if !strings.Contains(got.Content, "<TaxableBase>-250</TaxableBase>") {
		t.Fatalf("credit note sign missing: %s", got.Content)
	}
	if strings.Contains(got.Content, "<Sign>") {
		t.Fatalf("unreviewed Sign element emitted: %s", got.Content)
	}
}

func TestExportUsesReviewedDeclarationAndOptionalSign(t *testing.T) {
	schema := reviewedSchema()
	schema.XMLDeclaration = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`
	schema.IncludeSignElement = true
	f := &fakeStore{schema: schema, exportID: 9, records: []ExportRecord{{DocumentNumber: "CN-1", IssueDate: time.Now(), Sign: -1}}}
	got, err := NewService(f, ReviewedSchemaValidator{}).Export(context.Background(), 1, 2, 3, "CORETAX_OUTPUT_VAT")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got.Content, schema.XMLDeclaration) || !strings.Contains(got.Content, "<Sign>-1</Sign>") {
		t.Fatalf("reviewed XML options not applied: %s", got.Content)
	}
}

func TestLockRequiresRupiahReconciliation(t *testing.T) {
	f := &fakeStore{recap: []RecapLine{{Difference: 1}}}
	s := NewService(f, nil)
	if !errors.Is(s.Lock(context.Background(), 1, 2, 3), ErrReconciliation) {
		t.Fatal("expected reconciliation error")
	}
	if f.locked {
		t.Fatal("unreconciled period was locked")
	}
	f.recap[0].Difference = 0
	if err := s.Lock(context.Background(), 1, 2, 3); err != nil {
		t.Fatal(err)
	}
	if !f.locked {
		t.Fatal("reconciled period was not locked")
	}
}

func TestPartialPaymentWithholdingAndRounding(t *testing.T) {
	base := prorateBase(605, 1000, 1210)
	if base != 500 {
		t.Fatalf("base=%d want 500", base)
	}
	if got := calculateWithholding(base, 200); got != 10 {
		t.Fatalf("PPh23=%d want 10", got)
	}
	if got := calculateWithholding(333, 150); got != 5 {
		t.Fatalf("rounding=%d want 5", got)
	}
}

func TestReviewedSchemaChecksumIsReleaseGate(t *testing.T) {
	s := reviewedSchema()
	s.OfficialChecksum = "bad"
	if err := (ReviewedSchemaValidator{}).Validate(s, []byte("<TaxExport/>")); err == nil {
		t.Fatal("expected checksum failure")
	}
}

func TestBuildPeriodCapturesEveryPostedSourceType(t *testing.T) {
	f := &fakeStore{sources: []PostedSource{{"AR_INVOICE", 1}, {"AR_CREDIT_NOTE", 2}, {"AP_INVOICE", 3}, {"AP_DEBIT_NOTE", 4}, {"AP_PAYMENT", 5}}}
	if err := NewService(f, nil).BuildPeriod(context.Background(), 1, 2, 3); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(f.captured, ","); got != "AR_INVOICE,AR_CREDIT_NOTE,AP_INVOICE,AP_DEBIT_NOTE,AP_PAYMENT" {
		t.Fatalf("captured %s", got)
	}
}

func TestProcessPendingCompletesDurableCapture(t *testing.T) {
	f := &fakeStore{pending: []PendingCapture{{ID: 8, SourceType: "AR_CREDIT_NOTE", SourceID: 4, ActorID: 3}}}
	if err := NewService(f, nil).ProcessPending(context.Background(), 10); err != nil {
		t.Fatal(err)
	}
	if len(f.completed) != 1 || f.completed[0] != 8 {
		t.Fatalf("completed=%v", f.completed)
	}
}
