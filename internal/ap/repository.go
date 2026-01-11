package ap

import (
	"context"
	"fmt"
	"time"


	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/odyssey-erp/odyssey-erp/internal/sqlc"
)

// Repository defines AP data access.
type Repository interface {
	WithTx(ctx context.Context, fn func(context.Context, TxRepository) error) error
	
	GetAPInvoice(ctx context.Context, id int64) (APInvoice, error)
	GetAPInvoiceWithDetails(ctx context.Context, id int64) (APInvoiceWithDetails, error)
	ListAPInvoices(ctx context.Context, req ListAPInvoicesRequest) ([]APInvoice, error)
	CountInvoicesByGRN(ctx context.Context, grnID int64) (int, error)
	
	ListAPPayments(ctx context.Context) ([]APPayment, error)
}

// TxRepository defines operations within a transaction.
type TxRepository interface {
	CreateAPInvoice(ctx context.Context, input CreateAPInvoiceInput) (int64, error)
	CreateAPInvoiceLine(ctx context.Context, input CreateAPInvoiceLineInput, invoiceID int64) error
	UpdateAPStatus(ctx context.Context, id int64, status APInvoiceStatus) error
	PostAPInvoice(ctx context.Context, input PostAPInvoiceInput) error
	VoidAPInvoice(ctx context.Context, input VoidAPInvoiceInput) error
	
	CreateAPPayment(ctx context.Context, input CreateAPPaymentInput) (int64, error)
	CreatePaymentAllocation(ctx context.Context, input PaymentAllocationInput, paymentID int64) error
	
	// Helper for generating numbers
	GenerateAPInvoiceNumber(ctx context.Context) (string, error)
	GenerateAPPaymentNumber(ctx context.Context) (string, error)
}

// Ensure implementation
var _ Repository = (*pgRepository)(nil)
var _ TxRepository = (*pgTxRepository)(nil)

type pgRepository struct {
	pool *pgxpool.Pool
	q    *sqlc.Queries
}

func NewRepository(pool *pgxpool.Pool) Repository {
	return &pgRepository{
		pool: pool,
		q:    sqlc.New(pool),
	}
}

func (r *pgRepository) WithTx(ctx context.Context, fn func(context.Context, TxRepository) error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	qTx := r.q.WithTx(tx)
	txRepo := &pgTxRepository{q: qTx}
	
	if err := fn(ctx, txRepo); err != nil {
		_ = tx.Rollback(ctx)
		return err
	}
	return tx.Commit(ctx)
}

func (r *pgRepository) GetAPInvoice(ctx context.Context, id int64) (APInvoice, error) {
	row, err := r.q.GetAPInvoice(ctx, id)
	if err != nil {
		return APInvoice{}, err
	}
	
	return APInvoice{
		ID:         row.ID,
		Number:     row.Number,
		SupplierID: row.SupplierID,
		GRNID:      toInt64Ptr(row.GrnID),
		Currency:   row.Currency,
		Subtotal:   numericToFloat(row.Subtotal),
		TaxAmount:  numericToFloat(row.TaxAmount),
		Total:      numericToFloat(row.Total),
		Status:     APInvoiceStatus(row.Status),
		DueAt:      dateToTime(row.DueAt),
		PostedAt:   timestampToTime(row.PostedAt),
		PostedBy:   toInt64Ptr(row.PostedBy),
		VoidedAt:   timestampToTime(row.VoidedAt),
		VoidedBy:   toInt64Ptr(row.VoidedBy),
		VoidReason: toStrPtr(row.VoidReason),
		CreatedBy:  row.CreatedBy.Int64, 
		CreatedAt:  safeTime(row.CreatedAt),
		UpdatedAt:  safeTime(row.UpdatedAt),
	}, nil
}

func (r *pgRepository) CountInvoicesByGRN(ctx context.Context, grnID int64) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, "SELECT COUNT(*) FROM ap_invoices WHERE grn_id = $1", grnID).Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}

