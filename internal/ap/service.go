package ap

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	accountingmoney "github.com/odyssey-erp/odyssey-erp/internal/accounting/money"
	"github.com/odyssey-erp/odyssey-erp/internal/fx"
	"github.com/odyssey-erp/odyssey-erp/internal/procurement"
	shared "github.com/odyssey-erp/odyssey-erp/internal/shared"
)

var (
	ErrInvoiceNotFound = errors.New("invoice not found")
	ErrPaymentNotFound = errors.New("payment not found")
	ErrInvalidStatus   = errors.New("invalid status for operation")
	ErrAlreadyInvoiced = errors.New("invoice already exists for GRN")
)

type Service struct {
	repo               Repository
	procurementService *procurement.Service
	integration        procurement.IntegrationHandler
	tax                TaxServicePort
	fxResolver         FXRateResolver
	audit              AuditPort
}

// legacyFloat is only used to populate UI-compatible float fields after the
// exact calculation has completed.
func legacyFloat(value fx.Decimal) float64 {
	result, _ := strconv.ParseFloat(value.String(), 64)
	return result
}

// FXRateResolver is the only FX dependency AP is allowed to call.
type FXRateResolver interface {
	Resolve(ctx context.Context, base, quote string, date time.Time) (fx.FXQuote, error)
}

type ValuatedAPInvoicePoster interface {
	PostAPInvoiceWithValuation(context.Context, PostAPInvoiceInput, APInvoiceValuation) error
}

type TxValuatedAPInvoicePoster interface {
	PostAPInvoiceWithValuation(context.Context, PostAPInvoiceInput, APInvoiceValuation) error
}
type TransactionalAPAccountingPort interface {
	HandleAPInvoicePostedTx(context.Context, pgx.Tx, procurement.APInvoicePostedEvent) error
}
type AuditPort interface {
	Record(context.Context, shared.AuditLog) error
}
type TransactionalAuditPort interface {
	RecordTx(context.Context, pgx.Tx, shared.AuditLog) error
}

type APInvoiceValuation struct {
	BaseCurrency string
	BaseAmount   accountingmoney.Money
	Rate         fx.Decimal
	RateDate     time.Time
	Source       string
	LockedAt     time.Time
}

type APPaymentValuation struct {
	Currency, BaseCurrency     string
	OriginalAmount, BaseAmount accountingmoney.Money
	Rate                       fx.Decimal
	RateDate                   time.Time
	Source                     string
	LockedAt                   time.Time
}
type APAllocationValuation struct {
	AllocationID, InvoiceID    int64
	Currency, BaseCurrency     string
	OriginalAmount, BaseAmount accountingmoney.Money
	Rate                       fx.Decimal
	RateDate                   time.Time
	Source                     string
	LockedAt                   time.Time
}
type TxAPPaymentValuationWriter interface {
	UpdateAPPaymentValuation(context.Context, int64, APPaymentValuation) error
	UpdateAPAllocationValuation(context.Context, int64, int64, APAllocationValuation) error
}
type TransactionalAPPaymentFXJournalPort interface {
	CreateAPRealizedFXJournalTx(context.Context, pgx.Tx, *APPayment, *APInvoice, APAllocationValuation) error
}

type TaxServicePort interface {
	RecordAPInvoice(context.Context, int64, int64) error
	RecordAPDebitNote(context.Context, int64, int64) error
	RecordAPPayment(context.Context, int64, int64) error
	CancelAPInvoice(context.Context, int64, int64, string) error
}

func NewService(repo Repository, procService *procurement.Service) *Service {
	return &Service{
		repo:               repo,
		procurementService: procService,
	}
}

// SetIntegrationHandler injects the accounting integration hooks.
func (s *Service) SetIntegrationHandler(handler procurement.IntegrationHandler) {
	s.integration = handler
}

func (s *Service) SetTaxService(service TaxServicePort) { s.tax = service }

func (s *Service) SetFXResolver(resolver FXRateResolver) { s.fxResolver = resolver }
func (s *Service) SetAuditLogger(audit AuditPort)        { s.audit = audit }

