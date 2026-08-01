package ar

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	accountingmoney "github.com/odyssey-erp/odyssey-erp/internal/accounting/money"
	"github.com/odyssey-erp/odyssey-erp/internal/fx"
	shared "github.com/odyssey-erp/odyssey-erp/internal/shared"
)

// Error definitions
var (
	ErrInvoiceNotFound    = errors.New("ar: invoice not found")
	ErrInvalidStatus      = errors.New("ar: invalid invoice status for this operation")
	ErrInsufficientAmount = errors.New("ar: payment amount exceeds invoice balance")
	ErrAlreadyInvoiced    = errors.New("ar: delivery order already invoiced")
)

// RepositoryPort defines data access methods for AR.
type RepositoryPort interface {
	// Invoice operations
	CreateARInvoice(ctx context.Context, input CreateARInvoiceInput) (*ARInvoice, error)
	CreateARInvoiceLine(ctx context.Context, invoiceID int64, line CreateARInvoiceLineInput) (*ARInvoiceLine, error)
	GetARInvoice(ctx context.Context, id int64) (*ARInvoice, error)
	GetARInvoiceByDelivery(ctx context.Context, deliveryOrderID int64) (*ARInvoice, error)
	GetARInvoiceWithDetails(ctx context.Context, id int64) (*ARInvoiceWithDetails, error)
	ListARInvoices(ctx context.Context, req ListARInvoicesRequest) ([]ARInvoice, error)
	ListARInvoiceLines(ctx context.Context, invoiceID int64) ([]ARInvoiceLine, error)
	PostARInvoice(ctx context.Context, id int64, postedBy int64) error
	VoidARInvoice(ctx context.Context, id int64, voidedBy int64, reason string) error
	GetInvoiceBalance(ctx context.Context, id int64) (total, paid, balance float64, err error)
	CountInvoicesByDelivery(ctx context.Context, deliveryOrderID int64) (int, error)
	GenerateInvoiceNumber(ctx context.Context) (string, error)

	// Payment operations
	CreateARPayment(ctx context.Context, input CreateARPaymentInput) (*ARPayment, error)
	CreatePaymentAllocation(ctx context.Context, paymentID, invoiceID int64, amount float64) error
	GeneratePaymentNumber(ctx context.Context) (string, error)
	ListARPayments(ctx context.Context) ([]ARPayment, error)
	ListInvoicePayments(ctx context.Context, invoiceID int64) ([]ARPaymentSummary, error)

	// Aging operations
	ListAROutstanding(ctx context.Context) ([]ARInvoice, error)
}

// DeliveryServicePort for fetching delivery order details.
type DeliveryServicePort interface {
	GetDeliveryOrderForInvoicing(ctx context.Context, id int64) (*DeliveryOrderInfo, error)
}

// DeliveryOrderInfo contains delivery order data for invoicing.
type DeliveryOrderInfo struct {
	ID           int64
	DocNumber    string
	CustomerID   int64
	CustomerName string
	SalesOrderID int64
	WarehouseID  int64
	Currency     string
	Lines        []DeliveryLineInfo
}

// DeliveryLineInfo contains line data for invoicing.
type DeliveryLineInfo struct {
	ID          int64
	ProductID   int64
	ProductName string
	Quantity    float64
	UnitPrice   float64
	DiscountPct float64
	TaxPct      float64
}

// AccountingServicePort for creating journal entries.
type AccountingServicePort interface {
	CreateARPostingJournal(ctx context.Context, invoice *ARInvoice) error
	CreateARCreditNoteJournal(ctx context.Context, creditNote *ARCreditNote) error
}

// FXRateResolver is the only FX dependency AR is allowed to call.
type FXRateResolver interface {
	Resolve(ctx context.Context, base, quote string, date time.Time) (fx.FXQuote, error)
}

type ValuatedARInvoicePoster interface {
	PostARInvoiceWithValuation(context.Context, int64, int64, ARInvoiceValuation) error
}

