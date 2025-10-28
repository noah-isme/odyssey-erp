package procurement

import (
	"context"
	"errors"
	"time"

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

// TxRepository exposes transactional operations.
type TxRepository interface {
	CreatePR(ctx context.Context, pr PurchaseRequest) (int64, error)
	InsertPRLine(ctx context.Context, line PRLine) error
	UpdatePRStatus(ctx context.Context, id int64, status PRStatus) error
	CreatePO(ctx context.Context, po PurchaseOrder) (int64, error)
	InsertPOLine(ctx context.Context, line POLine) error
	UpdatePOStatus(ctx context.Context, id int64, status POStatus) error
	SetPOApproval(ctx context.Context, id int64, approvedBy int64, approvedAt time.Time) error
	CreateGRN(ctx context.Context, grn GoodsReceipt) (int64, error)
	InsertGRNLine(ctx context.Context, line GRNLine) error
	UpdateGRNStatus(ctx context.Context, id int64, status GRNStatus) error
	CreateAPInvoice(ctx context.Context, inv APInvoice) (int64, error)
	UpdateAPStatus(ctx context.Context, id int64, status APInvoiceStatus) error
	CreatePayment(ctx context.Context, payment APPayment) (int64, error)
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

// Fetch helpers

// GetPR returns purchase request and lines.
func (r *Repository) GetPR(ctx context.Context, id int64) (PurchaseRequest, []PRLine, error) {
	var pr PurchaseRequest
	err := r.pool.QueryRow(ctx, `SELECT id, number, COALESCE(supplier_id,0), request_by, status, note FROM prs WHERE id=$1`, id).
		Scan(&pr.ID, &pr.Number, &pr.SupplierID, &pr.RequestBy, &pr.Status, &pr.Note)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PurchaseRequest{}, nil, ErrNotFound
		}
		return PurchaseRequest{}, nil, err
	}
	rows, err := r.pool.Query(ctx, `SELECT id, pr_id, product_id, qty, note FROM pr_lines WHERE pr_id=$1 ORDER BY id`, id)
	if err != nil {
		return PurchaseRequest{}, nil, err
	}
	defer rows.Close()
	var lines []PRLine
	for rows.Next() {
		var line PRLine
		if err := rows.Scan(&line.ID, &line.PRID, &line.ProductID, &line.Qty, &line.Note); err != nil {
			return PurchaseRequest{}, nil, err
		}
		lines = append(lines, line)
	}
	if err := rows.Err(); err != nil {
		return PurchaseRequest{}, nil, err
	}
	return pr, lines, nil
}

// GetPO returns purchase order and lines.
func (r *Repository) GetPO(ctx context.Context, id int64) (PurchaseOrder, []POLine, error) {
	var po PurchaseOrder
	err := r.pool.QueryRow(ctx, `SELECT id, number, supplier_id, status, currency, COALESCE(expected_date, CURRENT_DATE), note FROM pos WHERE id=$1`, id).
		Scan(&po.ID, &po.Number, &po.SupplierID, &po.Status, &po.Currency, &po.ExpectedDate, &po.Note)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PurchaseOrder{}, nil, ErrNotFound
		}
		return PurchaseOrder{}, nil, err
	}
	rows, err := r.pool.Query(ctx, `SELECT id, po_id, product_id, qty, price, COALESCE(tax_id,0), note FROM po_lines WHERE po_id=$1 ORDER BY id`, id)
	if err != nil {
		return PurchaseOrder{}, nil, err
	}
	defer rows.Close()
	var lines []POLine
	for rows.Next() {
		var line POLine
		if err := rows.Scan(&line.ID, &line.POID, &line.ProductID, &line.Qty, &line.Price, &line.TaxID, &line.Note); err != nil {
			return PurchaseOrder{}, nil, err
		}
		lines = append(lines, line)
	}
	if err := rows.Err(); err != nil {
		return PurchaseOrder{}, nil, err
	}
	return po, lines, nil
}

