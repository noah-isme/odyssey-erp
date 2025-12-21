package inventory

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/odyssey-erp/odyssey-erp/internal/sqlc"
)

// Repository persists inventory data in PostgreSQL.
type Repository struct {
	pool    *pgxpool.Pool
	queries *sqlc.Queries
}

// NewRepository constructs Repository.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{
		pool:    pool,
		queries: sqlc.New(pool),
	}
}

// TxRepository exposes transactional operations used by service.
type TxRepository interface {
	InsertTransaction(ctx context.Context, tx Transaction) (int64, error)
	InsertTransactionLines(ctx context.Context, txID int64, lines []TransactionLine) error
	GetBalanceForUpdate(ctx context.Context, warehouseID, productID int64) (Balance, error)
	UpsertBalance(ctx context.Context, balance Balance) error
	InsertCardEntry(ctx context.Context, card StockCardEntry, warehouseID, productID int64, txID int64) error
}

type txRepo struct {
	queries *sqlc.Queries
}

// ErrBalanceNotFound indicates missing balance row.
var ErrBalanceNotFound = errors.New("inventory balance not found")

// WithTx executes the callback inside repeatable-read transaction.
func (r *Repository) WithTx(ctx context.Context, fn func(context.Context, TxRepository) error) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	q := r.queries.WithTx(tx)
	wrapper := &txRepo{queries: q}

	if err := fn(ctx, wrapper); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (r *Repository) GetStockCard(ctx context.Context, filter StockCardFilter) ([]StockCardEntry, error) {
	arg := sqlc.GetStockCardParams{
		WarehouseID: filter.WarehouseID,
		ProductID:   filter.ProductID,
		FromDate:    pgtype.Timestamptz{Time: filter.From, Valid: !filter.From.IsZero()},
		ToDate:      pgtype.Timestamptz{Time: filter.To, Valid: !filter.To.IsZero()},
		Limit:       int32(filter.Limit),
	}
	if arg.Limit <= 0 {
		arg.Limit = 200
	}

	rows, err := r.queries.GetStockCard(ctx, arg)
	if err != nil {
		return nil, err
	}

	var cards []StockCardEntry
	for _, row := range rows {
		entry := StockCardEntry{
			TxCode:      row.TxCode,
			TxType:      TransactionType(row.TxType),
			PostedAt:    row.PostedAt.Time,
			QtyIn:       float64(numericToFloat(row.QtyIn)),
			QtyOut:      float64(numericToFloat(row.QtyOut)),
			BalanceQty:  float64(numericToFloat(row.BalanceQty)),
			UnitCost:    float64(numericToFloat(row.UnitCost)),
			BalanceCost: float64(numericToFloat(row.BalanceCost)),
			Note:        row.Note,
		}
		cards = append(cards, entry)
	}
	return cards, nil
}

func (r *txRepo) InsertTransaction(ctx context.Context, tx Transaction) (int64, error) {
	return r.queries.InsertTransaction(ctx, sqlc.InsertTransactionParams{
		Code:        tx.Code,
		TxType:      string(tx.Type),
		WarehouseID: pgtype.Int8{Int64: tx.WarehouseID, Valid: tx.WarehouseID != 0},
		RefModule:   tx.RefModule,
		RefID:       pgtype.UUID{Bytes: parseUUID(tx.RefID), Valid: tx.RefID != ""},
		Note:        tx.Note,
		PostedAt:    pgtype.Timestamptz{Time: tx.PostedAt, Valid: true},
		CreatedBy:   pgtype.Int8{Int64: tx.CreatedBy, Valid: tx.CreatedBy != 0},
	})
}

func (r *txRepo) InsertTransactionLines(ctx context.Context, txID int64, lines []TransactionLine) error {
	for _, line := range lines {
		err := r.queries.InsertTransactionLine(ctx, sqlc.InsertTransactionLineParams{
			TxID:           txID,
			ProductID:      line.ProductID,
			Qty:            floatToNumeric(line.Qty),
			UnitCost:       floatToNumeric(line.UnitCost),
			SrcWarehouseID: pgtype.Int8{Int64: line.SrcWarehouseID, Valid: line.SrcWarehouseID != 0},
			DstWarehouseID: pgtype.Int8{Int64: line.DstWarehouseID, Valid: line.DstWarehouseID != 0},
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *txRepo) GetBalanceForUpdate(ctx context.Context, warehouseID, productID int64) (Balance, error) {
	row, err := r.queries.GetBalanceForUpdate(ctx, sqlc.GetBalanceForUpdateParams{
		WarehouseID: warehouseID,
		ProductID:   productID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Balance{WarehouseID: warehouseID, ProductID: productID}, ErrBalanceNotFound
		}
		return Balance{}, err
	}
	return Balance{
		WarehouseID: row.WarehouseID,
		ProductID:   row.ProductID,
		Qty:         float64(numericToFloat(row.Qty)),
		AvgCost:     float64(numericToFloat(row.AvgCost)),
		UpdatedAt:   row.UpdatedAt.Time,
	}, nil
}

func (r *txRepo) UpsertBalance(ctx context.Context, balance Balance) error {
	return r.queries.UpsertBalance(ctx, sqlc.UpsertBalanceParams{
		WarehouseID: balance.WarehouseID,
		ProductID:   balance.ProductID,
		Qty:         floatToNumeric(balance.Qty),
		AvgCost:     floatToNumeric(balance.AvgCost),
	})
}

func (r *txRepo) InsertCardEntry(ctx context.Context, card StockCardEntry, warehouseID, productID int64, txID int64) error {
	return r.queries.InsertCardEntry(ctx, sqlc.InsertCardEntryParams{
		WarehouseID: warehouseID,
		ProductID:   productID,
		TxID:        txID,
		TxCode:      card.TxCode,
		TxType:      string(card.TxType),
		QtyIn:       floatToNumeric(card.QtyIn),
		QtyOut:      floatToNumeric(card.QtyOut),
		BalanceQty:  floatToNumeric(card.BalanceQty),
		UnitCost:    floatToNumeric(card.UnitCost),
		BalanceCost: floatToNumeric(card.BalanceCost),
		PostedAt:    pgtype.Timestamptz{Time: card.PostedAt, Valid: true},
		Note:        card.Note,
	})
}

func parseUUID(s string) [16]byte {
	if s == "" {
		return [16]byte{}
	}
	id, err := uuid.Parse(s)
	if err != nil {
		return [16]byte{}
	}
	return id
}

func numericToFloat(n pgtype.Numeric) float64 {
	f, _ := n.Float64Value()
	return f.Float64
}

func floatToNumeric(f float64) pgtype.Numeric {
	var n pgtype.Numeric
	n.Scan(fmt.Sprintf("%f", f))
	return n
}