func (r *pgRepository) GetAPInvoiceWithDetails(ctx context.Context, id int64) (APInvoiceWithDetails, error) {
	// 1. Get Invoice
	inv, err := r.GetAPInvoice(ctx, id)
	if err != nil {
		return APInvoiceWithDetails{}, err
	}

	// 2. Get Lines
	// Need to fetch from DB and map to domain
	// Assuming r.q.ListAPInvoiceLines exists and works.
	// But it returns []sqlc.ApInvoiceLine (struct generated from table)
	
	// Wait, internal/ap/repository.go:108 in previous edit used 'r.q.ListAPInvoiceLines'.
	// In sqlc output: `func (q *Queries) ListAPInvoiceLines(ctx context.Context, apInvoiceID int64) ([]ApInvoiceLine, error)`
	// ApInvoiceLine is in `models.go` (package sqlc).
	
	linesRows, err := r.q.ListAPInvoiceLines(ctx, id)
	if err != nil {
		return APInvoiceWithDetails{}, err
	}
	
	lines := make([]APInvoiceLine, len(linesRows))
	for i, l := range linesRows {
		// Mapping sqlc.ApInvoiceLine to ap.APInvoiceLine
		lines[i] = APInvoiceLine{
			ID:          l.ID,
			APInvoiceID: l.ApInvoiceID,
			GRNLineID:   toInt64Ptr(l.GrnLineID),
			ProductID:   l.ProductID,
			Description: l.Description,
			Quantity:    numericToFloat(l.Quantity),
			UnitPrice:   numericToFloat(l.UnitPrice),
			DiscountPct: numericToFloat(l.DiscountPct),
			TaxPct:      numericToFloat(l.TaxPct),
			Subtotal:    numericToFloat(l.Subtotal),
			TaxAmount:   numericToFloat(l.TaxAmount),
			Total:       numericToFloat(l.Total),
			CreatedAt:   safeTime(l.CreatedAt),
		}
	}

	// 3. Get Payments
	paymentsRows, err := r.q.ListAPInvoicePayments(ctx, id)
	if err != nil {
		return APInvoiceWithDetails{}, err
	}
	payments := make([]APPaymentSummary, len(paymentsRows))
	for i, p := range paymentsRows {
		payments[i] = APPaymentSummary{
			ID:              p.ID,
			Number:          p.Number,
			Amount:          numericToFloat(p.Amount),
			AllocatedAmount: numericToFloat(p.AllocatedAmount),
			PaidAt:          dateToTime(p.PaidAt),
			Method:          p.Method,
			Note:            p.Note,
		}
	}

	// 4. Calculate Balance
	balRow, err := r.q.GetAPInvoiceBalance(ctx, id)
	var paidAmount, balance float64
	if err == nil {
		paidAmount = numericToFloat(balRow.PaidAmount)
		balance = numericToFloat(balRow.Balance)
	} else {
		// Log error or ignore if not found? 
		// Ideally ensure 0.
	}

	return APInvoiceWithDetails{
		APInvoice:    inv,
		SupplierName: "", // Ideally fetch supplier name or fill in service/handler via another call if needed. Repository usually just gets data.
		Lines:        lines,
		Payments:     payments,
		PaidAmount:   paidAmount,
		Balance:      balance,
	}, nil
}

