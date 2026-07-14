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
	InsertStockTake(ctx context.Context, arg sqlc.InsertStockTakeParams) (int64, error)
	InsertStockTakeLine(ctx context.Context, arg sqlc.InsertStockTakeLineParams) error

	// Stock Adjustments
	InsertAdjustment(ctx context.Context, arg sqlc.InsertAdjustmentParams) (int64, error)
	GetAdjustment(ctx context.Context, id int64) (StockAdjustment, error)
	InsertAdjustmentLine(ctx context.Context, arg sqlc.InsertAdjustmentLineParams) error
	UpdateAdjustmentStatus(ctx context.Context, arg sqlc.UpdateAdjustmentStatusParams) error
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
	defer func() { _ = tx.Rollback(ctx) }()

	q := r.queries.WithTx(tx)
	wrapper := &txRepo{queries: q}

	if err := fn(ctx, wrapper); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (r *Repository) GetStockTake(ctx context.Context, id int64) (StockTake, error) {
	row, err := r.queries.GetStockTake(ctx, id)
	if err != nil {
		return StockTake{}, err
	}
	st := StockTake{
		ID:            row.ID,
		UUID:          uuid.UUID(row.Uuid.Bytes).String(),
		Number:        row.Number,
		WarehouseID:   int64(row.WarehouseID),
		Status:        StockTakeStatus(row.Status),
		Note:          row.Note,
		TakenAt:       row.TakenAt.Time,
		CreatedBy:     row.CreatedBy,
		PostedBy:      row.PostedBy.Int64,
		PostedAt:      row.PostedAt.Time,
		CreatedAt:     row.CreatedAt.Time,
		CreatorEmail:  row.CreatorEmail,
		WarehouseName: row.WarehouseName,
	}
	lines, err := r.queries.GetStockTakeLines(ctx, id)
	if err != nil {
		return st, err
	}
	for _, l := range lines {
		st.Lines = append(st.Lines, StockTakeLine{
			ID:          l.ID,
			StockTakeID: l.StockTakeID,
			ProductID:   int64(l.ProductID),
			ProductName: l.ProductName,
			SystemQty:   numericToFloat(l.SystemQty),
			PhysicalQty: numericToFloat(l.PhysicalQty),
			VarianceQty: numericToFloat(l.VarianceQty),
			Note:        l.Note,
		})
	}
	return st, nil
}

func (r *Repository) ListStockTakes(ctx context.Context) ([]StockTake, error) {
	rows, err := r.queries.ListStockTakes(ctx)
	if err != nil {
		return nil, err
	}
	var res []StockTake
	for _, row := range rows {
		res = append(res, StockTake{
			ID:            row.ID,
			UUID:          uuid.UUID(row.Uuid.Bytes).String(),
			Number:        row.Number,
			WarehouseID:   int64(row.WarehouseID),
			Status:        StockTakeStatus(row.Status),
			Note:          row.Note,
			TakenAt:       row.TakenAt.Time,
			CreatedBy:     row.CreatedBy,
			CreatedAt:     row.CreatedAt.Time,
			WarehouseName: row.WarehouseName,
		})
	}
	return res, nil
}

func (r *Repository) UpdateStockTakeStatus(ctx context.Context, arg sqlc.UpdateStockTakeStatusParams) error {
	return r.queries.UpdateStockTakeStatus(ctx, arg)
}

func (r *Repository) GetValuation(ctx context.Context, warehouseID int64) ([]ValuationEntry, error) {
	rows, err := r.queries.GetStockValuation(ctx, warehouseID)
	if err != nil {
		return nil, err
	}
	var res []ValuationEntry
	for _, row := range rows {
		res = append(res, ValuationEntry{
			WarehouseID:   int64(row.WarehouseID),
			WarehouseName: row.WarehouseName,
			ProductID:     int64(row.ProductID),
			ProductName:   row.ProductName,
			SKU:           row.Sku,
			Qty:           numericToFloat(row.Qty),
			AvgCost:       numericToFloat(row.AvgCost),
			TotalValue:    numericToFloat(row.TotalValue),
		})
	}
	return res, nil
}

func (r *Repository) GetStockBalance(ctx context.Context, arg sqlc.GetStockBalanceParams) (sqlc.InventoryBalance, error) {
	return r.queries.GetStockBalance(ctx, arg)
}

