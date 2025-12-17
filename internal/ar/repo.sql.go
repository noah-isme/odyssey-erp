package ar

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository provides PostgreSQL backed persistence.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository constructs a repository.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// ListAROutstanding returns posted invoices with remaining balance.
func (r *Repository) ListAROutstanding(ctx context.Context) ([]ARInvoice, error) {
	rows, err := r.pool.Query(ctx, `SELECT id, number, customer_id, so_id, currency, total, status, due_at, created_at, updated_at FROM ar_invoices WHERE status IN ('POSTED','PAID') ORDER BY due_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var invoices []ARInvoice
	for rows.Next() {
		var inv ARInvoice
		if err := rows.Scan(&inv.ID, &inv.Number, &inv.CustomerID, &inv.SOID, &inv.Currency, &inv.Total, &inv.Status, &inv.DueAt, &inv.CreatedAt, &inv.UpdatedAt); err != nil {
			return nil, err
		}
		invoices = append(invoices, inv)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return invoices, nil
}

// TxRepository exposes transactional operations.
type TxRepository interface {
	CreateARInvoice(ctx context.Context, inv ARInvoice) (int64, error)
	UpdateARStatus(ctx context.Context, id int64, status ARInvoiceStatus) error
	CreateARPayment(ctx context.Context, payment ARPayment) (int64, error)
}

type txRepo struct {
	tx pgx.Tx
}

// WithTx wraps callback in repeatable-read transaction.
func (r *Repository) WithTx(ctx context.Context, fn func(context.Context, TxRepository) error) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return err
	}
	wrapper := &txRepo{tx: tx}
	if err := fn(ctx, wrapper); err != nil {
		_ = tx.Rollback(ctx)
		return err
	}
	return tx.Commit(ctx)
}

// ErrNotFound indicates resource not found.
var ErrNotFound = errors.New("ar: not found")

// CreateARInvoice creates a new AR invoice.
func (r *Repository) CreateARInvoice(ctx context.Context, input ARInvoiceInput) (*ARInvoice, error) {
	var id int64
	err := r.pool.QueryRow(ctx, `INSERT INTO ar_invoices (number, customer_id, so_id, currency, total, status, due_at, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) RETURNING id`, input.Number, input.CustomerID, input.SOID, input.Currency, input.Total, ARStatusDraft, input.DueDate, input.CreatedAt, input.UpdatedAt).Scan(&id)
	if err != nil {
		return nil, err
	}
	return &ARInvoice{
		ID:         id,
		Number:     input.Number,
		CustomerID: input.CustomerID,
		SOID:       input.SOID,
		Currency:   input.Currency,
		Total:      input.Total,
		Status:     ARStatusDraft,
		DueAt:      input.DueDate,
		CreatedAt:  input.CreatedAt,
		UpdatedAt:  input.UpdatedAt,
	}, nil
}

// CreateARPayment creates a new AR payment.
func (r *Repository) CreateARPayment(ctx context.Context, input ARPaymentInput) (*ARPayment, error) {
	var id int64
	err := r.pool.QueryRow(ctx, `INSERT INTO ar_payments (number, ar_invoice_id, amount, paid_at, method, note, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING id`, input.Number, input.ARInvoiceID, input.Amount, input.PaidAt, input.Method, input.Note, input.CreatedAt, input.UpdatedAt).Scan(&id)
	if err != nil {
		return nil, err
	}
	return &ARPayment{
		ID:          id,
		Number:      input.Number,
		ARInvoiceID: input.ARInvoiceID,
		Amount:      input.Amount,
		PaidAt:      input.PaidAt,
		Method:      input.Method,
		Note:        input.Note,
		CreatedAt:   input.CreatedAt,
		UpdatedAt:   input.UpdatedAt,
	}, nil
}

// ListARInvoices returns all AR invoices.
func (r *Repository) ListARInvoices(ctx context.Context) ([]ARInvoice, error) {
	rows, err := r.pool.Query(ctx, `SELECT id, number, customer_id, so_id, currency, total, status, due_at, created_at, updated_at FROM ar_invoices ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var invoices []ARInvoice
	for rows.Next() {
		var inv ARInvoice
		if err := rows.Scan(&inv.ID, &inv.Number, &inv.CustomerID, &inv.SOID, &inv.Currency, &inv.Total, &inv.Status, &inv.DueAt, &inv.CreatedAt, &inv.UpdatedAt); err != nil {
			return nil, err
		}
		invoices = append(invoices, inv)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return invoices, nil
}

// ListARPayments returns all AR payments.
func (r *Repository) ListARPayments(ctx context.Context) ([]ARPayment, error) {
	rows, err := r.pool.Query(ctx, `SELECT id, number, ar_invoice_id, amount, paid_at, method, note, created_at, updated_at FROM ar_payments ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var payments []ARPayment
	for rows.Next() {
		var pay ARPayment
		if err := rows.Scan(&pay.ID, &pay.Number, &pay.ARInvoiceID, &pay.Amount, &pay.PaidAt, &pay.Method, &pay.Note, &pay.CreatedAt, &pay.UpdatedAt); err != nil {
			return nil, err
		}
		payments = append(payments, pay)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return payments, nil
}

func (tx *txRepo) CreateARInvoice(ctx context.Context, inv ARInvoice) (int64, error) {
	var id int64
	err := tx.tx.QueryRow(ctx, `INSERT INTO ar_invoices (number, customer_id, so_id, currency, total, status, due_at, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) RETURNING id`, inv.Number, inv.CustomerID, inv.SOID, inv.Currency, inv.Total, inv.Status, inv.DueAt, inv.CreatedAt, inv.UpdatedAt).Scan(&id)
	return id, err
}

func (tx *txRepo) UpdateARStatus(ctx context.Context, id int64, status ARInvoiceStatus) error {
	_, err := tx.tx.Exec(ctx, `UPDATE ar_invoices SET status=$1 WHERE id=$2`, status, id)
	return err
}

func (tx *txRepo) CreateARPayment(ctx context.Context, payment ARPayment) (int64, error) {
	var id int64
	err := tx.tx.QueryRow(ctx, `INSERT INTO ar_payments (number, ar_invoice_id, amount, paid_at, method, note, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING id`, payment.Number, payment.ARInvoiceID, payment.Amount, payment.PaidAt, payment.Method, payment.Note, payment.CreatedAt, payment.UpdatedAt).Scan(&id)
	return id, err
}
