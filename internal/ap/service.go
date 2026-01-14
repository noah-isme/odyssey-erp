package ap

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/odyssey-erp/odyssey-erp/internal/procurement"
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
		var subtotal, taxAmount float64
		for _, line := range input.Lines {
			lineSubtotal := line.Quantity * line.UnitPrice * (1 - (line.DiscountPct / 100))
			lineTax := lineSubtotal * (line.TaxPct / 100)
			subtotal += lineSubtotal
			taxAmount += lineTax
		}

		input.Subtotal = subtotal
		input.TaxAmount = taxAmount
		input.Total = subtotal + taxAmount

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
	if inv.Status != APStatusDraft {
		return ErrInvalidStatus
	}
	if err := s.repo.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
		return tx.PostAPInvoice(ctx, input)
	}); err != nil {
		return err
	}

	if s.integration != nil {
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
		if err := s.integration.HandleAPInvoicePosted(ctx, procurement.APInvoicePostedEvent{
			ID:         invoice.ID,
			Number:     invoice.Number,
			SupplierID: invoice.SupplierID,
			GRNID:      grnID,
			Total:      invoice.Total,
			PostedAt:   *postedAt,
		}); err != nil {
			return err
		}
	}
	return nil
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
	return s.repo.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
		return tx.VoidAPInvoice(ctx, input)
	})
}

// RegisterAPPayment records a payment.
func (s *Service) RegisterAPPayment(ctx context.Context, input CreateAPPaymentInput) (APPayment, error) {
	if input.Amount <= 0 {
		return APPayment{}, errors.New("amount must be positive")
	}
	if len(input.Allocations) == 0 {
		return APPayment{}, errors.New("at least one allocation required")
	}

	totalAllocated := 0.0
	invoiceTotals := make(map[int64]float64)
	for _, alloc := range input.Allocations {
		if alloc.Amount <= 0 {
			return APPayment{}, errors.New("allocation amount must be positive")
		}
		totalAllocated += alloc.Amount
		invoiceTotals[alloc.APInvoiceID] += alloc.Amount
	}
	var supplierID int64
	for invoiceID, allocTotal := range invoiceTotals {
		inv, err := s.repo.GetAPInvoice(ctx, invoiceID)
		if err != nil {
			return APPayment{}, err
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
		if allocTotal > detail.Balance {
			return APPayment{}, fmt.Errorf("allocation exceeds invoice %s balance", inv.Number)
		}
	}
	if input.SupplierID == 0 && supplierID != 0 {
		input.SupplierID = supplierID
	}
	if totalAllocated > input.Amount {
		return APPayment{}, errors.New("total allocation exceeds payment amount")
	}

	var paymentID int64
	var allocationInvoiceID int64
	err := s.repo.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
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

		for _, alloc := range input.Allocations {
			if err := tx.CreatePaymentAllocation(ctx, alloc, id); err != nil {
				return err
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
		ID:          paymentID,
		Number:      input.Number,
		APInvoiceID: apInvoiceIDPtr,
		SupplierID:  input.SupplierID,
		Amount:      input.Amount,
		PaidAt:      input.PaidAt,
		Method:      input.Method,
		Note:        input.Note,
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

	// Return the payment (simple struct, no query for ID yet in repo, assumes success)
	return payment, nil
}

// CalculateAPAging returns aging summary.
func (s *Service) CalculateAPAging(ctx context.Context, asOf time.Time) (APAgingBucket, error) {
	balances, err := s.repo.GetAPInvoiceBalancesBatch(ctx)
	if err != nil {
		return APAgingBucket{}, err
	}

	bucket := APAgingBucket{}

	for _, inv := range balances {
		if inv.Balance <= 0 {
			continue
		}

		daysOverdue := int(asOf.Sub(inv.DueAt).Hours() / 24)

		if daysOverdue <= 0 {
			bucket.Current += inv.Balance
		} else if daysOverdue <= 30 {
			bucket.Bucket30 += inv.Balance
		} else if daysOverdue <= 60 {
			bucket.Bucket60 += inv.Balance
		} else if daysOverdue <= 90 {
			bucket.Bucket90 += inv.Balance
		} else {
			bucket.Bucket120 += inv.Balance
		}
	}
	return bucket, nil
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
