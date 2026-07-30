package ap

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/odyssey-erp/odyssey-erp/internal/procurement"
)

var (
	ErrDebitNoteNotFound   = errors.New("ap: debit note not found")
	ErrDebitExceedsBalance = errors.New("ap: debit note exceeds invoice balance")
)

func (s *Service) CreateAPDebitNoteFromReturn(ctx context.Context, input CreateAPDebitNoteFromReturnInput) (*APDebitNote, error) {
	if s.procurementService == nil {
		return nil, errors.New("ap: procurement service not configured")
	}
	returned, err := s.procurementService.GetGoodsReturnGRN(ctx, input.GoodsReturnGRNID)
	if err != nil {
		return nil, err
	}
	if returned.Status != procurement.GoodsReturnStatusConfirmed {
		return nil, errors.New("ap: goods return must be confirmed")
	}
	invoices, err := s.repo.ListAPInvoices(ctx, ListAPInvoicesRequest{SupplierID: returned.SupplierID})
	if err != nil {
		return nil, err
	}
	var invoice APInvoice
	for _, candidate := range invoices {
		if candidate.GRNID != nil && *candidate.GRNID == returned.GRNID {
			invoice = candidate
			break
		}
	}
	if invoice.ID == 0 {
		return nil, errors.New("ap: no invoice exists for returned GRN")
	}
	if invoice.Status != APStatusPosted && invoice.Status != APStatusPaid {
		return nil, ErrInvalidStatus
	}
	details, err := s.repo.GetAPInvoiceWithDetails(ctx, invoice.ID)
	if err != nil {
		return nil, err
	}
	invoiceLines := make(map[int64]APInvoiceLine, len(details.Lines))
	for _, line := range details.Lines {
		if line.GRNLineID != nil {
			invoiceLines[*line.GRNLineID] = line
		}
	}
	lines := make([]CreateAPDebitNoteLineInput, 0, len(returned.Lines))
	for _, returnedLine := range returned.Lines {
		invoiceLine, ok := invoiceLines[returnedLine.GRNLineID]
		if !ok || invoiceLine.ProductID != returnedLine.ProductID {
			return nil, fmt.Errorf("ap: returned GRN line %d is not invoiced", returnedLine.GRNLineID)
		}
		if returnedLine.QuantityReturned > invoiceLine.Quantity {
			return nil, errors.New("ap: returned quantity exceeds invoiced quantity")
		}
		invoiceLineID := invoiceLine.ID
		returnLineID := returnedLine.ID
		lines = append(lines, CreateAPDebitNoteLineInput{APInvoiceLineID: &invoiceLineID, GoodsReturnGRNLineID: &returnLineID, ProductID: returnedLine.ProductID, Description: invoiceLine.Description, Quantity: returnedLine.QuantityReturned, UnitPrice: invoiceLine.UnitPrice, DiscountPct: invoiceLine.DiscountPct, TaxPct: invoiceLine.TaxPct})
	}
	returnID := returned.ID
	return s.repo.CreateAPDebitNote(ctx, CreateAPDebitNoteInput{SupplierID: invoice.SupplierID, APInvoiceID: invoice.ID, GoodsReturnGRNID: &returnID, Currency: invoice.Currency, Reason: input.Reason, CreatedBy: input.CreatedBy, Lines: lines})
}

func (s *Service) GetAPDebitNoteWithDetails(ctx context.Context, id int64) (*APDebitNoteWithDetails, error) {
	return s.repo.GetAPDebitNoteWithDetails(ctx, id)
}

func (s *Service) ListAPDebitNotes(ctx context.Context, req ListAPDebitNotesRequest) ([]APDebitNote, error) {
	if req.Limit == 0 {
		req.Limit = 50
	}
	return s.repo.ListAPDebitNotes(ctx, req)
}

func (s *Service) PostAPDebitNote(ctx context.Context, input PostAPDebitNoteInput) error {
	note, err := s.repo.GetAPDebitNote(ctx, input.DebitNoteID)
	if err != nil {
		return err
	}
	if note.Status == APDebitNoteStatusPosted {
		if s.tax != nil {
			return s.tax.RecordAPDebitNote(ctx, note.ID, input.PostedBy)
		}
		return nil
	}
	if note.Status != APDebitNoteStatusDraft {
		return ErrInvalidStatus
	}
	invoice, err := s.repo.GetAPInvoice(ctx, note.APInvoiceID)
	if err != nil {
		return err
	}
	if invoice.Status != APStatusPosted && invoice.Status != APStatusPaid {
		return ErrInvalidStatus
	}
	if err := s.repo.PostAPDebitNote(ctx, note.ID, invoice.ID, input.PostedBy, note.Total); err != nil {
		return err
	}
	if s.integration != nil {
		var grnID int64
		if invoice.GRNID != nil {
			grnID = *invoice.GRNID
		}
		if err = s.integration.HandleDebitNotePosted(ctx, procurement.DebitNotePostedEvent{ID: note.ID, Number: note.Number, SupplierID: note.SupplierID, APInvoiceID: invoice.ID, GRNID: grnID, Total: note.Total, Subtotal: note.Subtotal, TaxAmount: note.TaxAmount, PostedAt: time.Now()}); err != nil {
			return err
		}
	}
	if s.tax != nil {
		return s.tax.RecordAPDebitNote(ctx, note.ID, input.PostedBy)
	}
	return nil
}

func (s *Service) VoidAPDebitNote(ctx context.Context, input VoidAPDebitNoteInput) error {
	if input.VoidReason == "" {
		return errors.New("ap: void reason required")
	}
	note, err := s.repo.GetAPDebitNote(ctx, input.DebitNoteID)
	if err != nil {
		return err
	}
	if note.Status != APDebitNoteStatusDraft {
		return ErrInvalidStatus
	}
	return s.repo.VoidAPDebitNote(ctx, note.ID, input.VoidedBy, input.VoidReason)
}
