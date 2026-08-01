package pos

import (
	"context"
	"github.com/stretchr/testify/require"
	"testing"
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