// CreateAPInvoice creates a new AP invoice manually.
func (s *Service) CreateAPInvoice(ctx context.Context, input CreateAPInvoiceInput) (APInvoice, error) {
	if len(input.Lines) == 0 {
		return APInvoice{}, errors.New("at least one line is required")
	}
	var invoiceID int64
	err := s.repo.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
		// Generate number if not provided
		if input.Number == "" {
			num, err := tx.GenerateAPInvoiceNumber(ctx)
			if err != nil {
				return err
			}
			input.Number = num
		}

		// Calculate totals from lines
		subtotal, taxAmount := fx.MustDecimal("0"), fx.MustDecimal("0")
		hundred := fx.MustDecimal("100")
		for _, line := range input.Lines {
			quantity, err := fx.FromLegacyFloat(line.Quantity, 6)
			if err != nil {
				return err
			}
			unitPrice, err := fx.FromLegacyFloat(line.UnitPrice, 6)
			if err != nil {
				return err
			}
			discount, err := fx.FromLegacyFloat(line.DiscountPct, 6)
			if err != nil {
				return err
			}
			tax, err := fx.FromLegacyFloat(line.TaxPct, 6)
			if err != nil {
				return err
			}
			lineSubtotal := quantity.Mul(unitPrice).Mul(fx.MustDecimal("1").Sub(discount.Div(hundred))).Round(2)
			lineTax := lineSubtotal.Mul(tax.Div(hundred)).Round(2)
			subtotal = subtotal.Add(lineSubtotal)
			taxAmount = taxAmount.Add(lineTax)
		}

		input.Subtotal = legacyFloat(subtotal)
		input.TaxAmount = legacyFloat(taxAmount)
		input.Total = legacyFloat(subtotal.Add(taxAmount))

		id, err := tx.CreateAPInvoice(ctx, input)
		if err != nil {
			return err
		}
		invoiceID = id

		for _, line := range input.Lines {
			if err := tx.CreateAPInvoiceLine(ctx, line, id); err != nil {
				return err
			}
		}
		return nil
	})

	if err != nil {
		return APInvoice{}, err
	}

	return s.repo.GetAPInvoice(ctx, invoiceID)
}

// CreateAPInvoiceFromGRN creates an invoice from a Goods Receipt Note.
func (s *Service) CreateAPInvoiceFromGRN(ctx context.Context, input CreateAPInvoiceFromGRNInput) (APInvoice, error) {
	count, err := s.repo.CountInvoicesByGRN(ctx, input.GRNID)
	if err != nil {
		return APInvoice{}, err
	}
	if count > 0 {
		return APInvoice{}, ErrAlreadyInvoiced
	}
	if input.DueDate.IsZero() {
		input.DueDate = time.Now().AddDate(0, 0, 30)
	}

	// 1. Get GRN details
	grn, lines, err := s.procurementService.GetGRNWithLines(ctx, input.GRNID)
	if err != nil {
		return APInvoice{}, fmt.Errorf("failed to get GRN: %w", err)
	}

	if grn.Status != procurement.GRNStatusPosted {
		return APInvoice{}, errors.New("GRN must be posted before invoicing")
	}

	// 2. Prepare Invoice Input
	currency := "IDR"
	invInput := CreateAPInvoiceInput{
		SupplierID: grn.SupplierID,
		GRNID:      &grn.ID,
		POID:       nil,
		DueDate:    input.DueDate,
		CreatedBy:  input.CreatedBy,
		Currency:   currency,
		Number:     input.Number,
	}
	if grn.POID != 0 {
		po, _, err := s.procurementService.GetPOWithLines(ctx, grn.POID)
		if err != nil {
			return APInvoice{}, fmt.Errorf("failed to load PO for GRN: %w", err)
		}
		if po.SupplierID != grn.SupplierID {
			return APInvoice{}, errors.New("GRN supplier does not match PO supplier")
		}
		if po.Status != procurement.POStatusApproved && po.Status != procurement.POStatusClosed {
			return APInvoice{}, errors.New("PO must be approved before invoicing GRN")
		}
		if po.Currency != "" {
			currency = po.Currency
			invInput.Currency = currency
		}
		poID := grn.POID
		invInput.POID = &poID
	}

	// 3. Map Lines
	for _, l := range lines {
		invInput.Lines = append(invInput.Lines, CreateAPInvoiceLineInput{
			GRNLineID:   &l.ID,
			ProductID:   l.ProductID,
			Description: fmt.Sprintf("Product %d", l.ProductID), // Should fetch product name ideally
			Quantity:    l.Qty,
			UnitPrice:   l.UnitCost,
			DiscountPct: 0,
			TaxPct:      0, // Need logic to fetch tax from PO
		})
	}

	// 4. Create Invoice
	return s.CreateAPInvoice(ctx, invInput)
}

