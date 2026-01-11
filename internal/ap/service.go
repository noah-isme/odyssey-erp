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
	repo            Repository
	procurementService *procurement.Service 
}

func NewService(repo Repository, procService *procurement.Service) *Service {
	return &Service{
		repo:            repo,
		procurementService: procService,
	}
}

// CreateAPInvoice creates a new AP invoice manually.
func (s *Service) CreateAPInvoice(ctx context.Context, input CreateAPInvoiceInput) (APInvoice, error) {
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
			lineTotal := line.Quantity * line.UnitPrice
			subtotal += lineTotal
			// Simple tax calc for now, assuming inclusive or exclusive logic handled in frontend/input
			// If taxPct provided:
			if line.TaxPct > 0 {
				taxAmount += lineTotal * (line.TaxPct / 100)
			}
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

	// 1. Get GRN details
	grn, lines, err := s.procurementService.GetGRNWithLines(ctx, input.GRNID)
	if err != nil {
		return APInvoice{}, fmt.Errorf("failed to get GRN: %w", err)
	}

	if grn.Status != procurement.GRNStatusPosted {
		return APInvoice{}, errors.New("GRN must be posted to create invoice")
	}

	// 2. Prepare Invoice Input
	invInput := CreateAPInvoiceInput{
		SupplierID: grn.SupplierID,
		GRNID:      &grn.ID,
		DueDate:    input.DueDate,
		CreatedBy:  input.CreatedBy,
		Currency:   "IDR", // Default or fetch from PO
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

// PostAPInvoice posts a draft invoice.
func (s *Service) PostAPInvoice(ctx context.Context, input PostAPInvoiceInput) error {
	inv, err := s.repo.GetAPInvoice(ctx, input.InvoiceID)
	if err != nil {
		return err
	}
	if inv.Status != APStatusDraft {
		return ErrInvalidStatus
	}
	return s.repo.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
		return tx.PostAPInvoice(ctx, input)
	})
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
	uniqueInvoices := make(map[int64]struct{})
	for _, alloc := range input.Allocations {
		if alloc.Amount <= 0 {
			return APPayment{}, errors.New("allocation amount must be positive")
		}
		totalAllocated += alloc.Amount
		uniqueInvoices[alloc.APInvoiceID] = struct{}{}

		inv, err := s.repo.GetAPInvoice(ctx, alloc.APInvoiceID)
		if err != nil {
			return APPayment{}, err
		}
		if inv.Status != APStatusPosted {
			return APPayment{}, ErrInvalidStatus
		}

		detail, err := s.repo.GetAPInvoiceWithDetails(ctx, alloc.APInvoiceID)
		if err != nil {
			return APPayment{}, err
		}
		if alloc.Amount > detail.Balance {
			return APPayment{}, errors.New("allocation exceeds invoice balance")
		}
	}
	if totalAllocated > input.Amount {
		return APPayment{}, errors.New("total allocation exceeds payment amount")
	}

	var paymentID int64
	err := s.repo.WithTx(ctx, func(ctx context.Context, tx TxRepository) error {
		if input.Number == "" {
			num, err := tx.GenerateAPPaymentNumber(ctx)
			if err != nil {
				return err
			}
			input.Number = num
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

	for invoiceID := range uniqueInvoices {
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

	// Return the payment (simple struct, no query for ID yet in repo, assumes success)
	return APPayment{ID: paymentID, Number: input.Number, Amount: input.Amount, PaidAt: input.PaidAt}, nil
}

// CalculateAPAging returns aging summary.
func (s *Service) CalculateAPAging(ctx context.Context, asOf time.Time) (APAgingBucket, error) {
	// Fetch all outstanding posted invoices
	invoices, err := s.repo.ListAPInvoices(ctx, ListAPInvoicesRequest{Status: APStatusPosted})
	if err != nil {
		return APAgingBucket{}, err
	}

	bucket := APAgingBucket{}

	for _, inv := range invoices {
		// Calculate balance (need to fetch balance for each? Expensive LOOP query!)
		// Optimization: ListAPInvoices should ideally return balance or fetching all allocations.
		// For MVP, loop might be acceptable if volume low, OR fetch details.
		// Better: Add "GetBalance" logic in the List query? 
		// For now simple approach:
		
		detail, err := s.repo.GetAPInvoiceWithDetails(ctx, inv.ID)
		if err != nil {
			continue 
		}
		balance := detail.Balance
		if balance <= 0 {
			continue
		}

		daysOverdue := int(asOf.Sub(inv.DueAt).Hours() / 24)

		if daysOverdue <= 0 {
			bucket.Current += balance
		} else if daysOverdue <= 30 {
			bucket.Bucket30 += balance
		} else if daysOverdue <= 60 {
			bucket.Bucket60 += balance
		} else if daysOverdue <= 90 {
			bucket.Bucket90 += balance
		} else {
			bucket.Bucket120 += balance
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
