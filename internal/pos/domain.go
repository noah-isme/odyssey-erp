package pos

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

var (
	ErrNotFound      = errors.New("pos: not found")
	ErrInvalidState  = errors.New("pos: invalid state")
	ErrInvalidAmount = errors.New("pos: invalid amount")
)

type Line struct {
	ProductID      int64 `json:"product_id"`
	Quantity       int64 `json:"quantity"`
	UnitPriceCents int64 `json:"unit_price_cents"`
	DiscountCents  int64 `json:"discount_cents"`
	TaxCents       int64 `json:"tax_cents"`
}

func (l *Line) UnmarshalJSON(data []byte) error {
	type Alias Line
	var aux struct {
		Alias
		ProductIDAlt1 int64 `json:"ProductID"`
		ProductIDAlt2 int64 `json:"productId"`
		QuantityAlt   int64 `json:"Quantity"`
		UnitPriceAlt1 int64 `json:"UnitPriceCents"`
		UnitPriceAlt2 int64 `json:"unitPriceCents"`
		DiscountAlt1  int64 `json:"DiscountCents"`
		DiscountAlt2  int64 `json:"discountCents"`
		TaxAlt1       int64 `json:"TaxCents"`
		TaxAlt2       int64 `json:"taxCents"`
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	*l = Line(aux.Alias)
	if l.ProductID == 0 {
		if aux.ProductIDAlt1 != 0 {
			l.ProductID = aux.ProductIDAlt1
		} else if aux.ProductIDAlt2 != 0 {
			l.ProductID = aux.ProductIDAlt2
		}
	}
	if l.Quantity == 0 && aux.QuantityAlt != 0 {
		l.Quantity = aux.QuantityAlt
	}
	if l.UnitPriceCents == 0 {
		if aux.UnitPriceAlt1 != 0 {
			l.UnitPriceCents = aux.UnitPriceAlt1
		} else if aux.UnitPriceAlt2 != 0 {
			l.UnitPriceCents = aux.UnitPriceAlt2
		}
	}
	if l.DiscountCents == 0 {
		if aux.DiscountAlt1 != 0 {
			l.DiscountCents = aux.DiscountAlt1
		} else if aux.DiscountAlt2 != 0 {
			l.DiscountCents = aux.DiscountAlt2
		}
	}
	if l.TaxCents == 0 {
		if aux.TaxAlt1 != 0 {
			l.TaxCents = aux.TaxAlt1
		} else if aux.TaxAlt2 != 0 {
			l.TaxCents = aux.TaxAlt2
		}
	}
	return nil
}

type Ticket struct {
	ID            int64  `json:"id"`
	CompanyID     int64  `json:"company_id"`
	SessionID     int64  `json:"session_id"`
	Currency      string `json:"currency"`
	Lines         []Line `json:"lines"`
	SubtotalCents int64  `json:"subtotal_cents"`
	TaxCents      int64  `json:"tax_cents"`
	TotalCents    int64  `json:"total_cents"`
	PaidCents     int64  `json:"paid_cents"`
	Status        string `json:"status"`
}

func (t *Ticket) UnmarshalJSON(data []byte) error {
	type Alias Ticket
	var aux struct {
		Alias
		IDAlt         int64  `json:"ID"`
		CompanyIDAlt  int64  `json:"CompanyID"`
		CompanyIDAlt2 int64  `json:"companyId"`
		SessionIDAlt1 int64  `json:"SessionID"`
		SessionIDAlt2 int64  `json:"sessionId"`
		CurrencyAlt   string `json:"Currency"`
		LinesAlt      []Line `json:"Lines"`
		StatusAlt     string `json:"Status"`
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	*t = Ticket(aux.Alias)
	if t.ID == 0 && aux.IDAlt != 0 {
		t.ID = aux.IDAlt
	}
	if t.CompanyID == 0 {
		if aux.CompanyIDAlt != 0 {
			t.CompanyID = aux.CompanyIDAlt
		} else if aux.CompanyIDAlt2 != 0 {
			t.CompanyID = aux.CompanyIDAlt2
		}
	}
	if t.SessionID == 0 {
		if aux.SessionIDAlt1 != 0 {
			t.SessionID = aux.SessionIDAlt1
		} else if aux.SessionIDAlt2 != 0 {
			t.SessionID = aux.SessionIDAlt2
		}
	}
	if t.Currency == "" && aux.CurrencyAlt != "" {
		t.Currency = aux.CurrencyAlt
	}
	if len(t.Lines) == 0 && len(aux.LinesAlt) > 0 {
		t.Lines = aux.LinesAlt
	}
	if t.Status == "" && aux.StatusAlt != "" {
		t.Status = aux.StatusAlt
	}
	return nil
}

type Payment struct {
	ID             int64  `json:"id"`
	TicketID       int64  `json:"ticket_id"`
	Method         string `json:"method"`
	AmountCents    int64  `json:"amount_cents"`
	Reference      string `json:"reference"`
	IdempotencyKey string `json:"idempotency_key"`
}

func (p *Payment) UnmarshalJSON(data []byte) error {
	type Alias Payment
	var aux struct {
		Alias
		IDAlt           int64  `json:"ID"`
		TicketIDAlt1    int64  `json:"TicketID"`
		TicketIDAlt2    int64  `json:"ticketId"`
		MethodAlt       string `json:"Method"`
		AmountAlt1      int64  `json:"AmountCents"`
		AmountAlt2      int64  `json:"amountCents"`
		ReferenceAlt    string `json:"Reference"`
		IdempotencyAlt1 string `json:"IdempotencyKey"`
		IdempotencyAlt2 string `json:"idempotencyKey"`
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	*p = Payment(aux.Alias)
	if p.ID == 0 && aux.IDAlt != 0 {
		p.ID = aux.IDAlt
	}
	if p.TicketID == 0 {
		if aux.TicketIDAlt1 != 0 {
			p.TicketID = aux.TicketIDAlt1
		} else if aux.TicketIDAlt2 != 0 {
			p.TicketID = aux.TicketIDAlt2
		}
	}
	if p.Method == "" && aux.MethodAlt != "" {
		p.Method = aux.MethodAlt
	}
	if p.AmountCents == 0 {
		if aux.AmountAlt1 != 0 {
			p.AmountCents = aux.AmountAlt1
		} else if aux.AmountAlt2 != 0 {
			p.AmountCents = aux.AmountAlt2
		}
	}
	if p.Reference == "" && aux.ReferenceAlt != "" {
		p.Reference = aux.ReferenceAlt
	}
	if p.IdempotencyKey == "" {
		if aux.IdempotencyAlt1 != "" {
			p.IdempotencyKey = aux.IdempotencyAlt1
		} else if aux.IdempotencyAlt2 != "" {
			p.IdempotencyKey = aux.IdempotencyAlt2
		}
	}
	return nil
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
