package integration

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/odyssey-erp/odyssey-erp/internal/accounting/journals"
	"github.com/odyssey-erp/odyssey-erp/internal/fx"
)

// CreateRevaluationJournalTx is the production ledger adapter for the FX
// revaluation service. It uses exact decimal journal lines and the caller's
// transaction; the detail row is committed by the same transaction.
func (h *Hooks) CreateRevaluationJournalTx(ctx context.Context, tx pgx.Tx, in fx.RevaluationJournal) (int64, error) {
	module, accountKey := "AR", "ar.invoice.ar"
	if in.DocumentType == fx.APInvoice {
		module, accountKey = "AP", "ap.invoice.ap"
	}
	balance, err := h.resolveAccount(ctx, module, accountKey)
	if err != nil {
		return 0, err
	}
	gain, err := h.resolveAccount(ctx, "FX", "fx.revaluation.gain")
	if err != nil {
		return 0, err
	}
	loss, err := h.resolveAccount(ctx, "FX", "fx.revaluation.loss")
	if err != nil {
		return 0, err
	}
	zero := fx.MustDecimal("0")
	amount := in.Amount
	if amount.Cmp(zero) < 0 {
		amount = zero.Sub(amount)
	}
	lines := []journals.PostingLineInput{}
	positive := in.Amount.Cmp(zero) >= 0
	if in.DocumentType == fx.APInvoice {
		positive = !positive
	}
	if positive {
		lines = append(lines, journals.PostingLineInput{AccountID: balance, DebitDecimal: amount}, journals.PostingLineInput{AccountID: gain, CreditDecimal: amount})
	} else {
		lines = append(lines, journals.PostingLineInput{AccountID: loss, DebitDecimal: amount}, journals.PostingLineInput{AccountID: balance, CreditDecimal: amount})
	}
	key := fmt.Sprintf("FX_REVALUATION:%d:%s:%d", in.PeriodID, in.DocumentType, in.DocumentID)
	ledger, ok := h.ledger.(TransactionalLedger)
	if !ok {
		return 0, fmt.Errorf("integration: transactional ledger required")
	}
	date := in.Date
	if date.IsZero() {
		date = time.Now()
	}
	entry, err := ledger.PostJournalInTx(ctx, tx, journals.PostingInput{PeriodID: in.PeriodID, Date: date, SourceModule: "FX.REVALUATION", SourceID: uuid.NewSHA1(uuid.Nil, []byte(key)), Memo: key, PostedBy: in.ActorID, Lines: lines})
	if err != nil {
		return 0, err
	}
	return entry.ID, nil
}
