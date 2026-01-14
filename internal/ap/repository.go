package ap

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
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
	GetAPInvoiceBalancesBatch(ctx context.Context) ([]APInvoiceBalance, error)

	ListAPPayments(ctx context.Context) ([]APPayment, error)
	GetAPPaymentWithDetails(ctx context.Context, id int64) (APPaymentWithDetails, error)
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
		ID:           row.ID,
		Number:       row.Number,
		SupplierID:   row.SupplierID,
		SupplierName: row.SupplierName,
		GRNID:        toInt64Ptr(row.GrnID),
		POID:         toInt64Ptr(row.PoID),
		Currency:     row.Currency,
		Subtotal:     numericToFloat(row.Subtotal),
		TaxAmount:    numericToFloat(row.TaxAmount),
		Total:        numericToFloat(row.Total),
		Status:       APInvoiceStatus(row.Status),
		DueAt:        dateToTime(row.DueAt),
		PostedAt:     timestampToTime(row.PostedAt),
		PostedBy:     toInt64Ptr(row.PostedBy),
		VoidedAt:     timestampToTime(row.VoidedAt),
		VoidedBy:     toInt64Ptr(row.VoidedBy),
		VoidReason:   toStrPtr(row.VoidReason),
		CreatedBy:    row.CreatedBy.Int64,
		CreatedAt:    safeTime(row.CreatedAt),
		UpdatedAt:    safeTime(row.UpdatedAt),
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

func (r *pgRepository) GetAPInvoiceBalancesBatch(ctx context.Context) ([]APInvoiceBalance, error) {
	rows, err := r.q.GetAPInvoiceBalancesBatch(ctx)
	if err != nil {
		return nil, err
	}
	balances := make([]APInvoiceBalance, len(rows))
	for i, row := range rows {
		balances[i] = APInvoiceBalance{
			ID:         row.ID,
			DueAt:      dateToTime(row.DueAt),
			Total:      numericToFloat(row.Total),
			PaidAmount: numericToFloat(row.PaidAmount),
			Balance:    numericToFloat(row.Balance),
		}
	}
	return balances, nil
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
		paidAmount = 0
		balance = inv.Total
	}

	return APInvoiceWithDetails{
		APInvoice:    inv,
		SupplierName: inv.SupplierName,
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
		if err != nil {
			return nil, err
		}
		invoices = make([]APInvoice, len(rows))
		for i, row := range rows {
			invoices[i] = APInvoice{
				ID: row.ID, Number: row.Number, SupplierID: row.SupplierID, GRNID: toInt64Ptr(row.GrnID),
				SupplierName: row.SupplierName, POID: toInt64Ptr(row.PoID),
				Currency: row.Currency, Subtotal: numericToFloat(row.Subtotal), TaxAmount: numericToFloat(row.TaxAmount), Total: numericToFloat(row.Total),
				Status: APInvoiceStatus(row.Status), DueAt: dateToTime(row.DueAt), PostedAt: timestampToTime(row.PostedAt), PostedBy: toInt64Ptr(row.PostedBy),
				VoidedAt: timestampToTime(row.VoidedAt), VoidedBy: toInt64Ptr(row.VoidedBy), VoidReason: toStrPtr(row.VoidReason), CreatedBy: row.CreatedBy.Int64, CreatedAt: safeTime(row.CreatedAt), UpdatedAt: safeTime(row.UpdatedAt),
			}
		}
	} else if req.SupplierID != 0 {
		rows, err := r.q.ListAPInvoicesBySupplier(ctx, req.SupplierID)
		if err != nil {
			return nil, err
		}
		invoices = make([]APInvoice, len(rows))
		for i, row := range rows {
			invoices[i] = APInvoice{
				ID: row.ID, Number: row.Number, SupplierID: row.SupplierID, GRNID: toInt64Ptr(row.GrnID),
				SupplierName: row.SupplierName, POID: toInt64Ptr(row.PoID),
				Currency: row.Currency, Subtotal: numericToFloat(row.Subtotal), TaxAmount: numericToFloat(row.TaxAmount), Total: numericToFloat(row.Total),
				Status: APInvoiceStatus(row.Status), DueAt: dateToTime(row.DueAt), PostedAt: timestampToTime(row.PostedAt), PostedBy: toInt64Ptr(row.PostedBy),
				VoidedAt: timestampToTime(row.VoidedAt), VoidedBy: toInt64Ptr(row.VoidedBy), VoidReason: toStrPtr(row.VoidReason), CreatedBy: row.CreatedBy.Int64, CreatedAt: safeTime(row.CreatedAt), UpdatedAt: safeTime(row.UpdatedAt),
			}
		}
	} else {
		rows, err := r.q.ListAPInvoices(ctx)
		if err != nil {
			return nil, err
		}
		invoices = make([]APInvoice, len(rows))
		for i, row := range rows {
			invoices[i] = APInvoice{
				ID: row.ID, Number: row.Number, SupplierID: row.SupplierID, GRNID: toInt64Ptr(row.GrnID),
				SupplierName: row.SupplierName, POID: toInt64Ptr(row.PoID),
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
			ID:           r.ID,
			Number:       r.Number,
			APInvoiceID:  toInt64Ptr(r.ApInvoiceID),
			SupplierID:   r.SupplierID.Int64,
			SupplierName: r.SupplierName,
			Amount:       numericToFloat(r.Amount),
			PaidAt:       dateToTime(r.PaidAt),
			Method:       r.Method,
			Note:         r.Note,
			CreatedBy:    r.CreatedBy.Int64,
			CreatedAt:    safeTime(r.CreatedAt),
		}
	}
	return payments, nil
}