func (r *Repository) GetReorderAlerts(ctx context.Context) ([]ReorderAlert, error) {
	rows, err := r.queries.GetReorderAlerts(ctx)
	if err != nil {
		return nil, err
	}
	var res []ReorderAlert
	for _, row := range rows {
		res = append(res, ReorderAlert{
			ProductID:     int64(row.ID),
			ProductName:   row.Name,
			SKU:           row.Sku,
			MinStock:      numericToFloat(row.MinStock),
			WarehouseID:   int64(row.WarehouseID),
			WarehouseName: row.WarehouseName,
			CurrentQty:    numericToFloat(row.Qty),
		})
	}
	return res, nil
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

func (r *txRepo) InsertStockTake(ctx context.Context, arg sqlc.InsertStockTakeParams) (int64, error) {
	return r.queries.InsertStockTake(ctx, arg)
}

func (r *txRepo) InsertStockTakeLine(ctx context.Context, arg sqlc.InsertStockTakeLineParams) error {
	return r.queries.InsertStockTakeLine(ctx, arg)
}

// --- Stock Adjustments (Transactional) ---

func (r *txRepo) InsertAdjustment(ctx context.Context, arg sqlc.InsertAdjustmentParams) (int64, error) {
	return r.queries.InsertAdjustment(ctx, arg)
}

func (r *txRepo) GetAdjustment(ctx context.Context, id int64) (StockAdjustment, error) {
	row, err := r.queries.GetAdjustment(ctx, id)
	if err != nil {
		return StockAdjustment{}, err
	}
	return mapRowToAdjustment(row), nil
}

func (r *txRepo) InsertAdjustmentLine(ctx context.Context, arg sqlc.InsertAdjustmentLineParams) error {
	return r.queries.InsertAdjustmentLine(ctx, arg)
}

func (r *txRepo) UpdateAdjustmentStatus(ctx context.Context, arg sqlc.UpdateAdjustmentStatusParams) error {
	return r.queries.UpdateAdjustmentStatus(ctx, arg)
}

// --- Stock Adjustments (Main) ---

func (r *Repository) InsertAdjustment(ctx context.Context, arg sqlc.InsertAdjustmentParams) (int64, error) {
	return r.queries.InsertAdjustment(ctx, arg)
}

func (r *Repository) GetAdjustment(ctx context.Context, id int64) (StockAdjustment, error) {
	row, err := r.queries.GetAdjustment(ctx, id)
	if err != nil {
		return StockAdjustment{}, err
	}
	adj := mapRowToAdjustment(row)
	lines, err := r.queries.GetAdjustmentLines(ctx, id)
	if err == nil {
		for _, l := range lines {
			adj.Lines = append(adj.Lines, StockAdjustmentLine{
				ID:           l.ID,
				AdjustmentID: l.AdjustmentID,
				ProductID:    int64(l.ProductID),
				ProductName:  l.ProductName,
				Qty:          numericToFloat(l.Qty),
				Note:         l.Note,
			})
		}
	}
	return adj, nil
}

func (r *Repository) ListAdjustments(ctx context.Context) ([]StockAdjustment, error) {
	rows, err := r.queries.ListAdjustments(ctx)
	if err != nil {
		return nil, err
	}
	var res []StockAdjustment
	for _, row := range rows {
		res = append(res, StockAdjustment{
			ID:            row.ID,
			UUID:          uuid.UUID(row.Uuid.Bytes).String(),
			Number:        row.Number,
			WarehouseID:   int64(row.WarehouseID),
			Status:        StockAdjustmentStatus(row.Status),
			Note:          row.Note,
			AdjustmentAt:  row.AdjustmentAt.Time,
			CreatedBy:     row.CreatedBy,
			CreatedAt:     row.CreatedAt.Time,
			WarehouseName: row.WarehouseName,
		})
	}
	return res, nil
}

func (r *Repository) InsertAdjustmentLine(ctx context.Context, arg sqlc.InsertAdjustmentLineParams) error {
	return r.queries.InsertAdjustmentLine(ctx, arg)
}

func (r *Repository) GetAdjustmentLines(ctx context.Context, adjustmentID int64) ([]StockAdjustmentLine, error) {
	rows, err := r.queries.GetAdjustmentLines(ctx, adjustmentID)
	if err != nil {
		return nil, err
	}
	var res []StockAdjustmentLine
	for _, l := range rows {
		res = append(res, StockAdjustmentLine{
			ID:           l.ID,
			AdjustmentID: l.AdjustmentID,
			ProductID:    int64(l.ProductID),
			ProductName:  l.ProductName,
			Qty:          numericToFloat(l.Qty),
			Note:         l.Note,
		})
	}
	return res, nil
}

func (r *Repository) UpdateAdjustmentStatus(ctx context.Context, arg sqlc.UpdateAdjustmentStatusParams) error {
	return r.queries.UpdateAdjustmentStatus(ctx, arg)
}

func (r *Repository) GetInboundHistory(ctx context.Context, productID int64, warehouseID int64) ([]sqlc.GetInboundHistoryRow, error) {
	return r.queries.GetInboundHistory(ctx, sqlc.GetInboundHistoryParams{
		ProductID:      productID,
		DstWarehouseID: pgtype.Int8{Int64: warehouseID, Valid: true},
	})
}

func mapRowToAdjustment(row sqlc.GetAdjustmentRow) StockAdjustment {
	return StockAdjustment{
		ID:            row.ID,
		UUID:          uuid.UUID(row.Uuid.Bytes).String(),
		Number:        row.Number,
		WarehouseID:   int64(row.WarehouseID),
		Status:        StockAdjustmentStatus(row.Status),
		Note:          row.Note,
		AdjustmentAt:  row.AdjustmentAt.Time,
		CreatedBy:     row.CreatedBy,
		PostedBy:      row.PostedBy.Int64,
		PostedAt:      row.PostedAt.Time,
		CreatedAt:     row.CreatedAt.Time,
		CreatorEmail:  row.CreatorEmail,
		WarehouseName: row.WarehouseName,
	}
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
	_ = n.Scan(fmt.Sprintf("%f", f))
	return n
}
