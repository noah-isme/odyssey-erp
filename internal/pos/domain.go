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

	CreatePOSHardware(context.Context, POSHardware) (POSHardware, error)
	CreateLoyaltyMember(context.Context, LoyaltyMember) (LoyaltyMember, error)
	CreateGiftCard(context.Context, GiftCard) (GiftCard, error)
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

// =============================================================================
// Advanced POS Features (Hardware, Loyalty, Gift Cards, Split Tender)
// =============================================================================

type POSHardware struct {
	ID           int64
	TerminalID   int64
	DeviceType   string // PRINTER, SCANNER
	DeviceIP     string
	DeviceConfig string
	Status       string
}

type LoyaltyMember struct {
	ID           int64
	CompanyID    int64
	CustomerName string
	Phone        string
	Points       int64
	Tier         string
}

type GiftCard struct {
	ID        int64
	CompanyID int64
	Code      string
	Balance   float64
	Currency  string
	Status    string
}

func (s *Service) CreatePOSHardware(ctx context.Context, h POSHardware) (POSHardware, error) {
	if s == nil || s.repo == nil {
		return POSHardware{}, errors.New("pos: repository is required")
	}
	if h.TerminalID == 0 || h.DeviceType == "" {
		return POSHardware{}, errors.New("pos: terminal_id and device_type are required")
	}
	h.Status = "ONLINE"
	return s.repo.CreatePOSHardware(ctx, h)
}

func (s *Service) CreateLoyaltyMember(ctx context.Context, lm LoyaltyMember) (LoyaltyMember, error) {
	if s == nil || s.repo == nil {
		return LoyaltyMember{}, errors.New("pos: repository is required")
	}
	if lm.CompanyID == 0 || lm.CustomerName == "" || lm.Phone == "" {
		return LoyaltyMember{}, errors.New("pos: invalid loyalty member data")
	}
	if lm.Tier == "" {
		lm.Tier = "STANDARD"
	}
	return s.repo.CreateLoyaltyMember(ctx, lm)
}

func (s *Service) CreateGiftCard(ctx context.Context, gc GiftCard) (GiftCard, error) {
	if s == nil || s.repo == nil {
		return GiftCard{}, errors.New("pos: repository is required")
	}
	if gc.CompanyID == 0 || gc.Code == "" {
		return GiftCard{}, errors.New("pos: invalid gift card data")
	}
	gc.Status = "ACTIVE"
	if gc.Currency == "" {
		gc.Currency = "IDR"
	}
	return s.repo.CreateGiftCard(ctx, gc)
}
