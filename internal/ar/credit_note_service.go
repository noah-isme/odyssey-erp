package ar

import (
	"context"
	"errors"
	"fmt"
)

var (
	ErrCreditNoteNotFound   = errors.New("ar: credit note not found")
	ErrCreditExceedsBalance = errors.New("ar: credit note exceeds invoice balance")
)

type CreditNoteRepositoryPort interface {
	CreateARCreditNote(ctx context.Context, input CreateARCreditNoteInput) (*ARCreditNote, error)
	GetARCreditNote(ctx context.Context, id int64) (*ARCreditNote, error)
	GetARCreditNoteWithDetails(ctx context.Context, id int64) (*ARCreditNoteWithDetails, error)
	ListARCreditNotes(ctx context.Context, req ListARCreditNotesRequest) ([]ARCreditNote, error)
	PostARCreditNote(ctx context.Context, id, invoiceID, postedBy int64, amount float64) error
	VoidARCreditNote(ctx context.Context, id, voidedBy int64, reason string) error
	GenerateCreditNoteNumber(ctx context.Context) (string, error)
	GetInvoiceCreditAvailable(ctx context.Context, invoiceID int64) (float64, error)
}

type ReturnDeliveryServicePort interface {
	GetReturnForCreditNote(ctx context.Context, id int64) (*ReturnDeliveryInfo, error)
}

type ReturnDeliveryInfo struct {
	ID                      int64
	OriginalDeliveryOrderID int64
	CustomerID              int64
	Status                  string
	Lines                   []ReturnDeliveryLineInfo
}

type ReturnDeliveryLineInfo struct {
	ID                  int64
	DeliveryOrderLineID int64
	ProductID           int64
	Quantity            float64
}

func (s *Service) SetReturnDeliveryService(delivery ReturnDeliveryServicePort) {
	s.returnDelivery = delivery
}

func (s *Service) CreateARCreditNoteFromReturn(ctx context.Context, input CreateARCreditNoteFromReturnInput) (*ARCreditNote, error) {
	if s.creditNotes == nil || s.returnDelivery == nil {
		return nil, errors.New("ar: credit note service not configured")
	}

	returned, err := s.returnDelivery.GetReturnForCreditNote(ctx, input.ReturnDeliveryOrderID)
	if err != nil {
		return nil, err
	}
	if returned.Status != "CONFIRMED" {
		return nil, errors.New("ar: return delivery order must be confirmed")
	}

	invoice, err := s.repo.GetARInvoiceByDelivery(ctx, returned.OriginalDeliveryOrderID)
	if err != nil {
		return nil, err
	}
	if invoice.CustomerID != returned.CustomerID {
		return nil, errors.New("ar: return customer does not match invoice customer")
	}

	details, err := s.repo.GetARInvoiceWithDetails(ctx, invoice.ID)
	if err != nil {
		return nil, err
	}
	invoiceLines := make(map[int64]ARInvoiceLine, len(details.Lines))
	for _, line := range details.Lines {
		invoiceLines[line.DeliveryOrderLineID] = line
	}

	lines := make([]CreateARCreditNoteLineInput, 0, len(returned.Lines))
	for _, returnedLine := range returned.Lines {
		invoiceLine, ok := invoiceLines[returnedLine.DeliveryOrderLineID]
		if !ok || invoiceLine.ProductID != returnedLine.ProductID {
			return nil, fmt.Errorf("ar: returned delivery line %d is not invoiced", returnedLine.DeliveryOrderLineID)
		}
		if returnedLine.Quantity > invoiceLine.Quantity {
			return nil, errors.New("ar: returned quantity exceeds invoiced quantity")
		}
		lines = append(lines, CreateARCreditNoteLineInput{
			ARInvoiceLineID:           invoiceLine.ID,
			ReturnDeliveryOrderLineID: returnedLine.ID,
			ProductID:                 returnedLine.ProductID,
			Description:               invoiceLine.Description,
			Quantity:                  returnedLine.Quantity,
			UnitPrice:                 invoiceLine.UnitPrice,
			DiscountPct:               invoiceLine.DiscountPct,
			TaxPct:                    invoiceLine.TaxPct,
		})
	}

	number, err := s.creditNotes.GenerateCreditNoteNumber(ctx)
	if err != nil {
		return nil, err
	}
	return s.creditNotes.CreateARCreditNote(ctx, CreateARCreditNoteInput{
		CustomerID:            invoice.CustomerID,
		ARInvoiceID:           invoice.ID,
		ReturnDeliveryOrderID: returned.ID,
		Number:                number,
		Currency:              invoice.Currency,
		Reason:                input.Reason,
		CreatedBy:             input.CreatedBy,
		Lines:                 lines,
	})
}