type TxValuatedARInvoicePoster interface {
	PostARInvoiceWithValuation(context.Context, int64, int64, ARInvoiceValuation) error
}
type TransactionalAccountingServicePort interface {
	CreateARPostingJournalTx(context.Context, pgx.Tx, *ARInvoice) error
}
type AuditPort interface {
	Record(context.Context, shared.AuditLog) error
}
type TransactionalAuditPort interface {
	RecordTx(context.Context, pgx.Tx, shared.AuditLog) error
}

type ARInvoiceValuation struct {
	BaseCurrency string
	BaseAmount   accountingmoney.Money
	Rate         fx.Decimal
	RateDate     time.Time
	Source       string
	LockedAt     time.Time
}

type ARPaymentValuation struct {
	Currency, BaseCurrency     string
	OriginalAmount, BaseAmount accountingmoney.Money
	Rate                       fx.Decimal
	RateDate                   time.Time
	Source                     string
	LockedAt                   time.Time
}
type ARAllocationValuation struct {
	AllocationID, InvoiceID    int64
	Currency, BaseCurrency     string
	OriginalAmount, BaseAmount accountingmoney.Money
	Rate                       fx.Decimal
	RateDate                   time.Time
	Source                     string
	LockedAt                   time.Time
}
type TxARPaymentValuationWriter interface {
	UpdateARPaymentValuation(context.Context, int64, ARPaymentValuation) error
	UpdateARAllocationValuation(context.Context, int64, int64, ARAllocationValuation) error
}
type TransactionalARPaymentFXJournalPort interface {
	CreateARRealizedFXJournalTx(context.Context, pgx.Tx, *ARPayment, *ARInvoice, ARAllocationValuation) error
}

type TaxServicePort interface {
	RecordARInvoice(context.Context, int64, int64) error
	RecordARCreditNote(context.Context, int64, int64) error
	CancelARInvoice(context.Context, int64, int64, string) error
}

// Service handles AR business logic.
type Service struct {
	repo           RepositoryPort
	delivery       DeliveryServicePort
	accounting     AccountingServicePort
	creditNotes    CreditNoteRepositoryPort
	returnDelivery ReturnDeliveryServicePort
	tax            TaxServicePort
	fxResolver     FXRateResolver
	audit          AuditPort
}

// legacyFloat is only used when populating the pre-decimal UI-compatible
// fields. Accounting arithmetic must remain in fx.Decimal.
func legacyFloat(value fx.Decimal) float64 {
	result, _ := strconv.ParseFloat(value.String(), 64)
	return result
}

// NewService builds Service instance.
func NewService(repo RepositoryPort) *Service {
	service := &Service{repo: repo}
	if creditNotes, ok := repo.(CreditNoteRepositoryPort); ok {
		service.creditNotes = creditNotes
	}
	return service
}

// SetDeliveryService sets the delivery service for integration.
func (s *Service) SetDeliveryService(delivery DeliveryServicePort) {
	s.delivery = delivery
}

// SetAccountingService sets the accounting service for journal integration.
func (s *Service) SetAccountingService(accounting AccountingServicePort) {
	s.accounting = accounting
}

func (s *Service) SetTaxService(service TaxServicePort) { s.tax = service }

func (s *Service) SetFXResolver(resolver FXRateResolver) { s.fxResolver = resolver }
func (s *Service) SetAuditLogger(audit AuditPort)        { s.audit = audit }

// CreateARInvoice creates a new AR invoice with lines.
func (s *Service) CreateARInvoice(ctx context.Context, input CreateARInvoiceInput) (*ARInvoice, error) {
	if input.CustomerID == 0 {
		return nil, errors.New("customer ID required")
	}
	if input.Total <= 0 {
		return nil, errors.New("total must be positive")
	}

	// Generate number if not provided
	if input.Number == "" {
		num, err := s.repo.GenerateInvoiceNumber(ctx)
		if err != nil {
			return nil, err
		}
		input.Number = num
	}

	invoice, err := s.repo.CreateARInvoice(ctx, input)
	if err != nil {
		return nil, err
	}

	// Create invoice lines
	for _, line := range input.Lines {
		_, err := s.repo.CreateARInvoiceLine(ctx, invoice.ID, line)
		if err != nil {
			return nil, err
		}
	}

	return invoice, nil
}

