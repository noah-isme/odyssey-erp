package ar

import (
	"context"
	"errors"
	"time"
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
	ListARPayments(ctx context.Context) ([]ARPayment, error)
	ListInvoicePayments(ctx context.Context, invoiceID int64) ([]ARPaymentSummary, error)
	GeneratePaymentNumber(ctx context.Context) (string, error)

	// Aging operations
	ListAROutstanding(ctx context.Context) ([]ARInvoice, error)

	// Legacy compatibility
	CreateARInvoiceLegacy(ctx context.Context, input ARInvoiceInput) (*ARInvoice, error)
	CreateARPaymentLegacy(ctx context.Context, input ARPaymentInput) (*ARPayment, error)
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
}

// Service handles AR business logic.
type Service struct {
	repo       RepositoryPort
	delivery   DeliveryServicePort
	accounting AccountingServicePort
}

// NewService builds Service instance.
func NewService(repo RepositoryPort) *Service {
	return &Service{repo: repo}
}

// SetDeliveryService sets the delivery service for integration.
func (s *Service) SetDeliveryService(delivery DeliveryServicePort) {
	s.delivery = delivery
}

// SetAccountingService sets the accounting service for journal integration.
func (s *Service) SetAccountingService(accounting AccountingServicePort) {
	s.accounting = accounting
}

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
	var subtotal, taxAmount, total float64
	var lines []CreateARInvoiceLineInput

	for _, line := range do.Lines {
		lineSubtotal := line.Quantity * line.UnitPrice * (1 - line.DiscountPct/100)
		lineTax := lineSubtotal * (line.TaxPct / 100)
		lineTotal := lineSubtotal + lineTax

		subtotal += lineSubtotal
		taxAmount += lineTax
		total += lineTotal

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
		Subtotal:        subtotal,
		TaxAmount:       taxAmount,
		Total:           total,
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
	if invoice.Status != ARStatusDraft {
		return ErrInvalidStatus
	}

	// Post the invoice
	if err := s.repo.PostARInvoice(ctx, input.InvoiceID, input.PostedBy); err != nil {
		return err
	}

	// Create accounting journal entry if service available
	if s.accounting != nil {
		invoice.Status = ARStatusPosted
		if err := s.accounting.CreateARPostingJournal(ctx, invoice); err != nil {
			// Log but don't fail - journal can be created manually
			// In production, this should use a saga/outbox pattern
		}
	}

	return nil
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

	return s.repo.VoidARInvoice(ctx, input.InvoiceID, input.VoidedBy, input.VoidReason)
}

// RegisterARPayment records a payment and allocates to invoice(s).
func (s *Service) RegisterARPayment(ctx context.Context, input CreateARPaymentInput) (*ARPayment, error) {
	if input.Amount <= 0 {
		return nil, errors.New("amount must be positive")
	}
	if len(input.Allocations) == 0 {
		return nil, errors.New("at least one allocation required")
	}

	// Validate allocations don't exceed payment amount
	var totalAllocated float64
	for _, alloc := range input.Allocations {
		if alloc.Amount <= 0 {
			return nil, errors.New("allocation amount must be positive")
		}
		totalAllocated += alloc.Amount

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

		// Check invoice balance
		_, _, balance, err := s.repo.GetInvoiceBalance(ctx, alloc.ARInvoiceID)
		if err != nil {
			return nil, err
		}
		if alloc.Amount > balance {
			return nil, ErrInsufficientAmount
		}
	}

	if totalAllocated > input.Amount {
		return nil, errors.New("total allocation exceeds payment amount")
	}

	// Generate number if not provided
	if input.Number == "" {
		num, err := s.repo.GeneratePaymentNumber(ctx)
		if err != nil {
			return nil, err
		}
		input.Number = num
	}

	// Create payment
	payment, err := s.repo.CreateARPayment(ctx, input)
	if err != nil {
		return nil, err
	}

	// Create allocations
	for _, alloc := range input.Allocations {
		if err := s.repo.CreatePaymentAllocation(ctx, payment.ID, alloc.ARInvoiceID, alloc.Amount); err != nil {
			return nil, err
		}
	}

	return payment, nil
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

	var bucket ARAgingBucket
	for _, inv := range invoices {
		// Get balance for this invoice
		_, _, balance, err := s.repo.GetInvoiceBalance(ctx, inv.ID)
		if err != nil {
			continue
		}
		if balance <= 0 {
			continue
		}

		days := int(asOf.Sub(inv.DueAt).Hours() / 24)
		switch {
		case days <= 0:
			bucket.Current += balance
		case days <= 30:
			bucket.Bucket30 += balance
		case days <= 60:
			bucket.Bucket60 += balance
		case days <= 90:
			bucket.Bucket90 += balance
		default:
			bucket.Bucket120 += balance
		}
	}
	return bucket, nil
}

// --- Legacy methods for backward compatibility ---

// CreateARInvoiceFromSO creates an AR invoice from a sales order (legacy).
func (s *Service) CreateARInvoiceFromSO(ctx context.Context, input ARInvoiceInput) (*ARInvoice, error) {
	if input.CustomerID == 0 {
		return nil, errors.New("customer ID required")
	}
	if input.SOID == 0 {
		return nil, errors.New("sales order ID required")
	}
	if input.Total <= 0 {
		return nil, errors.New("total must be positive")
	}
	now := time.Now()
	input.CreatedAt = now
	input.UpdatedAt = now
	return s.repo.CreateARInvoiceLegacy(ctx, input)
}

// GetARInvoices returns all AR invoices (legacy).
func (s *Service) GetARInvoices(ctx context.Context) ([]ARInvoice, error) {
	return s.repo.ListARInvoices(ctx, ListARInvoicesRequest{Limit: 1000})
}
