package ap

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/odyssey-erp/odyssey-erp/internal/sqlc"
)

func (r *pgRepository) CreateAPDebitNote(ctx context.Context, input CreateAPDebitNoteInput) (*APDebitNote, error) {
	if len(input.Lines) == 0 {
		return nil, errors.New("ap: debit note lines required")
	}
	var subtotal, taxAmount, total float64
	for _, line := range input.Lines {
		if line.Quantity <= 0 {
			return nil, errors.New("ap: debit note quantity must be positive")
		}
		lineSubtotal := line.Quantity * line.UnitPrice * (1 - line.DiscountPct/100)
		lineTax := lineSubtotal * line.TaxPct / 100
		subtotal += lineSubtotal
		taxAmount += lineTax
		total += lineSubtotal + lineTax
	}
	if total <= 0 {
		return nil, errors.New("ap: debit note total must be positive")
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := r.q.WithTx(tx)
	if input.Number == "" {
		input.Number, err = q.GenerateAPDebitNoteNumber(ctx)
		if err != nil {
			return nil, err
		}
	}
	id, err := q.CreateAPDebitNote(ctx, sqlc.CreateAPDebitNoteParams{
		Number: input.Number, SupplierID: input.SupplierID, ApInvoiceID: input.APInvoiceID,
		GoodsReturnGrnID: toNullInt64(input.GoodsReturnGRNID), Currency: input.Currency, Reason: input.Reason,
		Subtotal: floatToNumeric(subtotal), TaxAmount: floatToNumeric(taxAmount), Total: floatToNumeric(total),
		Status: sqlc.ApDebitNoteStatusDRAFT, CreatedBy: input.CreatedBy,
	})
	if err != nil {
		return nil, err
	}
	for _, line := range input.Lines {
		lineSubtotal := line.Quantity * line.UnitPrice * (1 - line.DiscountPct/100)
		lineTax := lineSubtotal * line.TaxPct / 100
		if _, err := q.CreateAPDebitNoteLine(ctx, sqlc.CreateAPDebitNoteLineParams{
			ApDebitNoteID: id, ApInvoiceLineID: toNullInt64(line.APInvoiceLineID), GoodsReturnGrnLineID: toNullInt64(line.GoodsReturnGRNLineID),
			ProductID: line.ProductID, Description: line.Description, Quantity: floatToNumeric(line.Quantity), UnitPrice: floatToNumeric(line.UnitPrice),
			DiscountPct: floatToNumeric(line.DiscountPct), TaxPct: floatToNumeric(line.TaxPct), Subtotal: floatToNumeric(lineSubtotal), TaxAmount: floatToNumeric(lineTax), Total: floatToNumeric(lineSubtotal + lineTax),
		}); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return r.GetAPDebitNote(ctx, id)
}

func (r *pgRepository) GetAPDebitNote(ctx context.Context, id int64) (*APDebitNote, error) {
	row, err := r.q.GetAPDebitNote(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrDebitNoteNotFound
	}
	if err != nil {
		return nil, err
	}
	return mapAPDebitNote(row), nil
}

func (r *pgRepository) GetAPDebitNoteWithDetails(ctx context.Context, id int64) (*APDebitNoteWithDetails, error) {
	note, err := r.GetAPDebitNote(ctx, id)
	if err != nil {
		return nil, err
	}
	rows, err := r.q.ListAPDebitNoteLines(ctx, id)
	if err != nil {
		return nil, err
	}
	lines := make([]APDebitNoteLine, len(rows))
	for i, row := range rows {
		lines[i] = APDebitNoteLine{ID: row.ID, APDebitNoteID: row.ApDebitNoteID, APInvoiceLineID: toInt64Ptr(row.ApInvoiceLineID), GoodsReturnGRNLineID: toInt64Ptr(row.GoodsReturnGrnLineID), ProductID: row.ProductID, Description: row.Description, Quantity: numericToFloat(row.Quantity), UnitPrice: numericToFloat(row.UnitPrice), DiscountPct: numericToFloat(row.DiscountPct), TaxPct: numericToFloat(row.TaxPct), Subtotal: numericToFloat(row.Subtotal), TaxAmount: numericToFloat(row.TaxAmount), Total: numericToFloat(row.Total), CreatedAt: safeTime(row.CreatedAt)}
	}
	return &APDebitNoteWithDetails{APDebitNote: *note, Lines: lines}, nil
}

func (r *pgRepository) ListAPDebitNotes(ctx context.Context, req ListAPDebitNotesRequest) ([]APDebitNote, error) {
	query := `SELECT d.id, d.number, d.supplier_id, COALESCE(s.name, ''), d.ap_invoice_id, d.goods_return_grn_id, d.currency, d.reason, d.subtotal, d.tax_amount, d.total, d.status, d.posted_at, d.posted_by, d.voided_at, d.voided_by, d.void_reason, d.created_by, d.created_at, d.updated_at, COALESCE(i.number, '') FROM ap_debit_notes d LEFT JOIN suppliers s ON s.id=d.supplier_id LEFT JOIN ap_invoices i ON i.id=d.ap_invoice_id WHERE 1=1`
	args := make([]any, 0, 4)
	if req.Status != "" {
		args = append(args, string(req.Status))
		query += fmt.Sprintf(" AND d.status=$%d", len(args))
	}
	if req.SupplierID != 0 {
		args = append(args, req.SupplierID)
		query += fmt.Sprintf(" AND d.supplier_id=$%d", len(args))
	}
	if req.InvoiceID != 0 {
		args = append(args, req.InvoiceID)
		query += fmt.Sprintf(" AND d.ap_invoice_id=$%d", len(args))
	}
	query += " ORDER BY d.created_at DESC"
	if req.Limit > 0 {
		args = append(args, req.Limit)
		query += fmt.Sprintf(" LIMIT $%d", len(args))
	}
	if req.Offset > 0 {
		args = append(args, req.Offset)
		query += fmt.Sprintf(" OFFSET $%d", len(args))
	}
	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	notes := make([]APDebitNote, 0)
	for rows.Next() {
		var row sqlc.GetAPDebitNoteRow
		if err := rows.Scan(&row.ID, &row.Number, &row.SupplierID, &row.SupplierName, &row.ApInvoiceID, &row.GoodsReturnGrnID, &row.Currency, &row.Reason, &row.Subtotal, &row.TaxAmount, &row.Total, &row.Status, &row.PostedAt, &row.PostedBy, &row.VoidedAt, &row.VoidedBy, &row.VoidReason, &row.CreatedBy, &row.CreatedAt, &row.UpdatedAt, &row.InvoiceNumber); err != nil {
			return nil, err
		}
		notes = append(notes, *mapAPDebitNote(row))
	}
	return notes, rows.Err()
}

func (r *pgRepository) PostAPDebitNote(ctx context.Context, id, invoiceID, postedBy int64, amount float64) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var available float64
	err = tx.QueryRow(ctx, `SELECT i.total - COALESCE((SELECT SUM(a.amount) FROM ap_payment_allocations a WHERE a.ap_invoice_id=i.id),0) - COALESCE((SELECT SUM(a.amount) FROM ap_debit_note_allocations a WHERE a.ap_invoice_id=i.id),0) FROM ap_invoices i WHERE i.id=$1 FOR UPDATE`, invoiceID).Scan(&available)
	if err != nil {
		return err
	}
	if amount > available {
		return ErrDebitExceedsBalance
	}
	cmd, err := tx.Exec(ctx, `UPDATE ap_debit_notes SET status='POSTED', posted_at=NOW(), posted_by=$2, updated_at=NOW() WHERE id=$1 AND status='DRAFT'`, id, postedBy)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return ErrInvalidStatus
	}
	if _, err := tx.Exec(ctx, `INSERT INTO ap_debit_note_allocations (ap_debit_note_id, ap_invoice_id, amount) VALUES ($1,$2,$3)`, id, invoiceID, amount); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *pgRepository) VoidAPDebitNote(ctx context.Context, id, voidedBy int64, reason string) error {
	cmd, err := r.pool.Exec(ctx, `UPDATE ap_debit_notes SET status='VOID', voided_at=NOW(), voided_by=$2, void_reason=$3, updated_at=NOW() WHERE id=$1 AND status='DRAFT'`, id, voidedBy, strings.TrimSpace(reason))
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return ErrInvalidStatus
	}
	return nil
}

func mapAPDebitNote(row sqlc.GetAPDebitNoteRow) *APDebitNote {
	return &APDebitNote{ID: row.ID, Number: row.Number, SupplierID: row.SupplierID, SupplierName: row.SupplierName, APInvoiceID: row.ApInvoiceID, InvoiceNumber: row.InvoiceNumber, GoodsReturnGRNID: toInt64Ptr(row.GoodsReturnGrnID), Currency: row.Currency, Reason: row.Reason, Subtotal: numericToFloat(row.Subtotal), TaxAmount: numericToFloat(row.TaxAmount), Total: numericToFloat(row.Total), Status: APDebitNoteStatus(row.Status), PostedAt: timestampToTime(row.PostedAt), PostedBy: toInt64Ptr(row.PostedBy), VoidedAt: timestampToTime(row.VoidedAt), VoidedBy: toInt64Ptr(row.VoidedBy), VoidReason: toStrPtr(row.VoidReason), CreatedBy: row.CreatedBy, CreatedAt: safeTime(row.CreatedAt), UpdatedAt: safeTime(row.UpdatedAt)}
}