// CreateARInvoiceFromDelivery creates an invoice from a delivered order.
func (s *Service) CreateARInvoiceFromDelivery(ctx context.Context, input CreateARInvoiceFromDeliveryInput) (*ARInvoice, error) {
	if s.delivery == nil {
		return nil, errors.New("delivery service not configured")
	}

	// Check if already invoiced
	count, err := s.repo.CountInvoicesByDelivery(ctx, input.DeliveryOrderID)
	if err != nil {
		return nil, err
	}
	if count > 0 {
		return nil, ErrAlreadyInvoiced
	}

	// Get delivery order details
	do, err := s.delivery.GetDeliveryOrderForInvoicing(ctx, input.DeliveryOrderID)
	if err != nil {
		return nil, err
	}

	// Calculate totals
	subtotal, taxAmount, total := fx.MustDecimal("0"), fx.MustDecimal("0"), fx.MustDecimal("0")
	hundred := fx.MustDecimal("100")
	var lines []CreateARInvoiceLineInput

	for _, line := range do.Lines {
		quantity, err := fx.FromLegacyFloat(line.Quantity, 6)
		if err != nil {
			return nil, err
		}
		unitPrice, err := fx.FromLegacyFloat(line.UnitPrice, 6)
		if err != nil {
			return nil, err
		}
		discount, err := fx.FromLegacyFloat(line.DiscountPct, 6)
		if err != nil {
			return nil, err
		}
		tax, err := fx.FromLegacyFloat(line.TaxPct, 6)
		if err != nil {
			return nil, err
		}
		lineSubtotal := quantity.Mul(unitPrice).Mul(fx.MustDecimal("1").Sub(discount.Div(hundred))).Round(2)
		lineTax := lineSubtotal.Mul(tax.Div(hundred)).Round(2)
		lineTotal := lineSubtotal.Add(lineTax).Round(2)

		subtotal = subtotal.Add(lineSubtotal)
		taxAmount = taxAmount.Add(lineTax)
		total = total.Add(lineTotal)

		lines = append(lines, CreateARInvoiceLineInput{
			DeliveryOrderLineID: line.ID,
			ProductID:           line.ProductID,
			Description:         line.ProductName,
			Quantity:            line.Quantity,
			UnitPrice:           line.UnitPrice,
			DiscountPct:         line.DiscountPct,
			TaxPct:              line.TaxPct,
		})
	}

	// Create invoice
	return s.CreateARInvoice(ctx, CreateARInvoiceInput{
		CustomerID:      do.CustomerID,
		SOID:            do.SalesOrderID,
		DeliveryOrderID: do.ID,
		Currency:        do.Currency,
		Subtotal:        legacyFloat(subtotal),
		TaxAmount:       legacyFloat(taxAmount),
		Total:           legacyFloat(total),
		DueDate:         input.DueDate,
		CreatedBy:       input.CreatedBy,
		Lines:           lines,
	})
}

// GetARInvoice retrieves an invoice by ID.
func (s *Service) GetARInvoice(ctx context.Context, id int64) (*ARInvoice, error) {
	return s.repo.GetARInvoice(ctx, id)
}

// GetARInvoiceWithDetails retrieves invoice with lines and payments.
func (s *Service) GetARInvoiceWithDetails(ctx context.Context, id int64) (*ARInvoiceWithDetails, error) {
	return s.repo.GetARInvoiceWithDetails(ctx, id)
}

