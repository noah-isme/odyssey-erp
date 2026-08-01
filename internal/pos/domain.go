package pos

import (
	"context"
	"errors"
	"fmt"
)

var (
	ErrNotFound      = errors.New("pos: not found")
	ErrInvalidState  = errors.New("pos: invalid state")
	ErrInvalidAmount = errors.New("pos: invalid amount")
)

type Line struct {
	ProductID                               int64
	Quantity                                int64
	UnitPriceCents, DiscountCents, TaxCents int64
}
type Ticket struct {
	ID, CompanyID, SessionID                       int64
	Currency                                       string
	Lines                                          []Line
	SubtotalCents, TaxCents, TotalCents, PaidCents int64
	Status                                         string
}
type Payment struct {
	ID, TicketID              int64
	Method                    string
	AmountCents               int64
	Reference, IdempotencyKey string
}

type Repository interface {
	CreateTicket(context.Context, Ticket) (Ticket, error)
	GetTicket(context.Context, int64, int64) (Ticket, error)
	HasPayment(context.Context, int64, string) (bool, error)
	RecordPayment(context.Context, Payment) (Payment, bool, error)
	UpdateTicket(context.Context, Ticket) error
}

type Service struct{ repo Repository }

func NewService(repo Repository) *Service { return &Service{repo: repo} }

func (s *Service) CreateTicket(ctx context.Context, ticket Ticket) (Ticket, error) {
	if ticket.CompanyID == 0 || ticket.SessionID == 0 || len(ticket.Lines) == 0 {
		return Ticket{}, ErrInvalidAmount
	}
	ticket.SubtotalCents, ticket.TaxCents = 0, 0
	for _, line := range ticket.Lines {
		if line.ProductID == 0 || line.Quantity <= 0 || line.UnitPriceCents < 0 {
			return Ticket{}, ErrInvalidAmount
		}
		lineTotal := line.Quantity*line.UnitPriceCents - line.DiscountCents + line.TaxCents
		if lineTotal < 0 {
			return Ticket{}, ErrInvalidAmount
		}
		ticket.SubtotalCents += line.Quantity*line.UnitPriceCents - line.DiscountCents
		ticket.TaxCents += line.TaxCents
	}
	ticket.TotalCents = ticket.SubtotalCents + ticket.TaxCents
	ticket.Status = "DRAFT"
	return s.repo.CreateTicket(ctx, ticket)
}

func (s *Service) AddPayment(ctx context.Context, companyID, ticketID int64, payment Payment) (Payment, error) {
	ticket, err := s.repo.GetTicket(ctx, companyID, ticketID)
	if err != nil {
		return Payment{}, err
	}
	if payment.AmountCents <= 0 || payment.IdempotencyKey == "" {
		return Payment{}, ErrInvalidAmount
	}
	seen, err := s.repo.HasPayment(ctx, ticketID, payment.IdempotencyKey)
	if err != nil {
		return Payment{}, err
	}
	if seen {
		created, _, err := s.repo.RecordPayment(ctx, Payment{TicketID: ticketID, IdempotencyKey: payment.IdempotencyKey})
		return created, err
	}
	if ticket.Status != "DRAFT" {
		return Payment{}, fmt.Errorf("%w: ticket is %s", ErrInvalidState, ticket.Status)
	}
	if ticket.PaidCents+payment.AmountCents > ticket.TotalCents {
		return Payment{}, ErrInvalidAmount
	}
	payment.TicketID = ticketID
	created, duplicate, err := s.repo.RecordPayment(ctx, payment)
	if err != nil {
		return Payment{}, err
	}
	if duplicate {
		return created, nil
	}
	// Re-read after the atomic insert so concurrent payments cannot overwrite
	// the ticket with a stale paid total or leave a fully paid ticket open.
	current, err := s.repo.GetTicket(ctx, companyID, ticketID)
	if err != nil {
		return Payment{}, err
	}
	ticket = current
	if ticket.PaidCents == ticket.TotalCents {
		ticket.Status = "COMPLETED"
	}
	if err := s.repo.UpdateTicket(ctx, ticket); err != nil {
		return Payment{}, err
	}
	return created, nil
}

func (s *Service) Refund(ctx context.Context, companyID, ticketID int64) (Ticket, error) {
	ticket, err := s.repo.GetTicket(ctx, companyID, ticketID)
	if err != nil {
		return Ticket{}, err
	}
	if ticket.Status != "COMPLETED" {
		return Ticket{}, ErrInvalidState
	}
	ticket.Status = "REFUNDED"
	if err := s.repo.UpdateTicket(ctx, ticket); err != nil {
		return Ticket{}, err
	}
	return ticket, nil
}
