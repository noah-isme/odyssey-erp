package integration

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/odyssey-erp/odyssey-erp/internal/accounting/journals"
	"github.com/odyssey-erp/odyssey-erp/internal/accounting/mappings"
	"github.com/odyssey-erp/odyssey-erp/internal/accounting/periods"
	"github.com/odyssey-erp/odyssey-erp/internal/accounting/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/inventory"
	"github.com/odyssey-erp/odyssey-erp/internal/pos"
	"github.com/odyssey-erp/odyssey-erp/internal/procurement"
)

// Ledger exposes journal posting operations required by integrations.
type Ledger interface {
	PostJournal(ctx context.Context, input journals.PostingInput) (journals.JournalEntry, error)
}

type TransactionalLedger interface {
	PostJournalInTx(ctx context.Context, tx pgx.Tx, input journals.PostingInput) (journals.JournalEntry, error)
}

// PeriodRepository provides period lookups.
type PeriodRepository interface {
	FindOpenPeriodByDate(ctx context.Context, date time.Time) (periods.Period, error)
}

// AccountMappingRepository provides mapping lookups.
type AccountMappingRepository interface {
	Get(ctx context.Context, module, key string) (mappings.AccountMapping, error)
}

// Hooks wires domain events from operational modules into the general ledger.
type Hooks struct {
	ledger      Ledger
	periodRepo  PeriodRepository
	mappingRepo AccountMappingRepository
}

// NewHooks constructs integration hooks.
func NewHooks(ledger Ledger, periodRepo PeriodRepository, mappingRepo AccountMappingRepository) *Hooks {
	return &Hooks{ledger: ledger, periodRepo: periodRepo, mappingRepo: mappingRepo}
}

func (h *Hooks) resolveAccount(ctx context.Context, module, key string) (int64, error) {
	mapping, err := h.mappingRepo.Get(ctx, module, key)
	if err != nil {
		return 0, err
	}
	return mapping.AccountID, nil
}

func (h *Hooks) post(ctx context.Context, input journals.PostingInput) error {
	if input.SourceID == uuid.Nil {
		return errors.New("integration: source id required")
	}
	_, err := h.ledger.PostJournal(ctx, input)
	if err != nil {
		if errors.Is(err, shared.ErrSourceAlreadyLinked) || strings.Contains(err.Error(), "uq_source_links") {
			return nil
		}
	}
	return err
}

func (h *Hooks) postTx(ctx context.Context, tx pgx.Tx, input journals.PostingInput) error {
	if input.SourceID == uuid.Nil {
		return errors.New("integration: source id required")
	}
	ledger, ok := h.ledger.(TransactionalLedger)
	if !ok {
		return errors.New("integration: ledger does not support caller transaction")
	}
	_, err := ledger.PostJournalInTx(ctx, tx, input)
	if errors.Is(err, shared.ErrSourceAlreadyLinked) {
		return nil
	}
	return err
}

