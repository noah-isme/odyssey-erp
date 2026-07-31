package integration

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/odyssey-erp/odyssey-erp/internal/accounting/journals"
	accountingshared "github.com/odyssey-erp/odyssey-erp/internal/accounting/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/ar"
)

func (h *Hooks) CreateARPostingJournal(ctx context.Context, invoice *ar.ARInvoice) error {
	if h == nil || h.ledger == nil || h.periodRepo == nil || h.mappingRepo == nil {
		return nil
	}
	postingDate := time.Now()
	if invoice.PostedAt != nil {
		postingDate = *invoice.PostedAt
	}
	period, err := h.periodRepo.FindOpenPeriodByDate(ctx, postingDate)
	if err != nil {
		return err
	}
	receivable, err := h.resolveAccount(ctx, "AR", "ar.invoice.ar")
	if err != nil {
		return err
	}
	revenue, err := h.resolveAccount(ctx, "AR", "ar.invoice.revenue")
	if err != nil {
		return err
	}
	lines := []journals.PostingLineInput{
		{AccountID: receivable, Debit: round2(invoice.Total)},
		{AccountID: revenue, Credit: round2(invoice.Subtotal)},
	}
	if invoice.TaxAmount > 0 {
		tax, err := h.resolveAccount(ctx, "AR", "ar.invoice.tax")
		if err != nil {
			return err
		}
		lines = append(lines, journals.PostingLineInput{AccountID: tax, Credit: round2(invoice.TaxAmount)})
	}
	return h.post(ctx, journals.PostingInput{
		PeriodID: period.ID, Date: postingDate, SourceModule: "AR.INVOICE",
		SourceID: uuid.NewSHA1(uuid.Nil, []byte(fmt.Sprintf("ARINV:%d", invoice.ID))),
		Memo:     fmt.Sprintf("AR Invoice %s", invoice.Number), PostedBy: pointerValue(invoice.PostedBy), Lines: lines,
	})
}

// CreateARPostingJournalTx is the transaction-aware counterpart used by
// invoice posting. It deliberately shares the same source key as the normal
// path so retries remain idempotent.
func (h *Hooks) CreateARPostingJournalTx(ctx context.Context, tx pgx.Tx, invoice *ar.ARInvoice) error {
	if h == nil || h.ledger == nil || h.periodRepo == nil || h.mappingRepo == nil {
		return nil
	}
	postingDate := time.Now()
	if invoice.PostedAt != nil {
		postingDate = *invoice.PostedAt
	}
	period, err := h.periodRepo.FindOpenPeriodByDate(ctx, postingDate)
	if err != nil {
		return err
	}
	receivable, err := h.resolveAccount(ctx, "AR", "ar.invoice.ar")
	if err != nil {
		return err
	}
	revenue, err := h.resolveAccount(ctx, "AR", "ar.invoice.revenue")
	if err != nil {
		return err
	}
	total, subtotal, taxAmount := invoice.Total, invoice.Subtotal, invoice.TaxAmount
	if invoice.BaseAmount.Amount != "" && invoice.BaseAmount.Amount != "0" {
		if base, parseErr := strconv.ParseFloat(invoice.BaseAmount.String(), 64); parseErr == nil && invoice.Total != 0 {
			ratio := base / invoice.Total
			total, subtotal, taxAmount = base, invoice.Subtotal*ratio, invoice.TaxAmount*ratio
		}
	}
	lines := []journals.PostingLineInput{{AccountID: receivable, Debit: round2(total)}, {AccountID: revenue, Credit: round2(subtotal)}}
	if taxAmount > 0 {
		tax, err := h.resolveAccount(ctx, "AR", "ar.invoice.tax")
		if err != nil {
			return err
		}
		lines = append(lines, journals.PostingLineInput{AccountID: tax, Credit: round2(taxAmount)})
	}
	return h.postTx(ctx, tx, journals.PostingInput{PeriodID: period.ID, Date: postingDate, SourceModule: "AR.INVOICE", SourceID: uuid.NewSHA1(uuid.Nil, []byte(fmt.Sprintf("ARINV:%d", invoice.ID))), Memo: fmt.Sprintf("AR Invoice %s", invoice.Number), PostedBy: pointerValue(invoice.PostedBy), Lines: lines})
}

func (h *Hooks) CreateARCreditNoteJournal(ctx context.Context, note *ar.ARCreditNote) error {
	if h == nil || h.ledger == nil || h.periodRepo == nil || h.mappingRepo == nil {
		return nil
	}
	postingDate := time.Now()
	if note.PostedAt != nil {
		postingDate = *note.PostedAt
	}
	period, err := h.periodRepo.FindOpenPeriodByDate(ctx, postingDate)
	if err != nil {
		return err
	}
	receivable, err := h.resolveAccount(ctx, "AR", "ar.invoice.ar")
	if err != nil {
		return err
	}
	revenue, err := h.resolveAccount(ctx, "AR", "ar.invoice.revenue")
	if err != nil {
		return err
	}
	lines := []journals.PostingLineInput{
		{AccountID: revenue, Debit: round2(note.Subtotal)},
		{AccountID: receivable, Credit: round2(note.Total)},
	}
	if note.TaxAmount > 0 {
		tax, err := h.resolveAccount(ctx, "AR", "ar.invoice.tax")
		if err != nil {
			return err
		}
		lines = append(lines, journals.PostingLineInput{AccountID: tax, Debit: round2(note.TaxAmount)})
	}
	_, err = h.ledger.PostJournal(ctx, journals.PostingInput{
		PeriodID: period.ID, Date: postingDate, SourceModule: "AR.CREDIT_NOTE",
		SourceID: uuid.NewSHA1(uuid.Nil, []byte(fmt.Sprintf("ARCN:%d", note.ID))),
		Memo:     fmt.Sprintf("AR Credit Note %s", note.Number), PostedBy: pointerValue(note.PostedBy), Lines: lines,
	})
	if errors.Is(err, accountingshared.ErrSourceAlreadyLinked) {
		return nil
	}
	return err
}

func pointerValue(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}

var _ ar.AccountingServicePort = (*Hooks)(nil)
