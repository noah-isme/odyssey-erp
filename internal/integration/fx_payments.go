package integration

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/odyssey-erp/odyssey-erp/internal/accounting/journals"
	"github.com/odyssey-erp/odyssey-erp/internal/ap"
	"github.com/odyssey-erp/odyssey-erp/internal/ar"
	"github.com/odyssey-erp/odyssey-erp/internal/fx"
)

func (h *Hooks) CreateARRealizedFXJournalTx(ctx context.Context, tx pgx.Tx, payment *ar.ARPayment, invoice *ar.ARInvoice, allocation ar.ARAllocationValuation) error {
	if h == nil || invoice == nil {
		return nil
	}
	period, err := h.periodRepo.FindOpenPeriodByDate(ctx, payment.PaidAt)
	if err != nil {
		return err
	}
	allocated, err := fx.ParseDecimal(allocation.OriginalAmount.String())
	if err != nil {
		return err
	}
	invoiceRate := invoice.FXRate
	if invoiceRate.IsZero() {
		invoiceRate = fx.MustDecimal("1")
	}
	paymentRate := payment.FXRate
	if paymentRate.IsZero() {
		paymentRate = allocation.Rate
	}
	result, err := fx.CalculatePaymentValuation(allocated, invoiceRate, paymentRate)
	if err != nil {
		return err
	}
	if result.RealizedDifference.IsZero() {
		return nil
	}
	arAccount, err := h.resolveAccount(ctx, "AR", "ar.invoice.ar")
	if err != nil {
		return err
	}
	gain, err := h.resolveAccount(ctx, "FX", "fx.realized.gain")
	if err != nil {
		return err
	}
	loss, err := h.resolveAccount(ctx, "FX", "fx.realized.loss")
	if err != nil {
		return err
	}
	lines := []journals.PostingLineInput{}
	zero := fx.MustDecimal("0")
	if result.RealizedDifference.Cmp(zero) > 0 {
		lines = append(lines, journals.PostingLineInput{AccountID: arAccount, DebitDecimal: result.RealizedDifference}, journals.PostingLineInput{AccountID: gain, CreditDecimal: result.RealizedDifference})
	} else {
		amount := zero.Sub(result.RealizedDifference)
		lines = append(lines, journals.PostingLineInput{AccountID: loss, DebitDecimal: amount}, journals.PostingLineInput{AccountID: arAccount, CreditDecimal: amount})
	}
	key := fx.PaymentFXSourceKey("AR", payment.ID, allocation.InvoiceID)
	return h.postFXTx(ctx, tx, key, journals.PostingInput{PeriodID: period.ID, Date: payment.PaidAt, SourceModule: "AR.PAYMENT.FX", SourceID: uuid.NewSHA1(uuid.Nil, []byte(key)), Memo: fmt.Sprintf("AR realized FX %s", payment.Number), Lines: lines})
}

func (h *Hooks) CreateAPRealizedFXJournalTx(ctx context.Context, tx pgx.Tx, payment *ap.APPayment, invoice *ap.APInvoice, allocation ap.APAllocationValuation) error {
	if h == nil || invoice == nil {
		return nil
	}
	period, err := h.periodRepo.FindOpenPeriodByDate(ctx, payment.PaidAt)
	if err != nil {
		return err
	}
	allocated, err := fx.ParseDecimal(allocation.OriginalAmount.String())
	if err != nil {
		return err
	}
	invoiceRate := invoice.FXRate
	if invoiceRate.IsZero() {
		invoiceRate = fx.MustDecimal("1")
	}
	paymentRate := payment.FXRate
	if paymentRate.IsZero() {
		paymentRate = allocation.Rate
	}
	result, err := fx.CalculatePaymentValuation(allocated, invoiceRate, paymentRate)
	if err != nil {
		return err
	}
	if result.RealizedDifference.IsZero() {
		return nil
	}
	apAccount, err := h.resolveAccount(ctx, "AP", "ap.invoice.ap")
	if err != nil {
		return err
	}
	gain, err := h.resolveAccount(ctx, "FX", "fx.realized.gain")
	if err != nil {
		return err
	}
	loss, err := h.resolveAccount(ctx, "FX", "fx.realized.loss")
	if err != nil {
		return err
	}
	lines := []journals.PostingLineInput{}
	zero := fx.MustDecimal("0")
	if result.RealizedDifference.Cmp(zero) > 0 {
		amount := result.RealizedDifference
		lines = append(lines, journals.PostingLineInput{AccountID: loss, DebitDecimal: amount}, journals.PostingLineInput{AccountID: apAccount, CreditDecimal: amount})
	} else {
		amount := zero.Sub(result.RealizedDifference)
		lines = append(lines, journals.PostingLineInput{AccountID: apAccount, DebitDecimal: amount}, journals.PostingLineInput{AccountID: gain, CreditDecimal: amount})
	}
	key := fx.PaymentFXSourceKey("AP", payment.ID, allocation.InvoiceID)
	return h.postFXTx(ctx, tx, key, journals.PostingInput{PeriodID: period.ID, Date: payment.PaidAt, SourceModule: "AP.PAYMENT.FX", SourceID: uuid.NewSHA1(uuid.Nil, []byte(key)), Memo: fmt.Sprintf("AP Payment FX %s", payment.Number), Lines: lines})
}