// ListARInvoices returns invoices with optional filtering.
func (s *Service) ListARInvoices(ctx context.Context, req ListARInvoicesRequest) ([]ARInvoice, error) {
	if req.Limit == 0 {
		req.Limit = 50
	}
	return s.repo.ListARInvoices(ctx, req)
}

// PostARInvoice posts a draft invoice and creates accounting entry.
func (s *Service) PostARInvoice(ctx context.Context, input PostARInvoiceInput) error {
	invoice, err := s.repo.GetARInvoice(ctx, input.InvoiceID)
	if err != nil {
		return err
	}
	if invoice == nil {
		return ErrInvoiceNotFound
	}
	if invoice.Status == ARStatusPosted && (s.accounting != nil || s.tax != nil) {
		if s.accounting != nil {
			if err = s.accounting.CreateARPostingJournal(ctx, invoice); err != nil {
				return err
			}
		}
		if s.tax != nil {
			err = s.tax.RecordARInvoice(ctx, invoice.ID, input.PostedBy)
		}
		return err
	}
	if invoice.Status != ARStatusDraft {
		return ErrInvalidStatus
	}
	valuation, err := s.resolveInvoiceValuation(ctx, invoice)
	if err != nil {
		return err
	}

	invoice.BaseCurrency, invoice.BaseAmount, invoice.FXRate = valuation.BaseCurrency, valuation.BaseAmount, valuation.Rate
	invoice.FXRateDate, invoice.FXRateSource, invoice.FXRateLockedAt = valuation.RateDate, valuation.Source, valuation.LockedAt
	journalInTx := false
	if repo, ok := s.repo.(interface {
		WithTx(context.Context, func(context.Context, TxRepository) error) error
	}); ok {
		err = repo.WithTx(ctx, func(txctx context.Context, tx TxRepository) error {
			if poster, ok := tx.(TxValuatedARInvoicePoster); ok {
				if err := poster.PostARInvoiceWithValuation(txctx, input.InvoiceID, input.PostedBy, *valuation); err != nil {
					return err
				}
			} else if err := tx.PostARInvoice(txctx, input.InvoiceID, input.PostedBy); err != nil {
				return err
			}
			if handle, ok := tx.(interface{ PGXTx() pgx.Tx }); ok {
				if accounting, ok := s.accounting.(TransactionalAccountingServicePort); ok {
					if err := accounting.CreateARPostingJournalTx(txctx, handle.PGXTx(), invoice); err != nil {
						return err
					}
					journalInTx = true
				}
				if audit, ok := s.audit.(TransactionalAuditPort); ok {
					if err := audit.RecordTx(txctx, handle.PGXTx(), shared.AuditLog{ActorID: input.PostedBy, Action: "post", Entity: "ar_invoice", EntityID: strconv.FormatInt(invoice.ID, 10), Meta: map[string]any{"fx_rate": valuation.Rate.String()}}); err != nil {
						return err
					}
				}
			}
			return nil
		})
	} else if poster, ok := s.repo.(ValuatedARInvoicePoster); ok {
		err = poster.PostARInvoiceWithValuation(ctx, input.InvoiceID, input.PostedBy, *valuation)
	} else {
		err = s.repo.PostARInvoice(ctx, input.InvoiceID, input.PostedBy)
	}
	if err != nil {
		return err
	}

	// Create accounting journal entry if service available
	if s.accounting != nil && !journalInTx {
		invoice.Status = ARStatusPosted
		if err := s.accounting.CreateARPostingJournal(ctx, invoice); err != nil {
			slog.Error("create AR posting journal", slog.Any("error", err), slog.Int64("invoice_id", input.InvoiceID))
			return err
		}
	}
	if s.tax != nil {
		err = s.tax.RecordARInvoice(ctx, invoice.ID, input.PostedBy)
		if err != nil {
			return err
		}
	}

	return nil
}