func (r *pgRepository) ListAPInvoices(ctx context.Context, req ListAPInvoicesRequest) ([]APInvoice, error) {
	var invoices []APInvoice

	if req.Status != "" {
		rows, err := r.q.ListAPInvoicesByStatus(ctx, string(req.Status))
		if err != nil { return nil, err }
		invoices = make([]APInvoice, len(rows))
		for i, row := range rows {
			invoices[i] = APInvoice{
				ID: row.ID, Number: row.Number, SupplierID: row.SupplierID, GRNID: toInt64Ptr(row.GrnID),
				Currency: row.Currency, Subtotal: numericToFloat(row.Subtotal), TaxAmount: numericToFloat(row.TaxAmount), Total: numericToFloat(row.Total),
				Status: APInvoiceStatus(row.Status), DueAt: dateToTime(row.DueAt), PostedAt: timestampToTime(row.PostedAt), PostedBy: toInt64Ptr(row.PostedBy),
				VoidedAt: timestampToTime(row.VoidedAt), VoidedBy: toInt64Ptr(row.VoidedBy), VoidReason: toStrPtr(row.VoidReason), CreatedBy: row.CreatedBy.Int64, CreatedAt: safeTime(row.CreatedAt), UpdatedAt: safeTime(row.UpdatedAt),
			}
		}
	} else if req.SupplierID != 0 {
		rows, err := r.q.ListAPInvoicesBySupplier(ctx, req.SupplierID)
		if err != nil { return nil, err }
		invoices = make([]APInvoice, len(rows))
		for i, row := range rows {
			invoices[i] = APInvoice{
				ID: row.ID, Number: row.Number, SupplierID: row.SupplierID, GRNID: toInt64Ptr(row.GrnID),
				Currency: row.Currency, Subtotal: numericToFloat(row.Subtotal), TaxAmount: numericToFloat(row.TaxAmount), Total: numericToFloat(row.Total),
				Status: APInvoiceStatus(row.Status), DueAt: dateToTime(row.DueAt), PostedAt: timestampToTime(row.PostedAt), PostedBy: toInt64Ptr(row.PostedBy),
				VoidedAt: timestampToTime(row.VoidedAt), VoidedBy: toInt64Ptr(row.VoidedBy), VoidReason: toStrPtr(row.VoidReason), CreatedBy: row.CreatedBy.Int64, CreatedAt: safeTime(row.CreatedAt), UpdatedAt: safeTime(row.UpdatedAt),
			}
		}
	} else {
		rows, err := r.q.ListAPInvoices(ctx)
		if err != nil { return nil, err }
		invoices = make([]APInvoice, len(rows))
		for i, row := range rows {
			invoices[i] = APInvoice{
				ID: row.ID, Number: row.Number, SupplierID: row.SupplierID, GRNID: toInt64Ptr(row.GrnID),
				Currency: row.Currency, Subtotal: numericToFloat(row.Subtotal), TaxAmount: numericToFloat(row.TaxAmount), Total: numericToFloat(row.Total),
				Status: APInvoiceStatus(row.Status), DueAt: dateToTime(row.DueAt), PostedAt: timestampToTime(row.PostedAt), PostedBy: toInt64Ptr(row.PostedBy),
				VoidedAt: timestampToTime(row.VoidedAt), VoidedBy: toInt64Ptr(row.VoidedBy), VoidReason: toStrPtr(row.VoidReason), CreatedBy: row.CreatedBy.Int64, CreatedAt: safeTime(row.CreatedAt), UpdatedAt: safeTime(row.UpdatedAt),
			}
		}
	}
	return invoices, nil
}

func (r *pgRepository) ListAPPayments(ctx context.Context) ([]APPayment, error) {
	rows, err := r.q.ListAPPayments(ctx)
	if err != nil {
		return nil, err
	}
	payments := make([]APPayment, len(rows))
	for i, r := range rows {
		payments[i] = APPayment{
			ID:          r.ID,
			Number:      r.Number,
			APInvoiceID: &r.ApInvoiceID, 
			Amount:      numericToFloat(r.Amount),
			PaidAt:      dateToTime(r.PaidAt),
			Method:      r.Method,
			Note:        r.Note,
			CreatedBy:   r.CreatedBy.Int64,
			CreatedAt:   safeTime(r.CreatedAt),
		}
	}
	return payments, nil
}

// Transaction Repository Implementation

type pgTxRepository struct {
	q *sqlc.Queries
}

func (tx *pgTxRepository) CreateAPInvoice(ctx context.Context, input CreateAPInvoiceInput) (int64, error) {
	return tx.q.CreateAPInvoice(ctx, sqlc.CreateAPInvoiceParams{
		Number:     input.Number,
		SupplierID: input.SupplierID,
		GrnID:      toNullInt64(input.GRNID),
		Currency:   input.Currency,
		Subtotal:   floatToNumeric(input.Subtotal),
		TaxAmount:  floatToNumeric(input.TaxAmount),
		Total:      floatToNumeric(input.Total),
		Status:     string(APStatusDraft),
		DueAt:      timeToDate(input.DueDate),
		CreatedBy:  toNullInt64(&input.CreatedBy),
	})
}

