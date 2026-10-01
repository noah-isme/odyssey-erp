package pos

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

type memoryRepo struct {
	ticket   Ticket
	payments map[string]Payment
	updated  []Ticket
}

func (r *memoryRepo) CreateTicket(_ context.Context, t Ticket) (Ticket, error) {
	r.ticket = t
	r.ticket.ID = 1
	return r.ticket, nil
}
func (r *memoryRepo) GetTicket(_ context.Context, _ int64, _ int64) (Ticket, error) {
	return r.ticket, nil
}
func (r *memoryRepo) HasPayment(_ context.Context, _ int64, key string) (bool, error) {
	_, ok := r.payments[key]
	return ok, nil
}
func (r *memoryRepo) RecordPayment(_ context.Context, p Payment) (Payment, bool, error) {
	if old, ok := r.payments[p.IdempotencyKey]; ok {
		return old, true, nil
	}
	p.ID = int64(len(r.payments) + 1)
	r.payments[p.IdempotencyKey] = p
	r.ticket.PaidCents += p.AmountCents
	return p, false, nil
}
func (r *memoryRepo) UpdateTicket(_ context.Context, t Ticket) error {
	r.ticket = t
	r.updated = append(r.updated, t)
	return nil
}

func (r *memoryRepo) CreatePOSHardware(_ context.Context, h POSHardware) (POSHardware, error) { return h, nil }
func (r *memoryRepo) CreateLoyaltyMember(_ context.Context, m LoyaltyMember) (LoyaltyMember, error) { return m, nil }
func (r *memoryRepo) CreateGiftCard(_ context.Context, g GiftCard) (GiftCard, error) { return g, nil }
func TestTicketPaymentAndRefund(t *testing.T) {
	r := &memoryRepo{payments: map[string]Payment{}}
	s := NewService(r)
	ticket, e := s.CreateTicket(context.Background(), Ticket{CompanyID: 1, SessionID: 2, Lines: []Line{{ProductID: 4, Quantity: 1, UnitPriceCents: 1000}}})
	require.NoError(t, e)
	_, e = s.AddPayment(context.Background(), 1, ticket.ID, Payment{Method: "CASH", AmountCents: 1000, IdempotencyKey: "p1"})
	require.NoError(t, e)
	require.Equal(t, "COMPLETED", r.ticket.Status)
	_, e = s.AddPayment(context.Background(), 1, ticket.ID, Payment{Method: "CASH", AmountCents: 1000, IdempotencyKey: "p1"})
	require.NoError(t, e)
	tkt, e := s.Refund(context.Background(), 1, ticket.ID)
	require.NoError(t, e)
	require.Equal(t, "REFUNDED", tkt.Status)
}

func TestTicketJSONSerialization(t *testing.T) {
	// Test snake_case parsing
	snakeJSON := `{"session_id": 42, "currency": "IDR", "lines": [{"product_id": 10, "quantity": 2, "unit_price_cents": 5000, "discount_cents": 500, "tax_cents": 100}]}`
	var tkt1 Ticket
	err := json.Unmarshal([]byte(snakeJSON), &tkt1)
	require.NoError(t, err)
	require.Equal(t, int64(42), tkt1.SessionID)
	require.Equal(t, "IDR", tkt1.Currency)
	require.Len(t, tkt1.Lines, 1)
	require.Equal(t, int64(10), tkt1.Lines[0].ProductID)
	require.Equal(t, int64(2), tkt1.Lines[0].Quantity)
	require.Equal(t, int64(5000), tkt1.Lines[0].UnitPriceCents)
	require.Equal(t, int64(500), tkt1.Lines[0].DiscountCents)
	require.Equal(t, int64(100), tkt1.Lines[0].TaxCents)

	// Test Payment snake_case parsing
	payJSON := `{"amount_cents": 9600, "method": "CASH", "idempotency_key": "k-123", "reference": "REF-01"}`
	var p1 Payment
	err = json.Unmarshal([]byte(payJSON), &p1)
	require.NoError(t, err)
	require.Equal(t, int64(9600), p1.AmountCents)
	require.Equal(t, "CASH", p1.Method)
	require.Equal(t, "k-123", p1.IdempotencyKey)
	require.Equal(t, "REF-01", p1.Reference)
}