// CreateAPInvoiceFromPO creates an invoice from an approved PO.
func (s *Service) CreateAPInvoiceFromPO(ctx context.Context, input CreateAPInvoiceFromPOInput) (APInvoice, error) {
	po, lines, err := s.procurementService.GetPOWithLines(ctx, input.POID)
	if err != nil {
		return APInvoice{}, fmt.Errorf("failed to get PO: %w", err)
	}
	if input.DueDate.IsZero() {
		input.DueDate = time.Now().AddDate(0, 0, 30)
	}

	if po.Status != procurement.POStatusApproved && po.Status != procurement.POStatusClosed {
		return APInvoice{}, errors.New("PO must be approved before invoicing")
	}
	if po.SupplierID == 0 {
		return APInvoice{}, errors.New("PO supplier must be set before invoicing")
	}

	currency := po.Currency
	if currency == "" {
		currency = "IDR"
	}

	invInput := CreateAPInvoiceInput{
		SupplierID: po.SupplierID,
		Currency:   currency,
		DueDate:    input.DueDate,
		CreatedBy:  input.CreatedBy,
		Number:     input.Number,
		POID:       &input.POID,
	}

	for _, l := range lines {
		desc := l.Note
		if desc == "" {
			desc = fmt.Sprintf("Product %d", l.ProductID)
		}
		invInput.Lines = append(invInput.Lines, CreateAPInvoiceLineInput{
			ProductID:   l.ProductID,
			Description: desc,
			Quantity:    l.Qty,
			UnitPrice:   l.Price,
			DiscountPct: 0,
			TaxPct:      0,
		})
	}

	return s.CreateAPInvoice(ctx, invInput)
}

// PostAPInvoice posts a draft invoice.
func (s *Service) PostAPInvoice(ctx context.Context, input PostAPInvoiceInput) error {
	inv, err := s.repo.GetAPInvoice(ctx, input.InvoiceID)
	if err != nil {
		return err
	}
	if inv.Status == APStatusPosted && s.tax != nil {
		return s.tax.RecordAPInvoice(ctx, input.InvoiceID, input.PostedBy)
	}
	if inv.Status != APStatusDraft {
		return ErrInvalidStatus
	}
	valuation, err := s.resolveInvoiceValuation(ctx, inv)
	if err != nil {
		return err
	}
	journalInTx := false
	if err := s.repo.WithTx(ctx, func(txctx context.Context, tx TxRepository) error {
		if poster, ok := tx.(TxValuatedAPInvoicePoster); ok {
			if err := poster.PostAPInvoiceWithValuation(txctx, input, *valuation); err != nil {
				return err
			}
		} else if err := tx.PostAPInvoice(txctx, input); err != nil {
			return err
		}
		if handle, ok := tx.(interface{ PGXTx() pgx.Tx }); ok {
			if accounting, ok := s.integration.(TransactionalAPAccountingPort); ok {
				baseTotal, _ := strconv.ParseFloat(valuation.BaseAmount.String(), 64)
				evt := procurement.APInvoicePostedEvent{ID: inv.ID, Number: inv.Number, SupplierID: inv.SupplierID, Total: inv.Total, BaseAmount: baseTotal, PostedAt: time.Now()}
				if inv.GRNID != nil {
					evt.GRNID = *inv.GRNID
				}
				if err := accounting.HandleAPInvoicePostedTx(txctx, handle.PGXTx(), evt); err != nil {
					return err
				}
				journalInTx = true
			}
			if audit, ok := s.audit.(TransactionalAuditPort); ok {
				if err := audit.RecordTx(txctx, handle.PGXTx(), shared.AuditLog{ActorID: input.PostedBy, Action: "post", Entity: "ap_invoice", EntityID: strconv.FormatInt(inv.ID, 10), Meta: map[string]any{"fx_rate": valuation.Rate.String()}}); err != nil {
					return err
				}
			}
		}
		return nil
	}); err != nil {
		return err
	}

	if s.integration != nil && !journalInTx {
		invoice, err := s.repo.GetAPInvoice(ctx, input.InvoiceID)
		if err != nil {
			return err
		}
		var grnID int64
		if invoice.GRNID != nil {
			grnID = *invoice.GRNID
		}
		postedAt := invoice.PostedAt
		if postedAt == nil {
			now := time.Now()
			postedAt = &now
		}
		baseTotal, _ := strconv.ParseFloat(invoice.BaseAmount.String(), 64)
		if err := s.integration.HandleAPInvoicePosted(ctx, procurement.APInvoicePostedEvent{
			ID:         invoice.ID,
			Number:     invoice.Number,
			SupplierID: invoice.SupplierID,
			GRNID:      grnID,
			Total:      invoice.Total,
			BaseAmount: baseTotal,
			PostedAt:   *postedAt,
		}); err != nil {
			return err
		}
	}
	if s.tax != nil {
		return s.tax.RecordAPInvoice(ctx, input.InvoiceID, input.PostedBy)
	}
	return nil
}

