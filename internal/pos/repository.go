package pos

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type SQLRepository struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) *SQLRepository { return &SQLRepository{pool: pool} }

func (r *SQLRepository) CreateTicket(ctx context.Context, ticket Ticket) (Ticket, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Ticket{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var out Ticket
	err = tx.QueryRow(ctx, `INSERT INTO pos_tickets(company_id,session_id,number,currency,subtotal,tax_amount,total,status,created_by) VALUES($1,$2,'POS-'||to_char(NOW(),'YYYYMMDDHH24MISSMS')||'-'||nextval('pos_tickets_id_seq')::text,$3,$4/100.0,$5/100.0,$6/100.0,'DRAFT',$7) RETURNING id,company_id,session_id,currency,(subtotal*100)::bigint,(tax_amount*100)::bigint,(total*100)::bigint,status`, ticket.CompanyID, ticket.SessionID, ticket.Currency, ticket.SubtotalCents, ticket.TaxCents, ticket.TotalCents, ticket.ID).Scan(&out.ID, &out.CompanyID, &out.SessionID, &out.Currency, &out.SubtotalCents, &out.TaxCents, &out.TotalCents, &out.Status)
	out.PaidCents = 0
	if err != nil {
		return Ticket{}, err
	}
	for _, line := range ticket.Lines {
		if _, err = tx.Exec(ctx, `INSERT INTO pos_ticket_lines(ticket_id,product_id,quantity,unit_price,discount,tax_amount) VALUES($1,$2,$3,$4/100.0,$5/100.0,$6/100.0)`, out.ID, line.ProductID, line.Quantity, line.UnitPriceCents, line.DiscountCents, line.TaxCents); err != nil {
			return Ticket{}, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return Ticket{}, err
	}
	return out, nil
}

func (r *SQLRepository) GetTicket(ctx context.Context, companyID, ticketID int64) (Ticket, error) {
	var t Ticket
	err := r.pool.QueryRow(ctx, `SELECT id,company_id,session_id,currency,(subtotal*100)::bigint,(tax_amount*100)::bigint,(total*100)::bigint,status FROM pos_tickets WHERE company_id=$1 AND id=$2`, companyID, ticketID).Scan(&t.ID, &t.CompanyID, &t.SessionID, &t.Currency, &t.SubtotalCents, &t.TaxCents, &t.TotalCents, &t.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		return Ticket{}, ErrNotFound
	}
	if err != nil {
		return Ticket{}, err
	}
	err = r.pool.QueryRow(ctx, `SELECT COALESCE(SUM((amount*100)::bigint),0) FROM pos_payments WHERE ticket_id=$1`, ticketID).Scan(&t.PaidCents)
	return t, err
}
func (r *SQLRepository) HasPayment(ctx context.Context, ticketID int64, key string) (bool, error) {
	var found bool
	err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pos_payments WHERE ticket_id=$1 AND idempotency_key=$2)`, ticketID, key).Scan(&found)
	return found, err
}
func (r *SQLRepository) RecordPayment(ctx context.Context, p Payment) (Payment, bool, error) {
	var out Payment
	err := r.pool.QueryRow(ctx, `WITH locked AS (SELECT id,total FROM pos_tickets WHERE id=$1 FOR UPDATE) INSERT INTO pos_payments(ticket_id,method,amount,reference,idempotency_key) SELECT locked.id,$2,$3/100.0,$4,$5 FROM locked WHERE locked.total >= (SELECT COALESCE(SUM(amount),0) FROM pos_payments WHERE ticket_id=$1)+$3/100.0 ON CONFLICT(ticket_id,idempotency_key) DO NOTHING RETURNING id,ticket_id,method,(amount*100)::bigint,reference,idempotency_key`, p.TicketID, p.Method, p.AmountCents, p.Reference, p.IdempotencyKey).Scan(&out.ID, &out.TicketID, &out.Method, &out.AmountCents, &out.Reference, &out.IdempotencyKey)
	if errors.Is(err, pgx.ErrNoRows) {
		err = r.pool.QueryRow(ctx, `SELECT id,ticket_id,method,(amount*100)::bigint,reference,idempotency_key FROM pos_payments WHERE ticket_id=$1 AND idempotency_key=$2`, p.TicketID, p.IdempotencyKey).Scan(&out.ID, &out.TicketID, &out.Method, &out.AmountCents, &out.Reference, &out.IdempotencyKey)
		return out, true, err
	}
	return out, false, err
}
func (r *SQLRepository) UpdateTicket(ctx context.Context, t Ticket) error {
	res, err := r.pool.Exec(ctx, `UPDATE pos_tickets SET status=$1 WHERE company_id=$2 AND id=$3`, t.Status, t.CompanyID, t.ID)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// =============================================================================
// Advanced POS Features (Hardware, Loyalty, Gift Cards, Split Tender)
// =============================================================================

func (r *SQLRepository) CreatePOSHardware(ctx context.Context, h POSHardware) (POSHardware, error) {
	err := r.pool.QueryRow(ctx, `INSERT INTO pos_hardware (terminal_id, device_type, device_ip, device_config, status) VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		h.TerminalID, h.DeviceType, h.DeviceIP, h.DeviceConfig, h.Status).Scan(&h.ID)
	return h, err
}

func (r *SQLRepository) CreateLoyaltyMember(ctx context.Context, lm LoyaltyMember) (LoyaltyMember, error) {
	err := r.pool.QueryRow(ctx, `INSERT INTO pos_loyalty_members (company_id, customer_name, phone, points, tier) VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		lm.CompanyID, lm.CustomerName, lm.Phone, lm.Points, lm.Tier).Scan(&lm.ID)
	return lm, err
}

func (r *SQLRepository) CreateGiftCard(ctx context.Context, gc GiftCard) (GiftCard, error) {
	err := r.pool.QueryRow(ctx, `INSERT INTO pos_gift_cards (company_id, code, balance, currency, status) VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		gc.CompanyID, gc.Code, gc.Balance, gc.Currency, gc.Status).Scan(&gc.ID)
	return gc, err
}

var _ Repository = (*SQLRepository)(nil)