// GetGRN returns GRN and lines.
func (r *Repository) GetGRN(ctx context.Context, id int64) (GoodsReceipt, []GRNLine, error) {
	var grn GoodsReceipt
	err := r.pool.QueryRow(ctx, `SELECT id, number, COALESCE(po_id,0), supplier_id, warehouse_id, status, received_at, note FROM grns WHERE id=$1`, id).
		Scan(&grn.ID, &grn.Number, &grn.POID, &grn.SupplierID, &grn.WarehouseID, &grn.Status, &grn.ReceivedAt, &grn.Note)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return GoodsReceipt{}, nil, ErrNotFound
		}
		return GoodsReceipt{}, nil, err
	}
	rows, err := r.pool.Query(ctx, `SELECT id, grn_id, product_id, qty, unit_cost FROM grn_lines WHERE grn_id=$1 ORDER BY id`, id)
	if err != nil {
		return GoodsReceipt{}, nil, err
	}
	defer rows.Close()
	var lines []GRNLine
	for rows.Next() {
		var line GRNLine
		if err := rows.Scan(&line.ID, &line.GRNID, &line.ProductID, &line.Qty, &line.UnitCost); err != nil {
			return GoodsReceipt{}, nil, err
		}
		lines = append(lines, line)
	}
	if err := rows.Err(); err != nil {
		return GoodsReceipt{}, nil, err
	}
	return grn, lines, nil
}

// GetAPInvoice fetches an AP invoice by ID.
func (r *Repository) GetAPInvoice(ctx context.Context, id int64) (APInvoice, error) {
	var inv APInvoice
	err := r.pool.QueryRow(ctx, `SELECT id, number, supplier_id, COALESCE(grn_id,0), currency, total, status, COALESCE(due_at, CURRENT_DATE)
FROM ap_invoices WHERE id=$1`, id).Scan(&inv.ID, &inv.Number, &inv.SupplierID, &inv.GRNID, &inv.Currency, &inv.Total, &inv.Status, &inv.DueAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return APInvoice{}, ErrNotFound
		}
	}
	return inv, err
}