func (s *Service) resolveInvoiceValuation(ctx context.Context, inv APInvoice) (*APInvoiceValuation, error) {
	base := inv.BaseCurrency
	if base == "" {
		base = "IDR"
	}
	if inv.Currency == "" || inv.Currency == base {
		return &APInvoiceValuation{BaseCurrency: base, BaseAmount: accountingmoney.Must(strconv.FormatFloat(inv.Total, 'f', 2, 64), 2), Rate: fx.MustDecimal("1"), RateDate: time.Now(), Source: "INTERNAL", LockedAt: time.Now()}, nil
	}
	if s.fxResolver == nil {
		return nil, fmt.Errorf("ap: FX resolver is required for %s invoice", inv.Currency)
	}
	date := time.Now()
	quote, err := s.fxResolver.Resolve(ctx, base, inv.Currency, date)
	if err != nil {
		return nil, err
	}
	originalText := inv.OriginalAmount.String()
	if inv.OriginalAmount.Cmp(accountingmoney.Money{}) == 0 && inv.Total != 0 {
		originalText = strconv.FormatFloat(inv.Total, 'f', 2, 64)
	}
	original, err := fx.ParseDecimal(originalText)
	if err != nil {
		return nil, err
	}
	amount, err := fx.CalculateBaseAmount(original, quote.Rate)
	if err != nil {
		return nil, err
	}
	baseAmount, err := accountingmoney.Parse(amount.String(), 2)
	if err != nil {
		return nil, err
	}
	return &APInvoiceValuation{BaseCurrency: base, BaseAmount: baseAmount, Rate: quote.Rate, RateDate: quote.RateDate, Source: quote.Source, LockedAt: time.Now()}, nil
}

func (s *Service) resolvePaymentRate(ctx context.Context, base, currency string, date time.Time) (fx.FXQuote, error) {
	if currency == "" || currency == base {
		return fx.FXQuote{BaseCurrency: base, QuoteCurrency: currency, Rate: fx.MustDecimal("1"), RateDate: date, Source: "INTERNAL"}, nil
	}
	if s.fxResolver == nil {
		return fx.FXQuote{}, errors.New("ap: FX resolver is required for foreign-currency payment")
	}
	return s.fxResolver.Resolve(ctx, base, currency, date)
}

// VoidAPInvoice voids an invoice.
func (s *Service) VoidAPInvoice(ctx context.Context, input VoidAPInvoiceInput) error {
	inv, err := s.repo.GetAPInvoice(ctx, input.InvoiceID)
	if err != nil {
		return err
	}
	if inv.Status == APStatusPaid || inv.Status == APStatusVoid {
		return ErrInvalidStatus
	}
	err = s.repo.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
		return tx.VoidAPInvoice(ctx, input)
	})
	if err != nil {
		return err
	}
	if inv.Status == APStatusPosted && s.tax != nil {
		return s.tax.CancelAPInvoice(ctx, input.InvoiceID, input.VoidedBy, input.VoidReason)
	}
	return nil
}