func (s *Service) resolveInvoiceValuation(ctx context.Context, inv *ARInvoice) (*ARInvoiceValuation, error) {
	base := inv.BaseCurrency
	if base == "" {
		base = "IDR"
	}
	if inv.Currency == "" || inv.Currency == base {
		now := time.Now()
		return &ARInvoiceValuation{BaseCurrency: base, BaseAmount: accountingmoney.Must(strconv.FormatFloat(inv.Total, 'f', 2, 64), 2), Rate: fx.MustDecimal("1"), RateDate: now, Source: "INTERNAL", LockedAt: now}, nil
	}
	if s.fxResolver == nil {
		return nil, errors.New("ar: FX resolver is required for foreign-currency invoice")
	}
	now := time.Now()
	quote, err := s.fxResolver.Resolve(ctx, base, inv.Currency, now)
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
	return &ARInvoiceValuation{BaseCurrency: base, BaseAmount: baseAmount, Rate: quote.Rate, RateDate: quote.RateDate, Source: quote.Source, LockedAt: now}, nil
}

// VoidARInvoice voids an invoice.
func (s *Service) VoidARInvoice(ctx context.Context, input VoidARInvoiceInput) error {
	invoice, err := s.repo.GetARInvoice(ctx, input.InvoiceID)
	if err != nil {
		return err
	}
	if invoice == nil {
		return ErrInvoiceNotFound
	}
	if invoice.Status != ARStatusDraft && invoice.Status != ARStatusPosted {
		return ErrInvalidStatus
	}
	if input.VoidReason == "" {
		return errors.New("void reason required")
	}

	if err = s.repo.VoidARInvoice(ctx, input.InvoiceID, input.VoidedBy, input.VoidReason); err != nil {
		return err
	}
	if invoice.Status == ARStatusPosted && s.tax != nil {
		return s.tax.CancelARInvoice(ctx, input.InvoiceID, input.VoidedBy, input.VoidReason)
	}
	return nil
}

