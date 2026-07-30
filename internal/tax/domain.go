package tax

import (
	"context"
	"errors"
	"time"
)

var (
	ErrInvalidInput   = errors.New("tax: invalid input")
	ErrConfiguration  = errors.New("tax: reviewed configuration is incomplete")
	ErrPeriodLocked   = errors.New("tax: period is locked")
	ErrReconciliation = errors.New("tax: ledger does not reconcile to the general ledger")
)

type Money int64

type Document struct {
	ID, CompanyID, PeriodID, SourceID, RuleVersionID int64
	SourceType, SourceNumber, Kind, Direction        string
	TaxNumber, CounterpartyName, CounterpartyTaxID   string
	IssueDate                                        time.Time
	TaxableBase, VATAmount, GrossAmount              Money
	Sign                                             int
	Status                                           string
}

type Withholding struct {
	ID, CompanyID, PeriodID, APInvoiceID int64
	APPaymentID                          *int64
	Article, Code, SupplierTaxID         string
	RecognitionDate                      time.Time
	TaxableBase, Amount                  Money
}

type RecapLine struct {
	Category, AccountCode, AccountName string
	DocumentCount                      int
	TaxableBase, TaxAmount, GLAmount   Money
	Difference                         Money
}

type Period struct {
	ID, CompanyID, AccountingPeriodID int64
	Name, Status                      string
	StartDate, EndDate                time.Time
}
type PostedSource struct {
	Type string
	ID   int64
}

type ExportSchema struct {
	ID                                  int64
	Kind, Version, MediaType, Body      string
	OfficialSourceURL, OfficialChecksum string
	EffectiveFrom                       time.Time
}

type ExportRecord struct {
	TaxNumber, DocumentNumber, CounterpartyName, CounterpartyTaxID string
	IssueDate                                                      time.Time
	TaxableBase, TaxAmount                                         Money
	Sign                                                           int
}

type ExportResult struct {
	ID, Count                int64
	Content, MediaType, Hash string
	SchemaVersion            string
	TaxableBase, TaxAmount   Money
}

type Store interface {
	CaptureARInvoice(context.Context, int64, int64) (Document, error)
	CaptureARCreditNote(context.Context, int64, int64) (Document, error)
	CaptureAPInvoice(context.Context, int64, int64) (Document, error)
	CaptureAPDebitNote(context.Context, int64, int64) (Document, error)
	CaptureAPPayment(context.Context, int64, int64) ([]Withholding, error)
	CancelDocument(context.Context, int64, int64, string) error
	CancelSource(context.Context, string, int64, int64, string) error
	ReplaceDocument(context.Context, int64, int64, int64, string) error
	ListDocuments(context.Context, int64, int64) ([]Document, error)
	ListPeriods(context.Context, int64) ([]Period, error)
	ListPostedSources(context.Context, int64, int64) ([]PostedSource, error)
	Recap(context.Context, int64, int64) ([]RecapLine, error)
	LockPeriod(context.Context, int64, int64, int64) error
	LoadExport(context.Context, int64, int64, string) (ExportSchema, []ExportRecord, error)
	RecordExport(context.Context, int64, int64, int64, string, int, Money, Money, int64) (int64, error)
}

type SchemaValidator interface {
	Validate(schema ExportSchema, payload []byte) error
}
