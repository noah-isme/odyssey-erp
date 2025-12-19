package ar

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/odyssey-erp/odyssey-erp/internal/ar/db"
)

// Repository provides PostgreSQL backed persistence.
type Repository struct {
	pool    *pgxpool.Pool
	queries *ardb.Queries
}

// NewRepository constructs a repository.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{
		pool:    pool,
		queries: ardb.New(pool),
	}
}

// ErrNotFound indicates resource not found.
var ErrNotFound = errors.New("ar: not found")

// TxRepository exposes transactional operations.
type TxRepository interface {
	CreateARInvoice(ctx context.Context, inv ARInvoiceInput) (int64, error)
	UpdateARStatus(ctx context.Context, id int64, status ARInvoiceStatus) error
	CreateARPayment(ctx context.Context, payment ARPaymentInput) (int64, error)
}

type txRepo struct {
	queries *ardb.Queries
}

// WithTx wraps callback in repeatable-read transaction.
func (r *Repository) WithTx(ctx context.Context, fn func(context.Context, TxRepository) error) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	qtx := r.queries.WithTx(tx)
	wrapper := &txRepo{queries: qtx}
	
	if err := fn(ctx, wrapper); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ListAROutstanding returns posted invoices with remaining balance.
func (r *Repository) ListAROutstanding(ctx context.Context) ([]ARInvoice, error) {
	rows, err := r.queries.ListAROutstanding(ctx)
	if err != nil {
		return nil, err
	}
	invoices := make([]ARInvoice, len(rows))
	for i, row := range rows {
		invoices[i] = mapARInvoice(row)
	}
	return invoices, nil
}

// CreateARInvoice creates a new AR invoice.
func (r *Repository) CreateARInvoice(ctx context.Context, input ARInvoiceInput) (*ARInvoice, error) {
	id, err := r.queries.CreateARInvoice(ctx, ardb.CreateARInvoiceParams{
		Number:     input.Number,
		CustomerID: input.CustomerID,
		SoID:       int8FromInt64(input.SOID),
		Currency:   input.Currency,
		Total:      float64ToNumeric(input.Total),
		Status:     string(ARStatusDraft),
		DueAt:      pgtype.Timestamptz{Time: input.DueDate, Valid: true},
		CreatedAt:  pgtype.Timestamptz{Time: input.CreatedAt, Valid: true},
		UpdatedAt:  pgtype.Timestamptz{Time: input.UpdatedAt, Valid: true},
	})
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
	id, err := r.queries.CreateARPayment(ctx, ardb.CreateARPaymentParams{
		Number:      input.Number,
		ArInvoiceID: input.ARInvoiceID,
		Amount:      float64ToNumeric(input.Amount),
		PaidAt:      pgtype.Timestamptz{Time: input.PaidAt, Valid: true},
		Method:      input.Method,
		Note:        input.Note,
		CreatedAt:   pgtype.Timestamptz{Time: input.CreatedAt, Valid: true},
		UpdatedAt:   pgtype.Timestamptz{Time: input.UpdatedAt, Valid: true},
	})
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
	rows, err := r.queries.ListARInvoices(ctx)
	if err != nil {
		return nil, err
	}
	invoices := make([]ARInvoice, len(rows))
	for i, row := range rows {
		invoices[i] = mapARInvoice(row)
	}
	return invoices, nil
}

// ListARPayments returns all AR payments.
func (r *Repository) ListARPayments(ctx context.Context) ([]ARPayment, error) {
	rows, err := r.queries.ListARPayments(ctx)
	if err != nil {
		return nil, err
	}
	payments := make([]ARPayment, len(rows))
	for i, row := range rows {
		payments[i] = mapARPayment(row)
	}
	return payments, nil
}

// Transactional methods used within WithTx closure

func (tx *txRepo) CreateARInvoice(ctx context.Context, input ARInvoiceInput) (int64, error) {
	return tx.queries.CreateARInvoice(ctx, ardb.CreateARInvoiceParams{
		Number:     input.Number,
		CustomerID: input.CustomerID,
		SoID:       int8FromInt64(input.SOID),
		Currency:   input.Currency,
		Total:      float64ToNumeric(input.Total),
		Status:     string(ARStatusDraft),
		DueAt:      pgtype.Timestamptz{Time: input.DueDate, Valid: true},
		CreatedAt:  pgtype.Timestamptz{Time: input.CreatedAt, Valid: true},
		UpdatedAt:  pgtype.Timestamptz{Time: input.UpdatedAt, Valid: true},
	})
}

func (tx *txRepo) UpdateARStatus(ctx context.Context, id int64, status ARInvoiceStatus) error {
	return tx.queries.UpdateARStatus(ctx, ardb.UpdateARStatusParams{
		ID:     id,
		Status: string(status),
	})
}

func (tx *txRepo) CreateARPayment(ctx context.Context, input ARPaymentInput) (int64, error) {
	return tx.queries.CreateARPayment(ctx, ardb.CreateARPaymentParams{
		Number:      input.Number,
		ArInvoiceID: input.ARInvoiceID,
		Amount:      float64ToNumeric(input.Amount),
		PaidAt:      pgtype.Timestamptz{Time: input.PaidAt, Valid: true},
		Method:      input.Method,
		Note:        input.Note,
		CreatedAt:   pgtype.Timestamptz{Time: input.CreatedAt, Valid: true},
		UpdatedAt:   pgtype.Timestamptz{Time: input.UpdatedAt, Valid: true},
	})
}


// Helpers

func int8FromInt64(i int64) pgtype.Int8 {
	if i == 0 {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: i, Valid: true}
}

func float64ToNumeric(f float64) pgtype.Numeric {
	var n pgtype.Numeric
	n.Scan(fmt.Sprintf("%f", f))
	return n
}

func numericToFloat64(n pgtype.Numeric) float64 {
	f, _ := n.Float64Value()
	return f.Float64
}

// Mappers

func mapARInvoice(row ardb.ArInvoice) ARInvoice {
	return ARInvoice{
		ID:         row.ID,
		Number:     row.Number,
		CustomerID: row.CustomerID,
		SOID:       row.SoID.Int64,
		Currency:   row.Currency,
		Total:      numericToFloat64(row.Total),
		Status:     ARInvoiceStatus(row.Status),
		DueAt:      row.DueAt.Time,
		CreatedAt:  row.CreatedAt.Time,
		UpdatedAt:  row.UpdatedAt.Time,
	}
}

func mapARPayment(row ardb.ArPayment) ARPayment {
	return ARPayment{
		ID:          row.ID,
		Number:      row.Number,
		ARInvoiceID: row.ArInvoiceID,
		Amount:      numericToFloat64(row.Amount),
		PaidAt:      row.PaidAt.Time,
		Method:      row.Method,
		Note:        row.Note,
		CreatedAt:   row.CreatedAt.Time,
		UpdatedAt:   row.UpdatedAt.Time,
	}
}