// ListAPOutstanding returns posted invoices with remaining balance.
func (r *Repository) ListAPOutstanding(ctx context.Context) ([]APInvoice, error) {
	rows, err := r.pool.Query(ctx, `SELECT id, number, supplier_id, COALESCE(grn_id,0), currency, total, status, COALESCE(due_at, CURRENT_DATE)
FROM ap_invoices WHERE status IN ('POSTED','PAID') ORDER BY due_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var invoices []APInvoice
	for rows.Next() {
		var inv APInvoice
		if err := rows.Scan(&inv.ID, &inv.Number, &inv.SupplierID, &inv.GRNID, &inv.Currency, &inv.Total, &inv.Status, &inv.DueAt); err != nil {
			return nil, err
		}
		invoices = append(invoices, inv)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return invoices, nil
}

func (tx *txRepo) CreatePR(ctx context.Context, pr PurchaseRequest) (int64, error) {
	var id int64
	err := tx.tx.QueryRow(ctx, `INSERT INTO prs (number, supplier_id, request_by, status, note, created_at)
VALUES ($1,$2,$3,$4,$5,NOW()) RETURNING id`, pr.Number, nullInt(pr.SupplierID), pr.RequestBy, pr.Status, pr.Note).Scan(&id)
	return id, err
}

func (tx *txRepo) InsertPRLine(ctx context.Context, line PRLine) error {
	_, err := tx.tx.Exec(ctx, `INSERT INTO pr_lines (pr_id, product_id, qty, note) VALUES ($1,$2,$3,$4)`, line.PRID, line.ProductID, line.Qty, line.Note)
	return err
}

func (tx *txRepo) UpdatePRStatus(ctx context.Context, id int64, status PRStatus) error {
	_, err := tx.tx.Exec(ctx, `UPDATE prs SET status=$1 WHERE id=$2`, status, id)
	return err
}

func (tx *txRepo) CreatePO(ctx context.Context, po PurchaseOrder) (int64, error) {
	var id int64
	err := tx.tx.QueryRow(ctx, `INSERT INTO pos (number, supplier_id, status, currency, expected_date, note, created_at)
VALUES ($1,$2,$3,$4,$5,$6,NOW()) RETURNING id`, po.Number, po.SupplierID, po.Status, po.Currency, nullDate(po.ExpectedDate), po.Note).Scan(&id)
	return id, err
}

func (tx *txRepo) InsertPOLine(ctx context.Context, line POLine) error {
	_, err := tx.tx.Exec(ctx, `INSERT INTO po_lines (po_id, product_id, qty, price, tax_id, note) VALUES ($1,$2,$3,$4,$5,$6)`, line.POID, line.ProductID, line.Qty, line.Price, nullInt(line.TaxID), line.Note)
	return err
}

func (tx *txRepo) UpdatePOStatus(ctx context.Context, id int64, status POStatus) error {
	_, err := tx.tx.Exec(ctx, `UPDATE pos SET status=$1 WHERE id=$2`, status, id)
	return err
}

func (tx *txRepo) SetPOApproval(ctx context.Context, id int64, approvedBy int64, approvedAt time.Time) error {
	_, err := tx.tx.Exec(ctx, `UPDATE pos SET approved_by=$1, approved_at=$2 WHERE id=$3`, nullInt(approvedBy), approvedAt, id)
	return err
}

func (tx *txRepo) CreateGRN(ctx context.Context, grn GoodsReceipt) (int64, error) {
	var id int64
	err := tx.tx.QueryRow(ctx, `INSERT INTO grns (number, po_id, supplier_id, warehouse_id, status, received_at, note, created_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,NOW()) RETURNING id`, grn.Number, nullInt(grn.POID), grn.SupplierID, grn.WarehouseID, grn.Status, grn.ReceivedAt, grn.Note).Scan(&id)
	return id, err
}

func (tx *txRepo) InsertGRNLine(ctx context.Context, line GRNLine) error {
	_, err := tx.tx.Exec(ctx, `INSERT INTO grn_lines (grn_id, product_id, qty, unit_cost) VALUES ($1,$2,$3,$4)`, line.GRNID, line.ProductID, line.Qty, line.UnitCost)
	return err
}

func (tx *txRepo) UpdateGRNStatus(ctx context.Context, id int64, status GRNStatus) error {
	_, err := tx.tx.Exec(ctx, `UPDATE grns SET status=$1 WHERE id=$2`, status, id)
	return err
}

func (tx *txRepo) CreateAPInvoice(ctx context.Context, inv APInvoice) (int64, error) {
	var id int64
	err := tx.tx.QueryRow(ctx, `INSERT INTO ap_invoices (number, supplier_id, grn_id, currency, total, status, issued_at, due_at, created_at)
VALUES ($1,$2,$3,$4,$5,$6,CURRENT_DATE,$7,NOW()) RETURNING id`, inv.Number, inv.SupplierID, nullInt(inv.GRNID), inv.Currency, inv.Total, inv.Status, nullDate(inv.DueAt)).Scan(&id)
	return id, err
}

func (tx *txRepo) UpdateAPStatus(ctx context.Context, id int64, status APInvoiceStatus) error {
	_, err := tx.tx.Exec(ctx, `UPDATE ap_invoices SET status=$1 WHERE id=$2`, status, id)
	return err
}

func (tx *txRepo) CreatePayment(ctx context.Context, payment APPayment) (int64, error) {
	var id int64
	err := tx.tx.QueryRow(ctx, `INSERT INTO ap_payments (number, ap_invoice_id, amount, paid_at, method, note)
VALUES ($1,$2,$3,CURRENT_DATE,'TRANSFER','') RETURNING id`, payment.Number, payment.APInvoiceID, payment.Amount).Scan(&id)
	return id, err
}

func nullInt(value int64) any {
	if value == 0 {
		return nil
	}
	return value
}

func nullDate(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t
}