// RegisterAPPayment records a payment.
func (s *Service) RegisterAPPayment(ctx context.Context, input CreateAPPaymentInput) (APPayment, error) {
	amount, err := fx.FromLegacyFloat(input.Amount, 2)
	if err != nil || amount.Cmp(fx.MustDecimal("0")) <= 0 {
		return APPayment{}, errors.New("amount must be positive")
	}
	if len(input.Allocations) == 0 {
		return APPayment{}, errors.New("at least one allocation required")
	}

	totalAllocated := fx.MustDecimal("0")
	invoiceTotals := make(map[int64]fx.Decimal)
	invoices := make(map[int64]APInvoice)
	for _, alloc := range input.Allocations {
		allocated, err := fx.FromLegacyFloat(alloc.Amount, 2)
		if err != nil || allocated.Cmp(fx.MustDecimal("0")) <= 0 {
			return APPayment{}, errors.New("allocation amount must be positive")
		}
		totalAllocated = totalAllocated.Add(allocated)
		invoiceTotals[alloc.APInvoiceID] = invoiceTotals[alloc.APInvoiceID].Add(allocated)
	}
	var supplierID int64
	for invoiceID, allocTotal := range invoiceTotals {
		inv, err := s.repo.GetAPInvoice(ctx, invoiceID)
		if err != nil {
			return APPayment{}, err
		}
		invoices[invoiceID] = inv
		if input.Currency == "" {
			input.Currency = inv.Currency
		}
		if input.Currency != inv.Currency {
			return APPayment{}, errors.New("payment allocations must use one currency")
		}
		if supplierID == 0 {
			supplierID = inv.SupplierID
		} else if inv.SupplierID != supplierID {
			return APPayment{}, errors.New("allocations must reference invoices from the same supplier")
		}
		if input.SupplierID != 0 && inv.SupplierID != input.SupplierID {
			return APPayment{}, errors.New("payment supplier does not match invoice supplier")
		}
		if inv.Status != APStatusPosted {
			return APPayment{}, fmt.Errorf("invoice %s must be posted before payment allocation", inv.Number)
		}
		if inv.POID != nil {
			po, _, err := s.procurementService.GetPOWithLines(ctx, *inv.POID)
			if err != nil {
				return APPayment{}, fmt.Errorf("failed to load PO for invoice %s: %w", inv.Number, err)
			}
			if po.SupplierID != inv.SupplierID {
				return APPayment{}, fmt.Errorf("invoice %s supplier does not match PO supplier", inv.Number)
			}
		}

		detail, err := s.repo.GetAPInvoiceWithDetails(ctx, invoiceID)
		if err != nil {
			return APPayment{}, err
		}
		balance, err := fx.FromLegacyFloat(detail.Balance, 2)
		if err != nil || allocTotal.Cmp(balance) > 0 {
			return APPayment{}, fmt.Errorf("allocation exceeds invoice %s balance", inv.Number)
		}
	}
	if input.SupplierID == 0 && supplierID != 0 {
		input.SupplierID = supplierID
	}
	if totalAllocated.Cmp(amount) > 0 {
		return APPayment{}, errors.New("total allocation exceeds payment amount")
	}
	if input.Currency == "" {
		input.Currency = "IDR"
	}
	baseCurrency := "IDR"
	for _, invoice := range invoices {
		if invoice.BaseCurrency != "" {
			baseCurrency = invoice.BaseCurrency
			break
		}
	}
	rate, err := s.resolvePaymentRate(ctx, baseCurrency, input.Currency, input.PaidAt)
	if err != nil {
		return APPayment{}, err
	}
	paymentRate := rate.Rate
	paymentBase, err := fx.CalculateBaseAmount(amount, paymentRate)
	if err != nil {
		return APPayment{}, err
	}
	paymentVal := APPaymentValuation{Currency: input.Currency, BaseCurrency: baseCurrency, OriginalAmount: accountingmoney.Must(amount.String(), 2), BaseAmount: accountingmoney.Must(paymentBase.String(), 2), Rate: paymentRate, RateDate: rate.RateDate, Source: rate.Source, LockedAt: time.Now()}
	allocVals := make([]APAllocationValuation, 0, len(input.Allocations))
	for _, alloc := range input.Allocations {
		invoice := invoices[alloc.APInvoiceID]
		invoiceRate := invoice.FXRate
		if invoiceRate.IsZero() {
			invoiceRate = fx.MustDecimal("1")
		}
		allocated, err := fx.FromLegacyFloat(alloc.Amount, 2)
		if err != nil {
			return APPayment{}, err
		}
		pv, err := fx.CalculatePaymentValuation(allocated, invoiceRate, paymentRate)
		if err != nil {
			return APPayment{}, err
		}
		baseCurrency := "IDR"
		if invoice.BaseCurrency != "" {
			baseCurrency = invoice.BaseCurrency
		}
		allocVals = append(allocVals, APAllocationValuation{InvoiceID: alloc.APInvoiceID, Currency: input.Currency, BaseCurrency: baseCurrency, OriginalAmount: accountingmoney.Must(allocated.String(), 2), BaseAmount: accountingmoney.Must(pv.SettlementBaseAmount.String(), 2), Rate: paymentRate, RateDate: rate.RateDate, Source: rate.Source, LockedAt: time.Now()})
	}

	var paymentID int64
	var allocationInvoiceID int64
	var paymentValuation APPaymentValuation = paymentVal
	err = s.repo.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
		if input.Number == "" {
			num, err := tx.GenerateAPPaymentNumber(ctx)
			if err != nil {
				return err
			}
			input.Number = num
		}

		if len(input.Allocations) == 1 {
			allocationInvoiceID = input.Allocations[0].APInvoiceID
		}

		id, err := tx.CreateAPPayment(ctx, input)
		if err != nil {
			return err
		}
		paymentID = id
		if writer, ok := tx.(TxAPPaymentValuationWriter); ok {
			if err := writer.UpdateAPPaymentValuation(ctx, paymentID, paymentValuation); err != nil {
				return err
			}
		}

		for i, alloc := range input.Allocations {
			if err := tx.CreatePaymentAllocation(ctx, alloc, id); err != nil {
				return err
			}
			if writer, ok := tx.(TxAPPaymentValuationWriter); ok {
				if err := writer.UpdateAPAllocationValuation(ctx, id, alloc.APInvoiceID, allocVals[i]); err != nil {
					return err
				}
			}
			if handle, ok := tx.(interface{ PGXTx() pgx.Tx }); ok {
				if journal, ok := s.integration.(TransactionalAPPaymentFXJournalPort); ok {
					p := &APPayment{ID: id, Number: input.Number, Amount: input.Amount, Currency: input.Currency, OriginalAmount: paymentVal.OriginalAmount, BaseCurrency: paymentVal.BaseCurrency, BaseAmount: paymentVal.BaseAmount, FXRate: paymentVal.Rate, FXRateDate: paymentVal.RateDate, FXRateSource: paymentVal.Source, FXRateLockedAt: paymentVal.LockedAt, PaidAt: input.PaidAt}
					invoice := invoices[alloc.APInvoiceID]
					if err := journal.CreateAPRealizedFXJournalTx(ctx, handle.PGXTx(), p, &invoice, allocVals[i]); err != nil {
						return err
					}
				}
				if audit, ok := s.audit.(TransactionalAuditPort); ok {
					if err := audit.RecordTx(ctx, handle.PGXTx(), shared.AuditLog{ActorID: input.CreatedBy, Action: "record", Entity: "ap_payment", EntityID: strconv.FormatInt(paymentID, 10), Meta: map[string]any{"fx_rate": paymentVal.Rate.String(), "allocation_id": alloc.APInvoiceID}}); err != nil {
						return err
					}
				}
			}
		}
		return nil
	})

	if err != nil {
		return APPayment{}, err
	}

	for invoiceID := range invoiceTotals {
		detail, err := s.repo.GetAPInvoiceWithDetails(ctx, invoiceID)
		if err != nil {
			return APPayment{}, err
		}
		if detail.Status == APStatusPosted && detail.Balance <= 0 {
			if err := s.repo.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
				return tx.UpdateAPStatus(ctx, invoiceID, APStatusPaid)
			}); err != nil {
				return APPayment{}, err
			}
		}
	}

	var apInvoiceIDPtr *int64
	if allocationInvoiceID != 0 {
		apInvoiceIDPtr = &allocationInvoiceID
	}
	payment := APPayment{
		ID: paymentID, Number: input.Number, APInvoiceID: apInvoiceIDPtr, SupplierID: input.SupplierID,
		Amount: input.Amount, Currency: paymentVal.Currency, OriginalAmount: paymentVal.OriginalAmount,
		BaseCurrency: paymentVal.BaseCurrency, BaseAmount: paymentVal.BaseAmount, FXRate: paymentVal.Rate,
		FXRateDate: paymentVal.RateDate, FXRateSource: paymentVal.Source, FXRateLockedAt: paymentVal.LockedAt,
		PaidAt: input.PaidAt, Method: input.Method, Note: input.Note,
	}

	if s.integration != nil {
		apInvoiceID := allocationInvoiceID
		if apInvoiceID == 0 {
			for invoiceID := range invoiceTotals {
				apInvoiceID = invoiceID
				break
			}
		}
		if err := s.integration.HandleAPPaymentPosted(ctx, procurement.APPaymentPostedEvent{
			ID:          paymentID,
			Number:      input.Number,
			APInvoiceID: apInvoiceID,
			Amount:      input.Amount,
			PaidAt:      input.PaidAt,
		}); err != nil {
			return payment, wrapLedgerPostError(err)
		}
	}
	if s.tax != nil {
		if err := s.tax.RecordAPPayment(ctx, paymentID, input.CreatedBy); err != nil {
			return payment, err
		}
	}

	// Return the payment (simple struct, no query for ID yet in repo, assumes success)
	return payment, nil
}

