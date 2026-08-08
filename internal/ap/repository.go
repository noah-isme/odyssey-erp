package ap

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	accountingmoney "github.com/odyssey-erp/odyssey-erp/internal/accounting/money"
	"github.com/odyssey-erp/odyssey-erp/internal/fx"
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

	CreateAPDebitNote(ctx context.Context, input CreateAPDebitNoteInput) (*APDebitNote, error)
	GetAPDebitNote(ctx context.Context, id int64) (*APDebitNote, error)
	GetAPDebitNoteWithDetails(ctx context.Context, id int64) (*APDebitNoteWithDetails, error)
	ListAPDebitNotes(ctx context.Context, req ListAPDebitNotesRequest) ([]APDebitNote, error)
	PostAPDebitNote(ctx context.Context, id, invoiceID, postedBy int64, amount float64) error
	VoidAPDebitNote(ctx context.Context, id, voidedBy int64, reason string) error

	// Q1 Additions
	CheckDuplicateInvoice(ctx context.Context, supplierID int64, docNumber string) (bool, error)
	UpdateAPInvoiceDuplicateStatus(ctx context.Context, id int64, status string) error

	// Q2 Additions
	GetActiveMatchingPolicy(ctx context.Context, companyID, supplierID, categoryID *int64) (*MatchingPolicy, error)
	GetPOLineProgressByPO(ctx context.Context, poID int64) (map[int64]*sqlc.PoLineProgress, error)

	// Q3 Additions
	GetAPException(ctx context.Context, id int64) (APException, error)
	ListAPExceptions(ctx context.Context, status string, ownerID, invoiceID int64, limit, offset int) ([]APException, error)
	GetLatestMatchingRun(ctx context.Context, invoiceID int64) (*MatchingRun, error)
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

	// Q2 Additions
	CreateMatchingRun(ctx context.Context, run MatchingRun) (int64, error)
	CreateMatchingRunLine(ctx context.Context, line MatchingRunLine) error

	// Q3 Additions
	CreateAPException(ctx context.Context, exc APException) (int64, error)
	UpdateAPExceptionStatus(ctx context.Context, id int64, status string, resolvedBy *int64) error
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
	txRepo := &pgTxRepository{q: qTx, tx: tx}

	if err := fn(ctx, txRepo); err != nil {
		_ = tx.Rollback(ctx)
		return err
	}
	return tx.Commit(ctx)
}

