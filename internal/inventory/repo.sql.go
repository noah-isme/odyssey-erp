package inventory

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository persists inventory data in PostgreSQL.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository constructs Repository.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// TxRepository exposes transactional operations used by service.
type TxRepository interface {
	InsertTransaction(ctx context.Context, tx Transaction) (int64, error)
	InsertTransactionLines(ctx context.Context, txID int64, lines []TransactionLine) error
	GetBalanceForUpdate(ctx context.Context, warehouseID, productID int64) (Balance, error)
	UpsertBalance(ctx context.Context, balance Balance) error
	InsertCardEntry(ctx context.Context, card StockCardEntry, warehouseID, productID int64, txID int64) error
}

type txRepository struct {
	tx pgx.Tx
}

// ErrBalanceNotFound indicates missing balance row.
var ErrBalanceNotFound = errors.New("inventory balance not found")

// WithTx executes the callback inside repeatable-read transaction.
func (r *Repository) WithTx(ctx context.Context, fn func(context.Context, TxRepository) error) error {
	if r == nil {
		return errors.New("inventory repository not initialised")
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return err
	}
	wrapper := &txRepository{tx: tx}
	if err := fn(ctx, wrapper); err != nil {
		_ = tx.Rollback(ctx)
		return err
	}
	return tx.Commit(ctx)
}

func (r *Repository) GetStockCard(ctx context.Context, filter StockCardFilter) ([]StockCardEntry, error) {
	if r == nil {
		return nil, errors.New("inventory repository not initialised")
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = 200
	}
	rows, err := r.pool.Query(ctx, `SELECT tx_code, tx_type, posted_at, qty_in, qty_out, balance_qty, unit_cost, balance_cost, note
FROM inventory_cards
WHERE warehouse_id=$1 AND product_id=$2 AND posted_at BETWEEN COALESCE($3, '-infinity') AND COALESCE($4, 'infinity')
ORDER BY posted_at ASC, id ASC
LIMIT $5`, filter.WarehouseID, filter.ProductID, nullTime(filter.From), nullTime(filter.To), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cards := []StockCardEntry{}
	for rows.Next() {
		var entry StockCardEntry
		if err := rows.Scan(&entry.TxCode, &entry.TxType, &entry.PostedAt, &entry.QtyIn, &entry.QtyOut, &entry.BalanceQty, &entry.UnitCost, &entry.BalanceCost, &entry.Note); err != nil {
			return nil, err
		}
		cards = append(cards, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return cards, nil
}

func (r *txRepository) InsertTransaction(ctx context.Context, tx Transaction) (int64, error) {
	var id int64
	err := r.tx.QueryRow(ctx, `INSERT INTO inventory_tx (code, tx_type, warehouse_id, ref_module, ref_id, note, posted_at, created_by, created_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,NOW()) RETURNING id`, tx.Code, string(tx.Type), nullInt(tx.WarehouseID), tx.RefModule, nullUUID(tx.RefID), tx.Note, tx.PostedAt, nullInt(tx.CreatedBy)).Scan(&id)
	return id, err
}

func (r *txRepository) InsertTransactionLines(ctx context.Context, txID int64, lines []TransactionLine) error {
	for _, line := range lines {
		if _, err := r.tx.Exec(ctx, `INSERT INTO inventory_tx_lines (tx_id, product_id, qty, unit_cost, src_warehouse_id, dst_warehouse_id)
VALUES ($1,$2,$3,$4,$5,$6)`, txID, line.ProductID, line.Qty, line.UnitCost, nullInt(line.SrcWarehouseID), nullInt(line.DstWarehouseID)); err != nil {
			return err
		}
	}
	return nil
}

func (r *txRepository) GetBalanceForUpdate(ctx context.Context, warehouseID, productID int64) (Balance, error) {
	var bal Balance
	err := r.tx.QueryRow(ctx, `SELECT warehouse_id, product_id, qty, avg_cost, updated_at FROM inventory_balances WHERE warehouse_id=$1 AND product_id=$2 FOR UPDATE`, warehouseID, productID).
		Scan(&bal.WarehouseID, &bal.ProductID, &bal.Qty, &bal.AvgCost, &bal.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Balance{WarehouseID: warehouseID, ProductID: productID}, ErrBalanceNotFound
		}
		return Balance{}, err
	}
	return bal, nil
}

func (r *txRepository) UpsertBalance(ctx context.Context, balance Balance) error {
	_, err := r.tx.Exec(ctx, `INSERT INTO inventory_balances (warehouse_id, product_id, qty, avg_cost, updated_at)
VALUES ($1,$2,$3,$4,NOW())
ON CONFLICT (warehouse_id, product_id) DO UPDATE SET qty=EXCLUDED.qty, avg_cost=EXCLUDED.avg_cost, updated_at=NOW()`, balance.WarehouseID, balance.ProductID, balance.Qty, balance.AvgCost)
	return err
}

func (r *txRepository) InsertCardEntry(ctx context.Context, card StockCardEntry, warehouseID, productID int64, txID int64) error {
	_, err := r.tx.Exec(ctx, `INSERT INTO inventory_cards (warehouse_id, product_id, tx_id, tx_code, tx_type, qty_in, qty_out, balance_qty, unit_cost, balance_cost, posted_at, note)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, warehouseID, productID, txID, card.TxCode, string(card.TxType), card.QtyIn, card.QtyOut, card.BalanceQty, card.UnitCost, card.BalanceCost, card.PostedAt, card.Note)
	return err
}

func nullInt(value int64) any {
	if value == 0 {
		return nil
	}
	return value
}

func nullUUID(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func nullTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t
}
