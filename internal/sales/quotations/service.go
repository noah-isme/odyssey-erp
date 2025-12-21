package quotations

import (
	"context"
	"errors"
	"fmt"


	"github.com/odyssey-erp/odyssey-erp/internal/sales/customers"
	"github.com/odyssey-erp/odyssey-erp/internal/sales/shared"
)

var (
	ErrInvalidStatus = errors.New("invalid status transition")
)

type Service struct {
	repo         Repository
	customerRepo customers.Repository
}

func NewService(repo Repository, customerRepo customers.Repository) *Service {
	return &Service{
		repo:         repo,
		customerRepo: customerRepo,
	}
}

func (s *Service) Create(ctx context.Context, req CreateQuotationRequest, createdBy int64) (*Quotation, error) {
	if req.ValidUntil.Before(req.QuoteDate) {
		return nil, errors.New("valid_until must be after quote_date")
	}

	_, err := s.customerRepo.Get(ctx, req.CustomerID)
	if err != nil {
		return nil, fmt.Errorf("verify customer: %w", err)
	}

	docNumber, err := s.repo.GenerateNumber(ctx, req.CompanyID, req.QuoteDate)
	if err != nil {
		return nil, fmt.Errorf("generate doc number: %w", err)
	}

	var subtotal, taxAmount, totalAmount float64
	for _, lineReq := range req.Lines {
		discount, tax, lineTotal := shared.CalculateLineTotals(
			lineReq.Quantity,
			lineReq.UnitPrice,
			lineReq.DiscountPercent,
			lineReq.TaxPercent,
		)
		subtotal += (lineReq.Quantity * lineReq.UnitPrice) - discount
		taxAmount += tax
		totalAmount += lineTotal
	}

	quotation := Quotation{
		DocNumber:   docNumber,
		CompanyID:   req.CompanyID,
		CustomerID:  req.CustomerID,
		QuoteDate:   req.QuoteDate,
		ValidUntil:  req.ValidUntil,
		Status:      QuotationStatusDraft,
		Currency:    req.Currency,
		Subtotal:    subtotal,
		TaxAmount:   taxAmount,
		TotalAmount: totalAmount,
		Notes:       req.Notes,
		CreatedBy:   createdBy,
	}

	var quotationID int64
	err = s.repo.WithTx(ctx, func(ctx context.Context, repo Repository) error {
		id, err := repo.Create(ctx, quotation)
		if err != nil {
			return fmt.Errorf("create quotation: %w", err)
		}
		quotationID = id

		for i, lineReq := range req.Lines {
			discount, tax, lineTotal := shared.CalculateLineTotals(
				lineReq.Quantity,
				lineReq.UnitPrice,
				lineReq.DiscountPercent,
				lineReq.TaxPercent,
			)

			line := QuotationLine{
				QuotationID:     quotationID,
				ProductID:       lineReq.ProductID,
				Description:     lineReq.Description,
				Quantity:        lineReq.Quantity,
				UOM:             lineReq.UOM,
				UnitPrice:       lineReq.UnitPrice,
				DiscountPercent: lineReq.DiscountPercent,
				DiscountAmount:  discount,
				TaxPercent:      lineReq.TaxPercent,
				TaxAmount:       tax,
				LineTotal:       lineTotal,
				Notes:           lineReq.Notes,
				LineOrder:       lineReq.LineOrder,
			}
			if line.LineOrder == 0 {
				line.LineOrder = i + 1
			}

			_, err := repo.InsertLine(ctx, line)
			if err != nil {
				return fmt.Errorf("insert quotation line: %w", err)
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return s.repo.Get(ctx, quotationID)
}

func (s *Service) Update(ctx context.Context, id int64, req UpdateQuotationRequest) (*Quotation, error) {
	existing, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get quotation: %w", err)
	}

	if existing.Status != QuotationStatusDraft {
		return nil, fmt.Errorf("%w: only DRAFT quotations can be updated", ErrInvalidStatus)
	}

	// Calculate new totals if lines are provided
	var subtotal, taxAmount, totalAmount float64
	var linesToInsert []QuotationLine

	if req.Lines != nil && len(*req.Lines) > 0 {
		for i, lineReq := range *req.Lines {
			discount, tax, lineTotal := shared.CalculateLineTotals(
				lineReq.Quantity,
				lineReq.UnitPrice,
				lineReq.DiscountPercent,
				lineReq.TaxPercent,
			)
			subtotal += (lineReq.Quantity * lineReq.UnitPrice) - discount
			taxAmount += tax
			totalAmount += lineTotal

			line := QuotationLine{
				QuotationID:     id,
				ProductID:       lineReq.ProductID,
				Description:     lineReq.Description,
				Quantity:        lineReq.Quantity,
				UOM:             lineReq.UOM,
				UnitPrice:       lineReq.UnitPrice,
				DiscountPercent: lineReq.DiscountPercent,
				DiscountAmount:  discount,
				TaxPercent:      lineReq.TaxPercent,
				TaxAmount:       tax,
				LineTotal:       lineTotal,
				Notes:           lineReq.Notes,
				LineOrder:       lineReq.LineOrder,
			}
			if line.LineOrder == 0 {
				line.LineOrder = i + 1
			}
			linesToInsert = append(linesToInsert, line)
		}
	} else {
		// Keep existing totals if lines not changed? 
		// Or if lines not provided, we assume checking only header update.
		// Use existing totals.
		subtotal = existing.Subtotal
		taxAmount = existing.TaxAmount
		totalAmount = existing.TotalAmount
	}

	updates := make(map[string]interface{})
	if req.QuoteDate != nil {
		updates["quote_date"] = *req.QuoteDate
	}
	if req.ValidUntil != nil {
		updates["valid_until"] = *req.ValidUntil
	}
	if req.Notes != nil {
		updates["notes"] = *req.Notes
	}
	// Always update totals if lines changed
	if req.Lines != nil {
		updates["subtotal"] = subtotal
		updates["tax_amount"] = taxAmount
		updates["total_amount"] = totalAmount
	}

	err = s.repo.WithTx(ctx, func(ctx context.Context, repo Repository) error {
		if len(updates) > 0 {
			if err := repo.Update(ctx, id, updates); err != nil {
				return err
			}
		}

		if req.Lines != nil {
			if err := repo.DeleteLines(ctx, id); err != nil {
				return err
			}
			for _, line := range linesToInsert {
				if _, err := repo.InsertLine(ctx, line); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("update quotation: %w", err)
	}

	return s.repo.Get(ctx, id)
}

func (s *Service) Submit(ctx context.Context, id int64, userID int64) (*Quotation, error) {
	existing, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get quotation: %w", err)
	}

	if existing.Status != QuotationStatusDraft {
		return nil, fmt.Errorf("%w: can only submit DRAFT quotations", ErrInvalidStatus)
	}

	err = s.repo.UpdateStatus(ctx, id, QuotationStatusSubmitted, userID, nil)
	if err != nil {
		return nil, fmt.Errorf("submit quotation: %w", err)
	}

	return s.repo.Get(ctx, id)
}

func (s *Service) Approve(ctx context.Context, id int64, approvedBy int64) (*Quotation, error) {
	existing, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get quotation: %w", err)
	}

	if existing.Status != QuotationStatusSubmitted {
		return nil, fmt.Errorf("%w: can only approve SUBMITTED quotations", ErrInvalidStatus)
	}

	err = s.repo.UpdateStatus(ctx, id, QuotationStatusApproved, approvedBy, nil)
	if err != nil {
		return nil, fmt.Errorf("approve quotation: %w", err)
	}

	return s.repo.Get(ctx, id)
}

func (s *Service) Reject(ctx context.Context, id int64, rejectedBy int64, reason string) (*Quotation, error) {
	existing, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get quotation: %w", err)
	}

	if existing.Status != QuotationStatusSubmitted {
		return nil, fmt.Errorf("%w: can only reject SUBMITTED quotations", ErrInvalidStatus)
	}

	err = s.repo.UpdateStatus(ctx, id, QuotationStatusRejected, rejectedBy, &reason)
	if err != nil {
		return nil, fmt.Errorf("reject quotation: %w", err)
	}

	return s.repo.Get(ctx, id)
}

func (s *Service) Get(ctx context.Context, id int64) (*Quotation, error) {
	return s.repo.Get(ctx, id)
}

func (s *Service) List(ctx context.Context, req ListQuotationsRequest) ([]QuotationWithDetails, int, error) {
	return s.repo.List(ctx, req)
}