func (r *pgRepository) PostAPInvoiceWithValuation(ctx context.Context, input PostAPInvoiceInput, v APInvoiceValuation) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var status string
	if err = tx.QueryRow(ctx, `SELECT status FROM ap_invoices WHERE id=$1 FOR UPDATE`, input.InvoiceID).Scan(&status); err != nil {
		return err
	}
	if status != string(APStatusDraft) {
		return ErrInvalidStatus
	}
	_, err = tx.Exec(ctx, `UPDATE ap_invoices SET original_currency_amount=$2, base_currency=$3, base_amount=$4, fx_rate=$5, fx_rate_date=$6, fx_rate_source=$7, fx_rate_locked_at=$8, status='POSTED', posted_at=NOW(), posted_by=$9, updated_at=NOW() WHERE id=$1`, input.InvoiceID, v.OriginalAmount.String(), v.BaseCurrency, v.BaseAmount.String(), v.Rate.String(), v.RateDate, v.Source, v.LockedAt, input.PostedBy)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *pgRepository) CheckDuplicateInvoice(ctx context.Context, supplierID int64, docNumber string) (bool, error) {
	_, err := r.q.CheckDuplicateInvoice(ctx, sqlc.CheckDuplicateInvoiceParams{
		SupplierID:             supplierID,
		SupplierDocumentNumber: toNullString(&docNumber),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (r *pgRepository) UpdateAPInvoiceDuplicateStatus(ctx context.Context, id int64, status string) error {
	return r.q.UpdateAPInvoiceDuplicateStatus(ctx, sqlc.UpdateAPInvoiceDuplicateStatusParams{
		ID:              id,
		DuplicateStatus: status,
	})
}

func (r *pgRepository) GetActiveMatchingPolicy(ctx context.Context, companyID, supplierID, categoryID *int64) (*MatchingPolicy, error) {
	pol, err := r.q.GetActiveMatchingPolicy(ctx, sqlc.GetActiveMatchingPolicyParams{
		CompanyID:  toNullInt64(companyID),
		SupplierID: toNullInt64(supplierID),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrMatchingPolicyNotFound
		}
		return nil, err
	}
	return &MatchingPolicy{
		ID:                  pol.ID,
		Name:                pol.Name,
		CompanyID:           toInt64Ptr(pol.CompanyID),
		SupplierID:          toInt64Ptr(pol.SupplierID),
		CategoryID:          toInt64Ptr(pol.CategoryID),
		QtyTolerancePct:     numericToFloat(pol.QtyTolerancePct),
		PriceTolerancePct:   numericToFloat(pol.PriceTolerancePct),
		TaxTolerancePct:     numericToFloat(pol.TaxTolerancePct),
		FreightTolerancePct: numericToFloat(pol.FreightTolerancePct),
		TotalToleranceAmt:   numericToFloat(pol.TotalToleranceAmt),
	}, nil
}

func (r *pgRepository) GetPOLineProgressByPO(ctx context.Context, poID int64) (map[int64]*sqlc.PoLineProgress, error) {
	rows, err := r.q.GetPOLineProgress(ctx, poID)
	if err != nil {
		return nil, err
	}
	res := make(map[int64]*sqlc.PoLineProgress)
	for i := range rows {
		row := rows[i]
		res[row.PoLineID] = &row
	}
	return res, nil
}

func (r *pgRepository) GetAPException(ctx context.Context, id int64) (APException, error) {
	row, err := r.q.GetAPException(ctx, id)
	if err != nil {
		return APException{}, err
	}
	var sla *time.Time
	if row.SlaDueAt.Valid {
		sla = &row.SlaDueAt.Time
	}
	var resolvedAt *time.Time
	if row.ResolvedAt.Valid {
		resolvedAt = &row.ResolvedAt.Time
	}
	return APException{
		ID:              row.ID,
		APInvoiceID:     row.ApInvoiceID,
		APMatchingRunID: toInt64Ptr(row.ApMatchingRunID),
		ExceptionType:   row.ExceptionType,
		Severity:        row.Severity,
		Status:          row.Status,
		OwnerID:         toInt64Ptr(row.OwnerID),
		SLADueAt:        sla,
		Reason:          row.Reason,
		Evidence:        toStringPtr(row.Evidence),
		Comments:        row.Comments,
		CreatedAt:       row.CreatedAt.Time,
		UpdatedAt:       row.UpdatedAt.Time,
		ResolvedAt:      resolvedAt,
		ResolvedBy:      toInt64Ptr(row.ResolvedBy),
	}, nil
}

func (r *pgRepository) ListAPExceptions(ctx context.Context, status string, ownerID, invoiceID int64, limit, offset int) ([]APException, error) {
	rows, err := r.q.ListAPExceptions(ctx, sqlc.ListAPExceptionsParams{
		Column1: status,
		Column2: ownerID,
		Column3: invoiceID,
		Limit:   int32(limit),
		Offset:  int32(offset),
	})
	if err != nil {
		return nil, err
	}
	var res []APException
	for _, row := range rows {
		var sla *time.Time
		if row.SlaDueAt.Valid {
			sla = &row.SlaDueAt.Time
		}
		var resolvedAt *time.Time
		if row.ResolvedAt.Valid {
			resolvedAt = &row.ResolvedAt.Time
		}
		res = append(res, APException{
			ID:              row.ID,
			APInvoiceID:     row.ApInvoiceID,
			APMatchingRunID: toInt64Ptr(row.ApMatchingRunID),
			ExceptionType:   row.ExceptionType,
			Severity:        row.Severity,
			Status:          row.Status,
			OwnerID:         toInt64Ptr(row.OwnerID),
			SLADueAt:        sla,
			Reason:          row.Reason,
			Evidence:        toStringPtr(row.Evidence),
			Comments:        row.Comments,
			CreatedAt:       row.CreatedAt.Time,
			UpdatedAt:       row.UpdatedAt.Time,
			ResolvedAt:      resolvedAt,
			ResolvedBy:      toInt64Ptr(row.ResolvedBy),
		})
	}
	return res, nil
}

func (r *pgRepository) GetLatestMatchingRun(ctx context.Context, invoiceID int64) (*MatchingRun, error) {
	row, err := r.q.GetLatestMatchingRun(ctx, invoiceID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	poTotal := numericToFloat(row.PoTotal)
	grnTotal := numericToFloat(row.GrnTotal)
	return &MatchingRun{
		ID:                row.ID,
		APInvoiceID:       row.ApInvoiceID,
		PolicyID:          toInt64Ptr(row.PolicyID),
		Status:            row.Status,
		InvoiceTotal:      numericToFloat(row.InvoiceTotal),
		POTotal:           &poTotal,
		GRNTotal:          &grnTotal,
		Reasons:           row.Reasons,
		ActionRecommended: row.ActionRecommended,
	}, nil
}

func (r *pgRepository) GetAPInvoice(ctx context.Context, id int64) (APInvoice, error) {
	dbInv, err := r.q.GetAPInvoice(ctx, id)
	if err != nil {
		return APInvoice{}, err
	}

	result := APInvoice{
		ID:                     dbInv.ID,
		Number:                 dbInv.Number,
		SupplierID:             dbInv.SupplierID,
		SupplierName:           dbInv.SupplierName,
		GRNID:                  toInt64Ptr(dbInv.GrnID),
		POID:                   toInt64Ptr(dbInv.PoID),
		Currency:               dbInv.Currency,
		Subtotal:               numericToFloat(dbInv.Subtotal),
		TaxAmount:              numericToFloat(dbInv.TaxAmount),
		Total:                  numericToFloat(dbInv.Total),
		Status:                 APInvoiceStatus(dbInv.Status),
		DuplicateStatus:        dbInv.DuplicateStatus,
		AttachmentHash:         toStrPtr(dbInv.AttachmentHash),
		SupplierDocumentNumber: toStrPtr(dbInv.SupplierDocumentNumber),
		DueAt:                  dbInv.DueAt.Time,
		PostedAt:               timestampToTime(dbInv.PostedAt),
		PostedBy:               toInt64Ptr(dbInv.PostedBy),
		VoidedAt:               timestampToTime(dbInv.VoidedAt),
		VoidedBy:               toInt64Ptr(dbInv.VoidedBy),
		VoidReason:             toStrPtr(dbInv.VoidReason),
		CreatedBy:              dbInv.CreatedBy.Int64,
		CreatedAt:              safeTime(dbInv.CreatedAt),
		UpdatedAt:              safeTime(dbInv.UpdatedAt),
	}
	if err := r.loadInvoiceValuation(ctx, id, &result); err != nil {
		return APInvoice{}, err
	}
	return result, nil
}

func (r *pgRepository) loadInvoiceValuation(ctx context.Context, id int64, inv *APInvoice) error {
	var original, base, rate, source, currency, baseCurrency string
	var rateDate, lockedAt time.Time
	err := r.pool.QueryRow(ctx, `SELECT COALESCE(i.currency,''), COALESCE(i.original_currency_amount,i.total,0)::text,
		COALESCE(i.base_currency,co.base_currency,'IDR'), COALESCE(i.base_amount,0)::text, COALESCE(i.fx_rate,0)::text,
		COALESCE(fx_rate_date, DATE '0001-01-01'), COALESCE(fx_rate_source,''),
		COALESCE(fx_rate_locked_at, TIMESTAMPTZ '0001-01-01') FROM ap_invoices i JOIN suppliers s ON s.id=i.supplier_id LEFT JOIN companies co ON co.id=s.company_id WHERE i.id=$1`, id).
		Scan(&currency, &original, &baseCurrency, &base, &rate, &rateDate, &source, &lockedAt)
	if err != nil {
		return err
	}
	var parseErr error
	if inv.OriginalAmount, parseErr = accountingmoney.Parse(original, decimalScale(original)); parseErr != nil {
		return parseErr
	}
	if inv.BaseAmount, parseErr = accountingmoney.Parse(base, decimalScale(base)); parseErr != nil {
		return parseErr
	}
	inv.Currency, inv.BaseCurrency, inv.FXRateDate, inv.FXRateSource, inv.FXRateLockedAt = currency, baseCurrency, rateDate, source, lockedAt
	inv.FXRate, parseErr = fx.ParseDecimal(rate)
	return parseErr
}

func decimalScale(v string) int {
	for i := 0; i < len(v); i++ {
		if v[i] == '.' {
			return len(v) - i - 1
		}
	}
	return 0
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

	var lines []APInvoiceLine
	for _, line := range linesRows {
		// Mapping sqlc.ApInvoiceLine to ap.APInvoiceLine
		lines = append(lines, APInvoiceLine{
			ID:          line.ID,
			APInvoiceID: line.ApInvoiceID,
			GRNLineID:   toInt64Ptr(line.GrnLineID),
			POLineID:    toInt64Ptr(line.PoLineID),
			ProductID:   line.ProductID,
			Description: line.Description,
			Quantity:    numericToFloat(line.Quantity),
			UnitPrice:   numericToFloat(line.UnitPrice),
			DiscountPct: numericToFloat(line.DiscountPct),
			TaxPct:      numericToFloat(line.TaxPct),
			Subtotal:    numericToFloat(line.Subtotal),
			TaxAmount:   numericToFloat(line.TaxAmount),
			Total:       numericToFloat(line.Total),
		})
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
	totalAllocated := fx.MustDecimal("0")
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
		allocated, err := fx.FromLegacyFloat(alloc.Amount, 2)
		if err != nil {
			return APPaymentWithDetails{}, err
		}
		totalAllocated = totalAllocated.Add(allocated)
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

	paymentAmount, err := fx.FromLegacyFloat(payment.Amount, 2)
	if err != nil {
		return APPaymentWithDetails{}, err
	}
	unallocated := paymentAmount.Sub(totalAllocated)
	if unallocated.Cmp(fx.MustDecimal("0")) < 0 {
		unallocated = fx.MustDecimal("0")
	}

	return APPaymentWithDetails{
		APPayment:      payment,
		Allocations:    allocations,
		TotalAllocated: legacyFloat(totalAllocated),
		Unallocated:    legacyFloat(unallocated),
		LedgerPosted:   posted,
	}, nil
}

// Transaction Repository Implementation

type pgTxRepository struct {
	q  *sqlc.Queries
	tx pgx.Tx
}

func (tx *pgTxRepository) PGXTx() pgx.Tx { return tx.tx }

func (tx *pgTxRepository) PostAPInvoiceWithValuation(ctx context.Context, input PostAPInvoiceInput, v APInvoiceValuation) error {
	var status string
	if err := tx.tx.QueryRow(ctx, `SELECT status FROM ap_invoices WHERE id=$1 FOR UPDATE`, input.InvoiceID).Scan(&status); err != nil {
		return err
	}
	if status != string(APStatusDraft) {
		return ErrInvalidStatus
	}
	_, err := tx.tx.Exec(ctx, `UPDATE ap_invoices SET original_currency_amount=$2, base_currency=$3, base_amount=$4, fx_rate=$5, fx_rate_date=$6, fx_rate_source=$7, fx_rate_locked_at=$8, status='POSTED', posted_at=NOW(), posted_by=$9, updated_at=NOW() WHERE id=$1`, input.InvoiceID, v.OriginalAmount.String(), v.BaseCurrency, v.BaseAmount.String(), v.Rate.String(), v.RateDate, v.Source, v.LockedAt, input.PostedBy)
	return err
}

func (tx *pgTxRepository) UpdateAPPaymentValuation(ctx context.Context, id int64, v APPaymentValuation) error {
	_, err := tx.tx.Exec(ctx, `UPDATE ap_payments SET currency=$2, original_currency_amount=$3, base_currency=$4, base_amount=$5, fx_rate=$6, fx_rate_date=$7, fx_rate_source=$8, fx_rate_locked_at=$9, updated_at=NOW() WHERE id=$1`, id, v.Currency, v.OriginalAmount.String(), v.BaseCurrency, v.BaseAmount.String(), v.Rate.String(), v.RateDate, v.Source, v.LockedAt)
	return err
}
func (tx *pgTxRepository) UpdateAPAllocationValuation(ctx context.Context, paymentID, invoiceID int64, v APAllocationValuation) error {
	_, err := tx.tx.Exec(ctx, `UPDATE ap_payment_allocations SET original_currency_amount=$3, base_amount=$4, currency=$5, base_currency=$6, fx_rate=$7, fx_rate_date=$8, fx_rate_source=$9, fx_rate_locked_at=$10 WHERE ap_payment_id=$1 AND ap_invoice_id=$2`, paymentID, invoiceID, v.OriginalAmount.String(), v.BaseAmount.String(), v.Currency, v.BaseCurrency, v.Rate.String(), v.RateDate, v.Source, v.LockedAt)
	return err
}
func (tx *pgTxRepository) CreateAPInvoice(ctx context.Context, input CreateAPInvoiceInput) (int64, error) {
	return tx.q.CreateAPInvoice(ctx, sqlc.CreateAPInvoiceParams{
		Number:                 input.Number,
		SupplierID:             input.SupplierID,
		GrnID:                  toNullInt64(input.GRNID),
		PoID:                   toNullInt64(input.POID),
		Currency:               input.Currency,
		SupplierDocumentNumber: toNullString(input.SupplierDocumentNumber),
		AttachmentHash:         toNullString(input.AttachmentHash),
		Subtotal:               floatToNumeric(input.Subtotal),
		TaxAmount:              floatToNumeric(input.TaxAmount),
		Total:                  floatToNumeric(input.Total),
		Status:                 string(APStatusDraft),
		DueAt:                  timeToDate(input.DueDate),
		CreatedBy:              toNullInt64(&input.CreatedBy),
	})
}

func (tx *pgTxRepository) CreateAPInvoiceLine(ctx context.Context, input CreateAPInvoiceLineInput, invoiceID int64) error {
	quantity, err := fx.FromLegacyFloat(input.Quantity, 6)
	if err != nil {
		return err
	}
	unitPrice, err := fx.FromLegacyFloat(input.UnitPrice, 6)
	if err != nil {
		return err
	}
	discount, err := fx.FromLegacyFloat(input.DiscountPct, 6)
	if err != nil {
		return err
	}
	taxRate, err := fx.FromLegacyFloat(input.TaxPct, 6)
	if err != nil {
		return err
	}
	hundred := fx.MustDecimal("100")
	lineSubtotal := quantity.Mul(unitPrice).Mul(fx.MustDecimal("1").Sub(discount.Div(hundred))).Round(2)
	lineTax := lineSubtotal.Mul(taxRate.Div(hundred)).Round(2)
	lineTotal := lineSubtotal.Add(lineTax).Round(2)
	_, err = tx.q.CreateAPInvoiceLine(ctx, sqlc.CreateAPInvoiceLineParams{
		ApInvoiceID: invoiceID,
		GrnLineID:   toNullInt64(input.GRNLineID),
		PoLineID:    toNullInt64(input.POLineID),
		ProductID:   input.ProductID,
		Description: input.Description,
		Quantity:    floatToNumeric(input.Quantity),
		UnitPrice:   floatToNumeric(input.UnitPrice),
		DiscountPct: floatToNumeric(input.DiscountPct),
		TaxPct:      floatToNumeric(input.TaxPct),
		Subtotal:    decimalToNumeric(lineSubtotal),
		TaxAmount:   decimalToNumeric(lineTax),
		Total:       decimalToNumeric(lineTotal),
	})
	return err
}

func (tx *pgTxRepository) CreateMatchingRun(ctx context.Context, run MatchingRun) (int64, error) {
	return tx.q.CreateMatchingRun(ctx, sqlc.CreateMatchingRunParams{
		ApInvoiceID:       run.APInvoiceID,
		PolicyID:          toNullInt64(run.PolicyID),
		Status:            run.Status,
		InvoiceTotal:      floatToNumeric(run.InvoiceTotal),
		PoTotal:           floatToNumeric(*run.POTotal),
		GrnTotal:          floatToNumeric(*run.GRNTotal),
		Reasons:           run.Reasons,
		ActionRecommended: run.ActionRecommended,
	})
}

func (tx *pgTxRepository) CreateMatchingRunLine(ctx context.Context, line MatchingRunLine) error {
	var poQty, poPrice, grnQty float64
	if line.POQty != nil {
		poQty = *line.POQty
	}
	if line.POPrice != nil {
		poPrice = *line.POPrice
	}
	if line.GRNQty != nil {
		grnQty = *line.GRNQty
	}
	_, err := tx.q.CreateMatchingRunLine(ctx, sqlc.CreateMatchingRunLineParams{
		ApMatchingRunID: line.MatchingRunID,
		ApInvoiceLineID: line.APInvoiceLineID,
		PoLineID:        toNullInt64(line.POLineID),
		GrnLineID:       toNullInt64(line.GRNLineID),
		InvoiceQty:      floatToNumeric(line.InvoiceQty),
		InvoicePrice:    floatToNumeric(line.InvoicePrice),
		PoQty:           floatToNumeric(poQty),
		PoPrice:         floatToNumeric(poPrice),
		GrnQty:          floatToNumeric(grnQty),
		Status:          line.Status,
		Reasons:         line.Reasons,
	})
	return err
}

func (tx *pgTxRepository) CreateAPException(ctx context.Context, exc APException) (int64, error) {
	var slaDue pgtype.Timestamptz
	if exc.SLADueAt != nil {
		slaDue = pgtype.Timestamptz{Time: *exc.SLADueAt, Valid: true}
	}
	return tx.q.CreateAPException(ctx, sqlc.CreateAPExceptionParams{
		ApInvoiceID:     exc.APInvoiceID,
		ApMatchingRunID: toNullInt64(exc.APMatchingRunID),
		ExceptionType:   exc.ExceptionType,
		Severity:        exc.Severity,
		Status:          exc.Status,
		OwnerID:         toNullInt64(exc.OwnerID),
		SlaDueAt:        slaDue,
		Reason:          exc.Reason,
		Evidence:        toNullString(exc.Evidence),
		Comments:        exc.Comments,
	})
}

func (tx *pgTxRepository) UpdateAPExceptionStatus(ctx context.Context, id int64, status string, resolvedBy *int64) error {
	return tx.q.UpdateAPExceptionStatus(ctx, sqlc.UpdateAPExceptionStatusParams{
		ID:         id,
		Status:     status,
		ResolvedBy: toNullInt64(resolvedBy),
	})
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

func decimalToNumeric(d fx.Decimal) pgtype.Numeric {
	var n pgtype.Numeric
	if err := n.Scan(d.String()); err != nil {
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
func toNullString(s *string) pgtype.Text {
	if s == nil {
		return pgtype.Text{Valid: false}
	}
	return pgtype.Text{String: *s, Valid: true}
}

func toStringPtr(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	return &t.String
}