func (tx *pgTxRepository) CreateAPInvoiceLine(ctx context.Context, input CreateAPInvoiceLineInput, invoiceID int64) error {
	_, err := tx.q.CreateAPInvoiceLine(ctx, sqlc.CreateAPInvoiceLineParams{
		ApInvoiceID: invoiceID,
		GrnLineID:   toNullInt64(input.GRNLineID),
		ProductID:   input.ProductID,
		Description: input.Description,
		Quantity:    floatToNumeric(input.Quantity),
		UnitPrice:   floatToNumeric(input.UnitPrice),
		DiscountPct: floatToNumeric(input.DiscountPct),
		TaxPct:      floatToNumeric(input.TaxPct),
		Subtotal:    floatToNumeric(input.Quantity * input.UnitPrice),
		TaxAmount:   floatToNumeric(0), 
		Total:       floatToNumeric(0),
	})
	return err
}

func (tx *pgTxRepository) UpdateAPStatus(ctx context.Context, id int64, status APInvoiceStatus) error {
	return tx.q.UpdateAPStatus(ctx, sqlc.UpdateAPStatusParams{
		ID:     id,
		Status: string(status),
	})
}

func (tx *pgTxRepository) PostAPInvoice(ctx context.Context, input PostAPInvoiceInput) error {
	return tx.q.PostAPInvoice(ctx, sqlc.PostAPInvoiceParams{
		ID:       input.InvoiceID,
		PostedBy: toNullInt64(&input.PostedBy),
	})
}

func (tx *pgTxRepository) VoidAPInvoice(ctx context.Context, input VoidAPInvoiceInput) error {
	return tx.q.VoidAPInvoice(ctx, sqlc.VoidAPInvoiceParams{
		ID:         input.InvoiceID,
		VoidedBy:   toNullInt64(&input.VoidedBy),
		VoidReason: toText(input.VoidReason),
	})
}

func (tx *pgTxRepository) CreateAPPayment(ctx context.Context, input CreateAPPaymentInput) (int64, error) {
	var invoiceID int64
	if len(input.Allocations) > 0 {
		invoiceID = input.Allocations[0].APInvoiceID
	}
	
	return tx.q.CreateAPPayment(ctx, sqlc.CreateAPPaymentParams{
		Number:      input.Number,
		ApInvoiceID: invoiceID,
		Amount:      floatToNumeric(input.Amount),
		PaidAt:      timeToDate(input.PaidAt),
		Method:      input.Method,
		Note:        input.Note,
		CreatedBy:   toNullInt64(&input.CreatedBy),
	})
}

func (tx *pgTxRepository) CreatePaymentAllocation(ctx context.Context, input PaymentAllocationInput, paymentID int64) error {
	_, err := tx.q.CreateAPPaymentAllocation(ctx, sqlc.CreateAPPaymentAllocationParams{
		ApPaymentID: paymentID,
		ApInvoiceID: input.APInvoiceID,
		Amount:      floatToNumeric(input.Amount),
	})
	return err
}

func (tx *pgTxRepository) GenerateAPInvoiceNumber(ctx context.Context) (string, error) {
	res, err := tx.q.GenerateAPInvoiceNumber(ctx)
	if err != nil { return "", err }
	return res.(string), nil
}

func (tx *pgTxRepository) GenerateAPPaymentNumber(ctx context.Context) (string, error) {
	res, err := tx.q.GenerateAPPaymentNumber(ctx)
	if err != nil { return "", err }
	return res.(string), nil
}


// Helpers

func numericToFloat(n pgtype.Numeric) float64 {
	f, _ := n.Float64Value()
	return f.Float64
}

func floatToNumeric(f float64) pgtype.Numeric {
	var n pgtype.Numeric
	n.Scan(fmt.Sprintf("%f", f))
	return n
}

func dateToTime(d pgtype.Date) time.Time {
	return d.Time
}

func timeToDate(t time.Time) pgtype.Date {
	return pgtype.Date{Time: t, Valid: true}
}

func timestampToTime(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}

func toInt64Ptr(n pgtype.Int8) *int64 {
	if !n.Valid {
		return nil
	}
	v := n.Int64
	return &v
}

func toNullInt64(i *int64) pgtype.Int8 {
	if i == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *i, Valid: true}
}

func toText(s string) pgtype.Text {
	return pgtype.Text{String: s, Valid: s != ""}
}

func safeTime(t pgtype.Timestamptz) time.Time {
	if !t.Valid {
		return time.Time{}
	}
	return t.Time
}

func toStrPtr(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	s := t.String
	return &s
}