func (h *Hooks) postFXTx(ctx context.Context, tx pgx.Tx, sourceKey string, input journals.PostingInput) error {
	var journalID int64
	err := tx.QueryRow(ctx, `SELECT journal_entry_id FROM fx_journal_idempotency WHERE source_key=$1`, sourceKey).Scan(&journalID)
	if err == nil {
		return nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	ledger, ok := h.ledger.(TransactionalLedger)
	if !ok {
		return errors.New("integration: ledger does not support caller transaction")
	}
	entry, err := ledger.PostJournalInTx(ctx, tx, input)
	if errors.Is(err, shared.ErrSourceAlreadyLinked) {
		return nil
	}
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO fx_journal_idempotency (source_key, journal_entry_id) VALUES ($1,$2) ON CONFLICT (source_key) DO NOTHING`, sourceKey, entry.ID)
	return err
}

// HandleGRNPosted posts the accounting entry for a goods receipt.
func (h *Hooks) HandleGRNPosted(ctx context.Context, evt procurement.GRNPostedEvent) error {
	if h == nil || h.ledger == nil || h.periodRepo == nil || h.mappingRepo == nil {
		return nil
	}
	if evt.ReceivedAt.IsZero() {
		return errors.New("integration: GRN received date required")
	}
	period, err := h.periodRepo.FindOpenPeriodByDate(ctx, evt.ReceivedAt)
	if err != nil {
		return err
	}
	inventoryAccount, err := h.resolveAccount(ctx, "GRN", "grn.inventory")
	if err != nil {
		return err
	}
	grirAccount, err := h.resolveAccount(ctx, "GRN", "grn.grir")
	if err != nil {
		return err
	}
	var total float64
	for _, line := range evt.Lines {
		total += monetary(line.Qty, line.UnitCost)
	}
	total = round2(total)
	if total == 0 {
		return nil
	}
	sourceID := uuid.NewSHA1(uuid.Nil, []byte(fmt.Sprintf("GRN:%d", evt.ID)))
	input := journals.PostingInput{
		PeriodID:     period.ID,
		Date:         evt.ReceivedAt,
		SourceModule: "PROCUREMENT.GRN",
		SourceID:     sourceID,
		Memo:         fmt.Sprintf("GRN %s", evt.Number),
		Lines: []journals.PostingLineInput{
			{AccountID: inventoryAccount, Debit: total},
			{AccountID: grirAccount, Credit: total},
		},
	}
	return h.post(ctx, input)
}

// HandlePOSSalePosted posts the cash/tender side of a completed POS sale.
// Inventory cost movements are posted separately by the inventory integration
// hook; this journal is keyed by the ticket so retries cannot duplicate it.
func (h *Hooks) HandlePOSSalePosted(ctx context.Context, evt pos.SalePostedEvent) error {
	if h == nil || h.ledger == nil || h.periodRepo == nil || h.mappingRepo == nil {
		return nil
	}
	if evt.TicketID == 0 || evt.BaseAmount <= 0 {
		return errors.New("integration: POS sale amount required")
	}
	period, err := h.periodRepo.FindOpenPeriodByDate(ctx, time.Now().UTC())
	if err != nil {
		return err
	}
	cash, err := h.resolveAccount(ctx, "POS", "pos.cash")
	if err != nil {
		return err
	}
	revenue, err := h.resolveAccount(ctx, "POS", "pos.sales")
	if err != nil {
		return err
	}
	sourceID := uuid.NewSHA1(uuid.Nil, []byte(fmt.Sprintf("POS:%d", evt.TicketID)))
	return h.post(ctx, journals.PostingInput{
		PeriodID: period.ID, Date: time.Now().UTC(), SourceModule: "POS.SALE", SourceID: sourceID,
		Memo: fmt.Sprintf("POS sale %d", evt.TicketID), PostedBy: evt.ActorID,
		Lines: []journals.PostingLineInput{{AccountID: cash, Debit: evt.BaseAmount}, {AccountID: revenue, Credit: evt.BaseAmount}},
	})
}

func (h *Hooks) HandlePOSRefunded(ctx context.Context, evt pos.SalePostedEvent) error {
	if h == nil || h.ledger == nil || h.periodRepo == nil || h.mappingRepo == nil {
		return nil
	}
	if evt.TicketID == 0 || evt.BaseAmount <= 0 {
		return errors.New("integration: POS refund amount required")
	}
	period, err := h.periodRepo.FindOpenPeriodByDate(ctx, time.Now().UTC())
	if err != nil {
		return err
	}
	cash, err := h.resolveAccount(ctx, "POS", "pos.cash")
	if err != nil {
		return err
	}
	revenue, err := h.resolveAccount(ctx, "POS", "pos.sales")
	if err != nil {
		return err
	}
	sourceID := uuid.NewSHA1(uuid.Nil, []byte(fmt.Sprintf("POS:REFUND:%d", evt.TicketID)))
	return h.post(ctx, journals.PostingInput{PeriodID: period.ID, Date: time.Now().UTC(), SourceModule: "POS.REFUND", SourceID: sourceID, Memo: fmt.Sprintf("POS refund %d", evt.TicketID), PostedBy: evt.ActorID, Lines: []journals.PostingLineInput{{AccountID: revenue, Debit: evt.BaseAmount}, {AccountID: cash, Credit: evt.BaseAmount}}})
}

// HandleAPInvoicePosted posts the accounting entry for an AP invoice.
func (h *Hooks) HandleAPInvoicePosted(ctx context.Context, evt procurement.APInvoicePostedEvent) error {
	if h == nil || h.ledger == nil || h.periodRepo == nil || h.mappingRepo == nil {
		return nil
	}
	if evt.PostedAt.IsZero() {
		return errors.New("integration: AP invoice post date required")
	}
	if evt.Total <= 0 {
		return nil
	}
	period, err := h.periodRepo.FindOpenPeriodByDate(ctx, evt.PostedAt)
	if err != nil {
		return err
	}
	var debitKey string
	if evt.GRNID != 0 {
		debitKey = "ap.invoice.inventory"
	} else {
		debitKey = "ap.invoice.expense"
	}
	debitAccount, err := h.resolveAccount(ctx, "AP", debitKey)
	if err != nil {
		return err
	}
	apAccount, err := h.resolveAccount(ctx, "AP", "ap.invoice.ap")
	if err != nil {
		return err
	}
	amount := round2(evt.Total)
	if evt.BaseAmount > 0 {
		amount = round2(evt.BaseAmount)
	}
	sourceID := uuid.NewSHA1(uuid.Nil, []byte(fmt.Sprintf("APINV:%d", evt.ID)))
	input := journals.PostingInput{
		PeriodID:     period.ID,
		Date:         evt.PostedAt,
		SourceModule: "PROCUREMENT.AP_INVOICE",
		SourceID:     sourceID,
		Memo:         fmt.Sprintf("AP Invoice %s", evt.Number),
		Lines: []journals.PostingLineInput{
			{AccountID: debitAccount, Debit: amount},
			{AccountID: apAccount, Credit: amount},
		},
	}
	return h.post(ctx, input)
}

func (h *Hooks) HandleAPInvoicePostedTx(ctx context.Context, tx pgx.Tx, evt procurement.APInvoicePostedEvent) error {
	if h == nil || h.ledger == nil || h.periodRepo == nil || h.mappingRepo == nil {
		return nil
	}
	if evt.PostedAt.IsZero() {
		return errors.New("integration: AP invoice post date required")
	}
	if evt.Total <= 0 {
		return nil
	}
	period, err := h.periodRepo.FindOpenPeriodByDate(ctx, evt.PostedAt)
	if err != nil {
		return err
	}
	key := "ap.invoice.expense"
	if evt.GRNID != 0 {
		key = "ap.invoice.inventory"
	}
	debit, err := h.resolveAccount(ctx, "AP", key)
	if err != nil {
		return err
	}
	ap, err := h.resolveAccount(ctx, "AP", "ap.invoice.ap")
	if err != nil {
		return err
	}
	amount := round2(evt.Total)
	if evt.BaseAmount > 0 {
		amount = round2(evt.BaseAmount)
	}
	return h.postTx(ctx, tx, journals.PostingInput{PeriodID: period.ID, Date: evt.PostedAt, SourceModule: "PROCUREMENT.AP_INVOICE", SourceID: uuid.NewSHA1(uuid.Nil, []byte(fmt.Sprintf("APINV:%d", evt.ID))), Memo: fmt.Sprintf("AP Invoice %s", evt.Number), Lines: []journals.PostingLineInput{{AccountID: debit, Debit: amount}, {AccountID: ap, Credit: amount}}})
}

// HandleAPPaymentPosted posts the accounting entry for an AP payment.
func (h *Hooks) HandleAPPaymentPosted(ctx context.Context, evt procurement.APPaymentPostedEvent) error {
	if h == nil || h.ledger == nil || h.periodRepo == nil || h.mappingRepo == nil {
		return nil
	}
	if evt.PaidAt.IsZero() {
		return errors.New("integration: AP payment date required")
	}
	if evt.Amount <= 0 {
		return nil
	}
	period, err := h.periodRepo.FindOpenPeriodByDate(ctx, evt.PaidAt)
	if err != nil {
		return err
	}
	apAccount, err := h.resolveAccount(ctx, "AP", "ap.payment.ap")
	if err != nil {
		return err
	}
	cashAccount, err := h.resolveAccount(ctx, "AP", "ap.payment.cash")
	if err != nil {
		return err
	}
	amount := round2(evt.Amount)
	sourceID := uuid.NewSHA1(uuid.Nil, []byte(fmt.Sprintf("APPAY:%d", evt.ID)))
	input := journals.PostingInput{
		PeriodID:     period.ID,
		Date:         evt.PaidAt,
		SourceModule: "PROCUREMENT.AP_PAYMENT",
		SourceID:     sourceID,
		Memo:         fmt.Sprintf("AP Payment %s", evt.Number),
		Lines: []journals.PostingLineInput{
			{AccountID: apAccount, Debit: amount},
			{AccountID: cashAccount, Credit: amount},
		},
	}
	return h.post(ctx, input)
}

func (h *Hooks) HandleGoodsReturnConfirmed(ctx context.Context, evt procurement.GoodsReturnConfirmedEvent) error {
	return nil
}

func (h *Hooks) HandleDebitNotePosted(ctx context.Context, evt procurement.DebitNotePostedEvent) error {
	if h == nil || h.ledger == nil || h.periodRepo == nil || h.mappingRepo == nil || evt.Total <= 0 {
		return nil
	}
	if evt.PostedAt.IsZero() {
		return errors.New("integration: debit note post date required")
	}
	period, err := h.periodRepo.FindOpenPeriodByDate(ctx, evt.PostedAt)
	if err != nil {
		return err
	}
	apAccount, err := h.resolveAccount(ctx, "AP", "ap.debit_note.ap")
	if err != nil {
		return err
	}
	creditKey := "ap.debit_note.expense"
	if evt.GRNID != 0 {
		creditKey = "ap.debit_note.inventory"
	}
	creditAccount, err := h.resolveAccount(ctx, "AP", creditKey)
	if err != nil {
		return err
	}
	amount := round2(evt.Total)
	return h.post(ctx, journals.PostingInput{
		PeriodID: period.ID, Date: evt.PostedAt, SourceModule: "PROCUREMENT.AP_DEBIT_NOTE",
		SourceID: uuid.NewSHA1(uuid.Nil, []byte(fmt.Sprintf("APDN:%d", evt.ID))), Memo: fmt.Sprintf("AP Debit Note %s", evt.Number),
		Lines: []journals.PostingLineInput{{AccountID: apAccount, Debit: amount}, {AccountID: creditAccount, Credit: amount}},
	})
}

// HandleInventoryAdjustmentPosted posts the accounting entry for inventory adjustments.
func (h *Hooks) HandleInventoryAdjustmentPosted(ctx context.Context, evt inventory.AdjustmentPostedEvent) error {
	if h == nil || h.ledger == nil || h.periodRepo == nil || h.mappingRepo == nil {
		return nil
	}
	if evt.PostedAt.IsZero() {
		return errors.New("integration: adjustment post date required")
	}
	if abs(evt.Qty) < 1e-9 {
		return nil
	}
	period, err := h.periodRepo.FindOpenPeriodByDate(ctx, evt.PostedAt)
	if err != nil {
		return err
	}
	inventoryAccount, err := h.resolveAccount(ctx, "INVENTORY", "inventory.adjustment.inventory")
	if err != nil {
		return err
	}
	if evt.RefModule == "RETURN_DELIVERY" {
		cogsAccount, err := h.resolveAccount(ctx, "AR", "ar.return.cogs")
		if err != nil {
			return err
		}
		amount := round2(abs(evt.Qty) * evt.UnitCost)
		if amount == 0 {
			return nil
		}
		lines := []journals.PostingLineInput{
			{AccountID: inventoryAccount, Debit: amount},
			{AccountID: cogsAccount, Credit: amount},
		}
		if evt.Qty < 0 {
			lines = []journals.PostingLineInput{
				{AccountID: cogsAccount, Debit: amount},
				{AccountID: inventoryAccount, Credit: amount},
			}
		}
		return h.post(ctx, journals.PostingInput{
			PeriodID:     period.ID,
			Date:         evt.PostedAt,
			SourceModule: "SALES.RETURN_INVENTORY",
			SourceID:     uuid.NewSHA1(uuid.Nil, []byte(fmt.Sprintf("RETURN:%s:%s:%d", evt.RefID, evt.Code, evt.ProductID))),
			Memo:         fmt.Sprintf("Sales Return Inventory %s", evt.Code),
			Lines:        lines,
		})
	}
	gainAccount, err := h.resolveAccount(ctx, "INVENTORY", "inventory.adjustment.gain")
	if err != nil {
		return err
	}
	lossAccount, err := h.resolveAccount(ctx, "INVENTORY", "inventory.adjustment.loss")
	if err != nil {
		return err
	}
	amount := round2(abs(evt.Qty) * evt.UnitCost)
	if amount == 0 {
		return nil
	}
	sourceID := uuid.NewSHA1(uuid.Nil, []byte(fmt.Sprintf("ADJ:%s:%d", evt.Code, evt.ProductID)))
	lines := make([]journals.PostingLineInput, 0, 2)
	memo := fmt.Sprintf("Inventory Adjustment %s", evt.Code)
	if evt.Qty > 0 {
		lines = append(lines,
			journals.PostingLineInput{AccountID: inventoryAccount, Debit: amount},
			journals.PostingLineInput{AccountID: gainAccount, Credit: amount},
		)
	} else {
		lines = append(lines,
			journals.PostingLineInput{AccountID: lossAccount, Debit: amount},
			journals.PostingLineInput{AccountID: inventoryAccount, Credit: amount},
		)
	}
	input := journals.PostingInput{
		PeriodID:     period.ID,
		Date:         evt.PostedAt,
		SourceModule: "INVENTORY.ADJUSTMENT",
		SourceID:     sourceID,
		Memo:         memo,
		Lines:        lines,
	}
	return h.post(ctx, input)
}

// PostWIPToFinishedGoods records the single manufacturing reclassification
// entry after a WIP-backed finished-goods receipt.
func (h *Hooks) PostWIPToFinishedGoods(ctx context.Context, eventID int64, amount float64) error {
	if h == nil || h.ledger == nil || h.periodRepo == nil || h.mappingRepo == nil || amount <= 0 {
		return nil
	}
	now := time.Now().UTC()
	period, err := h.periodRepo.FindOpenPeriodByDate(ctx, now)
	if err != nil {
		return err
	}
	wip, err := h.resolveAccount(ctx, "INVENTORY", "manufacturing.wip.inventory")
	if err != nil {
		return err
	}
	fg, err := h.resolveAccount(ctx, "INVENTORY", "manufacturing.finished_goods.inventory")
	if err != nil {
		return err
	}
	amount = round2(amount)
	return h.post(ctx, journals.PostingInput{PeriodID: period.ID, Date: now, SourceModule: "MRP.PRODUCTION_RECEIPT", SourceID: uuid.NewSHA1(uuid.Nil, []byte(fmt.Sprintf("MRP-RECEIPT:%d", eventID))), Memo: fmt.Sprintf("WIP to finished goods receipt %d", eventID), Lines: []journals.PostingLineInput{{AccountID: fg, Debit: amount}, {AccountID: wip, Credit: amount}}})
}

var _ procurement.IntegrationHandler = (*Hooks)(nil)
var _ inventory.IntegrationHandler = (*Hooks)(nil)