func (r *pgRepository) GetAPPaymentWithDetails(ctx context.Context, id int64) (APPaymentWithDetails, error) {
	var (
		apInvoiceID pgtype.Int8
		supplierID  pgtype.Int8
		amount      pgtype.Numeric
		paidAt      pgtype.Date
		createdBy   pgtype.Int8
		createdAt   pgtype.Timestamptz
		updatedAt   pgtype.Timestamptz
	)
	var payment APPayment
	err := r.pool.QueryRow(ctx, `
SELECT p.id, p.number, p.ap_invoice_id, p.supplier_id, COALESCE(s.name, '') AS supplier_name,
       p.amount, p.paid_at, p.method, p.note, p.created_by, p.created_at, p.updated_at
FROM ap_payments p
LEFT JOIN suppliers s ON s.id = p.supplier_id
WHERE p.id = $1`, id).Scan(
		&payment.ID,
		&payment.Number,
		&apInvoiceID,
		&supplierID,
		&payment.SupplierName,
		&amount,
		&paidAt,
		&payment.Method,
		&payment.Note,
		&createdBy,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return APPaymentWithDetails{}, err
	}
	payment.APInvoiceID = toInt64Ptr(apInvoiceID)
	payment.SupplierID = supplierID.Int64
	payment.Amount = numericToFloat(amount)
	payment.PaidAt = dateToTime(paidAt)
	payment.CreatedBy = createdBy.Int64
	payment.CreatedAt = safeTime(createdAt)
	payment.UpdatedAt = safeTime(updatedAt)

	rows, err := r.pool.Query(ctx, `
SELECT pa.id, pa.ap_payment_id, pa.ap_invoice_id, pa.amount,
       i.number AS invoice_number, i.po_id, i.total, i.status, i.due_at
FROM ap_payment_allocations pa
JOIN ap_invoices i ON i.id = pa.ap_invoice_id
WHERE pa.ap_payment_id = $1
ORDER BY i.number`, id)
	if err != nil {
		return APPaymentWithDetails{}, err
	}
	defer rows.Close()

	var allocations []APPaymentAllocationDetail
	var totalAllocated float64
	for rows.Next() {
		var alloc APPaymentAllocationDetail
		var allocAmount pgtype.Numeric
		var poID pgtype.Int8
		var total pgtype.Numeric
		var status string
		var dueAt pgtype.Date
		if err := rows.Scan(
			&alloc.ID,
			&alloc.APPaymentID,
			&alloc.APInvoiceID,
			&allocAmount,
			&alloc.InvoiceNumber,
			&poID,
			&total,
			&status,
			&dueAt,
		); err != nil {
			return APPaymentWithDetails{}, err
		}
		alloc.POID = toInt64Ptr(poID)
		alloc.InvoiceStatus = APInvoiceStatus(status)
		alloc.InvoiceTotal = numericToFloat(total)
		alloc.DueAt = dateToTime(dueAt)
		alloc.Amount = numericToFloat(allocAmount)
		totalAllocated += alloc.Amount
		allocations = append(allocations, alloc)
	}
	if err := rows.Err(); err != nil {
		return APPaymentWithDetails{}, err
	}

	var posted bool
	sourceID := apPaymentSourceID(payment.ID)
	if err := r.pool.QueryRow(ctx, `
SELECT EXISTS (
    SELECT 1
    FROM journal_entries
    WHERE source_module = $1 AND source_id = $2 AND status = 'POSTED'
)`, "PROCUREMENT.AP_PAYMENT", uuidToPg(sourceID)).Scan(&posted); err != nil {
		return APPaymentWithDetails{}, err
	}

	unallocated := payment.Amount - totalAllocated
	if unallocated < 0 {
		unallocated = 0
	}

	return APPaymentWithDetails{
		APPayment:      payment,
		Allocations:    allocations,
		TotalAllocated: totalAllocated,
		Unallocated:    unallocated,
		LedgerPosted:   posted,
	}, nil
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
		PoID:       toNullInt64(input.POID),
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
	lineSubtotal := input.Quantity * input.UnitPrice * (1 - (input.DiscountPct / 100))
	lineTax := lineSubtotal * (input.TaxPct / 100)
	lineTotal := lineSubtotal + lineTax
	_, err := tx.q.CreateAPInvoiceLine(ctx, sqlc.CreateAPInvoiceLineParams{
		ApInvoiceID: invoiceID,
		GrnLineID:   toNullInt64(input.GRNLineID),
		ProductID:   input.ProductID,
		Description: input.Description,
		Quantity:    floatToNumeric(input.Quantity),
		UnitPrice:   floatToNumeric(input.UnitPrice),
		DiscountPct: floatToNumeric(input.DiscountPct),
		TaxPct:      floatToNumeric(input.TaxPct),
		Subtotal:    floatToNumeric(lineSubtotal),
		TaxAmount:   floatToNumeric(lineTax),
		Total:       floatToNumeric(lineTotal),
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
	var invoiceIDPtr *int64
	if len(input.Allocations) > 0 {
		invoiceID := input.Allocations[0].APInvoiceID
		invoiceIDPtr = &invoiceID
	}
	var supplierIDPtr *int64
	if input.SupplierID != 0 {
		supplierIDPtr = &input.SupplierID
	}

	return tx.q.CreateAPPayment(ctx, sqlc.CreateAPPaymentParams{
		Number:      input.Number,
		ApInvoiceID: toNullInt64(invoiceIDPtr),
		SupplierID:  toNullInt64(supplierIDPtr),
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
	if err != nil {
		return "", err
	}
	return res.(string), nil
}

func (tx *pgTxRepository) GenerateAPPaymentNumber(ctx context.Context) (string, error) {
	res, err := tx.q.GenerateAPPaymentNumber(ctx)
	if err != nil {
		return "", err
	}
	return res.(string), nil
}

// Helpers

func numericToFloat(n pgtype.Numeric) float64 {
	f, _ := n.Float64Value()
	return f.Float64
}

func floatToNumeric(f float64) pgtype.Numeric {
	var n pgtype.Numeric
	if err := n.Scan(fmt.Sprintf("%f", f)); err != nil {
		return pgtype.Numeric{}
	}
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

func uuidToPg(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: true}
}

func apPaymentSourceID(id int64) uuid.UUID {
	return uuid.NewSHA1(uuid.Nil, []byte(fmt.Sprintf("APPAY:%d", id)))
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