// CalculateAPAging returns aging summary.
func (s *Service) CalculateAPAging(ctx context.Context, asOf time.Time) (APAgingBucket, error) {
	balances, err := s.repo.GetAPInvoiceBalancesBatch(ctx)
	if err != nil {
		return APAgingBucket{}, err
	}

	current, bucket30, bucket60, bucket90, bucket120 := fx.MustDecimal("0"), fx.MustDecimal("0"), fx.MustDecimal("0"), fx.MustDecimal("0"), fx.MustDecimal("0")

	for _, inv := range balances {
		balance, err := fx.FromLegacyFloat(inv.Balance, 2)
		if err != nil || balance.Cmp(fx.MustDecimal("0")) <= 0 {
			continue
		}

		daysOverdue := int(asOf.Sub(inv.DueAt).Hours() / 24)

		if daysOverdue <= 0 {
			current = current.Add(balance)
		} else if daysOverdue <= 30 {
			bucket30 = bucket30.Add(balance)
		} else if daysOverdue <= 60 {
			bucket60 = bucket60.Add(balance)
		} else if daysOverdue <= 90 {
			bucket90 = bucket90.Add(balance)
		} else {
			bucket120 = bucket120.Add(balance)
		}
	}
	return APAgingBucket{Current: legacyFloat(current), Bucket30: legacyFloat(bucket30), Bucket60: legacyFloat(bucket60), Bucket90: legacyFloat(bucket90), Bucket120: legacyFloat(bucket120)}, nil
}

func (s *Service) ListAPInvoices(ctx context.Context, req ListAPInvoicesRequest) ([]APInvoice, error) {
	return s.repo.ListAPInvoices(ctx, req)
}

func (s *Service) GetAPInvoiceWithDetails(ctx context.Context, id int64) (APInvoiceWithDetails, error) {
	return s.repo.GetAPInvoiceWithDetails(ctx, id)
}

func (s *Service) ListAPPayments(ctx context.Context) ([]APPayment, error) {
	return s.repo.ListAPPayments(ctx)
}

func (s *Service) GetAPPaymentWithDetails(ctx context.Context, id int64) (APPaymentWithDetails, error) {
	return s.repo.GetAPPaymentWithDetails(ctx, id)
}