// RegisterARPayment records a payment and allocates to invoice(s).
func (s *Service) RegisterARPayment(ctx context.Context, input CreateARPaymentInput) (*ARPayment, error) {
	amount, err := fx.FromLegacyFloat(input.Amount, 2)
	if err != nil || amount.Cmp(fx.MustDecimal("0")) <= 0 {
		return nil, errors.New("amount must be positive")
	}
	if len(input.Allocations) == 0 {
		return nil, errors.New("at least one allocation required")
	}

	// Validate allocations don't exceed payment amount and resolve one locked
	// payment quote for all allocations.
	totalAllocated := fx.MustDecimal("0")
	var firstInvoice *ARInvoice
	for _, alloc := range input.Allocations {
		allocated, err := fx.FromLegacyFloat(alloc.Amount, 2)
		if err != nil || allocated.Cmp(fx.MustDecimal("0")) <= 0 {
			return nil, errors.New("allocation amount must be positive")
		}
		totalAllocated = totalAllocated.Add(allocated)

		invoice, err := s.repo.GetARInvoice(ctx, alloc.ARInvoiceID)
		if err != nil {
			return nil, err
		}
		if invoice == nil {
			return nil, ErrInvoiceNotFound
		}
		if invoice.Status != ARStatusPosted {
			return nil, ErrInvalidStatus
		}
		if firstInvoice == nil {
			firstInvoice = invoice
		}
		if input.Currency == "" {
			input.Currency = invoice.Currency
		}
		if input.Currency != invoice.Currency {
			return nil, errors.New("payment allocations must use one currency")
		}

		// Check invoice balance
		_, _, balance, err := s.repo.GetInvoiceBalance(ctx, alloc.ARInvoiceID)
		if err != nil {
			return nil, err
		}
		balanceDecimal, err := fx.FromLegacyFloat(balance, 2)
		if err != nil || allocated.Cmp(balanceDecimal) > 0 {
			return nil, ErrInsufficientAmount
		}
	}

	if totalAllocated.Cmp(amount) > 0 {
		return nil, errors.New("total allocation exceeds payment amount")
	}
	if input.Currency == "" {
		input.Currency = "IDR"
	}
	baseCurrency := "IDR"
	if firstInvoice != nil && firstInvoice.BaseCurrency != "" {
		baseCurrency = firstInvoice.BaseCurrency
	}
	rate, err := s.resolvePaymentRate(ctx, baseCurrency, input.Currency, input.PaidAt)
	if err != nil {
		return nil, err
	}
	paymentRate := rate.Rate
	paymentBase, err := fx.CalculateBaseAmount(amount, paymentRate)
	if err != nil {
		return nil, err
	}
	paymentVal := ARPaymentValuation{Currency: input.Currency, BaseCurrency: baseCurrency, OriginalAmount: accountingmoney.Must(amount.String(), 2), BaseAmount: accountingmoney.Must(paymentBase.String(), 2), Rate: paymentRate, RateDate: rate.RateDate, Source: rate.Source, LockedAt: time.Now()}
	allocVals := make([]ARAllocationValuation, 0, len(input.Allocations))
	for _, alloc := range input.Allocations {
		invoice, _ := s.repo.GetARInvoice(ctx, alloc.ARInvoiceID)
		invoiceRate := invoice.FXRate
		if invoiceRate.IsZero() {
			invoiceRate = fx.MustDecimal("1")
		}
		allocated, err := fx.FromLegacyFloat(alloc.Amount, 2)
		if err != nil {
			return nil, err
		}
		pv, err := fx.CalculatePaymentValuation(allocated, invoiceRate, paymentRate)
		if err != nil {
			return nil, err
		}
		allocVals = append(allocVals, ARAllocationValuation{InvoiceID: alloc.ARInvoiceID, Currency: input.Currency, BaseCurrency: baseCurrency, OriginalAmount: accountingmoney.Must(allocated.String(), 2), BaseAmount: accountingmoney.Must(pv.SettlementBaseAmount.String(), 2), Rate: paymentRate, RateDate: rate.RateDate, Source: rate.Source, LockedAt: time.Now()})
	}

	// Generate number if not provided
	if input.Number == "" {
		num, err := s.repo.GeneratePaymentNumber(ctx)
		if err != nil {
			return nil, err
		}
		input.Number = num
	}

	payment := &ARPayment{Number: input.Number, Amount: input.Amount, Currency: paymentVal.Currency, OriginalAmount: paymentVal.OriginalAmount, BaseCurrency: paymentVal.BaseCurrency, BaseAmount: paymentVal.BaseAmount, FXRate: paymentVal.Rate, FXRateDate: paymentVal.RateDate, FXRateSource: paymentVal.Source, FXRateLockedAt: paymentVal.LockedAt, PaidAt: input.PaidAt, Method: input.Method, Note: input.Note, CreatedBy: input.CreatedBy}
	if repo, ok := s.repo.(interface {
		WithTx(context.Context, func(context.Context, TxRepository) error) error
	}); ok {
		err = repo.WithTx(ctx, func(txctx context.Context, tx TxRepository) error {
			if input.Number == "" {
				number, err := tx.GeneratePaymentNumber(txctx)
				if err != nil {
					return err
				}
				input.Number = number
				payment.Number = number
			}
			created, err := tx.CreateARPayment(txctx, input)
			if err != nil {
				return err
			}
			payment.ID = created.ID
			writer, _ := tx.(TxARPaymentValuationWriter)
			if writer != nil {
				if err := writer.UpdateARPaymentValuation(txctx, payment.ID, paymentVal); err != nil {
					return err
				}
			}
			for i, alloc := range input.Allocations {
				if err := tx.CreatePaymentAllocation(txctx, payment.ID, alloc.ARInvoiceID, alloc.Amount); err != nil {
					return err
				}
				if writer != nil {
					if err := writer.UpdateARAllocationValuation(txctx, payment.ID, alloc.ARInvoiceID, allocVals[i]); err != nil {
						return err
					}
				}
				if handle, ok := tx.(interface{ PGXTx() pgx.Tx }); ok {
					if journal, ok := s.accounting.(TransactionalARPaymentFXJournalPort); ok {
						invoice, _ := s.repo.GetARInvoice(ctx, alloc.ARInvoiceID)
						if err := journal.CreateARRealizedFXJournalTx(txctx, handle.PGXTx(), payment, invoice, allocVals[i]); err != nil {
							return err
						}
					}
					if audit, ok := s.audit.(TransactionalAuditPort); ok {
						if err := audit.RecordTx(txctx, handle.PGXTx(), shared.AuditLog{ActorID: input.CreatedBy, Action: "record", Entity: "ar_payment", EntityID: strconv.FormatInt(payment.ID, 10), Meta: map[string]any{"fx_rate": paymentVal.Rate.String(), "allocation_id": alloc.ARInvoiceID}}); err != nil {
							return err
						}
					}
				}
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
		return payment, nil
	}
	created, err := s.repo.CreateARPayment(ctx, input)
	if err != nil {
		return nil, err
	}
	payment.ID = created.ID
	for _, alloc := range input.Allocations {
		if err := s.repo.CreatePaymentAllocation(ctx, payment.ID, alloc.ARInvoiceID, alloc.Amount); err != nil {
			return nil, err
		}
	}
	_ = firstInvoice
	return payment, nil
}

func (s *Service) resolvePaymentRate(ctx context.Context, base, currency string, date time.Time) (fx.FXQuote, error) {
	if currency == "" || currency == base {
		return fx.FXQuote{BaseCurrency: base, QuoteCurrency: currency, Rate: fx.MustDecimal("1"), RateDate: date, Source: "INTERNAL"}, nil
	}
	if s.fxResolver == nil {
		return fx.FXQuote{}, errors.New("ar: FX resolver is required for foreign-currency payment")
	}
	return s.fxResolver.Resolve(ctx, base, currency, date)
}

// GetARPayments returns all AR payments.
func (s *Service) GetARPayments(ctx context.Context) ([]ARPayment, error) {
	return s.repo.ListARPayments(ctx)
}

// CalculateARAging groups invoices by due date buckets.
func (s *Service) CalculateARAging(ctx context.Context, asOf time.Time) (ARAgingBucket, error) {
	invoices, err := s.repo.ListAROutstanding(ctx)
	if err != nil {
		return ARAgingBucket{}, err
	}
	if asOf.IsZero() {
		asOf = time.Now()
	}

	current, bucket30, bucket60, bucket90, bucket120 := fx.MustDecimal("0"), fx.MustDecimal("0"), fx.MustDecimal("0"), fx.MustDecimal("0"), fx.MustDecimal("0")
	for _, inv := range invoices {
		// Get balance for this invoice
		_, _, balance, err := s.repo.GetInvoiceBalance(ctx, inv.ID)
		if err != nil {
			continue
		}
		balanceDecimal, err := fx.FromLegacyFloat(balance, 2)
		if err != nil || balanceDecimal.Cmp(fx.MustDecimal("0")) <= 0 {
			continue
		}

		days := int(asOf.Sub(inv.DueAt).Hours() / 24)
		switch {
		case days <= 0:
			current = current.Add(balanceDecimal)
		case days <= 30:
			bucket30 = bucket30.Add(balanceDecimal)
		case days <= 60:
			bucket60 = bucket60.Add(balanceDecimal)
		case days <= 90:
			bucket90 = bucket90.Add(balanceDecimal)
		default:
			bucket120 = bucket120.Add(balanceDecimal)
		}
	}
	return ARAgingBucket{Current: legacyFloat(current), Bucket30: legacyFloat(bucket30), Bucket60: legacyFloat(bucket60), Bucket90: legacyFloat(bucket90), Bucket120: legacyFloat(bucket120)}, nil
}