func (s *Service) GetARCreditNoteWithDetails(ctx context.Context, id int64) (*ARCreditNoteWithDetails, error) {
	if s.creditNotes == nil {
		return nil, errors.New("ar: credit note service not configured")
	}
	return s.creditNotes.GetARCreditNoteWithDetails(ctx, id)
}

func (s *Service) ListARCreditNotes(ctx context.Context, req ListARCreditNotesRequest) ([]ARCreditNote, error) {
	if s.creditNotes == nil {
		return nil, errors.New("ar: credit note service not configured")
	}
	if req.Limit == 0 {
		req.Limit = 50
	}
	return s.creditNotes.ListARCreditNotes(ctx, req)
}

func (s *Service) PostARCreditNote(ctx context.Context, input PostARCreditNoteInput) error {
	if s.creditNotes == nil {
		return errors.New("ar: credit note service not configured")
	}
	creditNote, err := s.creditNotes.GetARCreditNote(ctx, input.CreditNoteID)
	if err != nil {
		return err
	}
	if creditNote == nil {
		return ErrCreditNoteNotFound
	}
	if creditNote.Status == ARCreditNoteStatusPosted {
		if s.tax != nil {
			return s.tax.RecordARCreditNote(ctx, creditNote.ID, input.PostedBy)
		}
		return nil
	}
	if creditNote.Status != ARCreditNoteStatusDraft {
		return ErrInvalidStatus
	}

	invoice, err := s.repo.GetARInvoice(ctx, creditNote.ARInvoiceID)
	if err != nil {
		return err
	}
	if invoice.Status != ARStatusPosted && invoice.Status != ARStatusPaid {
		return ErrInvalidStatus
	}
	available, err := s.creditNotes.GetInvoiceCreditAvailable(ctx, invoice.ID)
	if err != nil {
		return err
	}
	if creditNote.Total > available {
		return ErrCreditExceedsBalance
	}
	if err := s.creditNotes.PostARCreditNote(ctx, creditNote.ID, invoice.ID, input.PostedBy, creditNote.Total); err != nil {
		return err
	}
	creditNote.Status = ARCreditNoteStatusPosted
	postedBy := input.PostedBy
	creditNote.PostedBy = &postedBy
	if s.accounting != nil {
		if err = s.accounting.CreateARCreditNoteJournal(ctx, creditNote); err != nil {
			return err
		}
	}
	if s.tax != nil {
		err = s.tax.RecordARCreditNote(ctx, creditNote.ID, input.PostedBy)
		return err
	}
	return nil
}

func (s *Service) VoidARCreditNote(ctx context.Context, input VoidARCreditNoteInput) error {
	if s.creditNotes == nil {
		return errors.New("ar: credit note service not configured")
	}
	creditNote, err := s.creditNotes.GetARCreditNote(ctx, input.CreditNoteID)
	if err != nil {
		return err
	}
	if creditNote.Status != ARCreditNoteStatusDraft {
		return ErrInvalidStatus
	}
	if input.VoidReason == "" {
		return errors.New("ar: void reason required")
	}
	return s.creditNotes.VoidARCreditNote(ctx, creditNote.ID, input.VoidedBy, input.VoidReason)
}
