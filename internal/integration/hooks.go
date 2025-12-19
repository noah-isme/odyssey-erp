package integration

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/odyssey-erp/odyssey-erp/internal/accounting/journals"
	"github.com/odyssey-erp/odyssey-erp/internal/accounting/mappings"
	"github.com/odyssey-erp/odyssey-erp/internal/accounting/periods"
	"github.com/odyssey-erp/odyssey-erp/internal/accounting/shared"
	"github.com/odyssey-erp/odyssey-erp/internal/inventory"
	"github.com/odyssey-erp/odyssey-erp/internal/procurement"
)

// Ledger exposes journal posting operations required by integrations.
type Ledger interface {
	PostJournal(ctx context.Context, input journals.PostingInput) (journals.JournalEntry, error)
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
		if errors.Is(err, shared.ErrSourceAlreadyLinked) {
			return nil
		}
	}
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

var _ procurement.IntegrationHandler = (*Hooks)(nil)
var _ inventory.IntegrationHandler = (*Hooks)(nil)
