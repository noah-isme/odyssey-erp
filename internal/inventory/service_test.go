package inventory

import (
	"context"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/require"

	"github.com/odyssey-erp/odyssey-erp/internal/sqlc"
	_ "github.com/odyssey-erp/odyssey-erp/testing"
)

type memoryRepo struct {
	balances map[string]Balance
	cards    []StockCardEntry
	nextID   int64
	takes    map[int64]StockTake
}

type memoryTx struct {
	repo *memoryRepo
}

func newMemoryRepo() *memoryRepo {
	return &memoryRepo{
		balances: make(map[string]Balance),
		takes:    make(map[int64]StockTake),
	}
}

func (r *memoryRepo) balanceKey(warehouseID, productID int64) string {
	return key(warehouseID, productID)
}

func key(warehouseID, productID int64) string {
	return fmt.Sprintf("%d:%d", warehouseID, productID)
}

func (r *memoryRepo) WithTx(ctx context.Context, fn func(context.Context, TxRepository) error) error {
	tx := &memoryTx{repo: r}
	return fn(ctx, tx)
}

func (r *memoryRepo) GetStockCard(ctx context.Context, filter StockCardFilter) ([]StockCardEntry, error) {
	result := make([]StockCardEntry, len(r.cards))
	copy(result, r.cards)
	return result, nil
}

func (r *memoryRepo) GetStockTake(ctx context.Context, id int64) (StockTake, error) {
	return r.takes[id], nil
}

func (r *memoryRepo) ListStockTakes(ctx context.Context) ([]StockTake, error) {
	var res []StockTake
	for _, t := range r.takes {
		res = append(res, t)
	}
	return res, nil
}

func (r *memoryRepo) UpdateStockTakeStatus(ctx context.Context, arg sqlc.UpdateStockTakeStatusParams) error {
	take := r.takes[arg.ID]
	take.Status = StockTakeStatus(arg.Status)
	r.takes[arg.ID] = take
	return nil
}

func (r *memoryRepo) GetValuation(ctx context.Context, warehouseID int64) ([]ValuationEntry, error) {
	return nil, nil
}

func (r *memoryRepo) GetReorderAlerts(ctx context.Context) ([]ReorderAlert, error) {
	return nil, nil
}

func (r *memoryRepo) InsertAdjustment(ctx context.Context, arg sqlc.InsertAdjustmentParams) (int64, error) {
	return 1, nil
}

func (r *memoryRepo) GetAdjustment(ctx context.Context, id int64) (StockAdjustment, error) {
	return StockAdjustment{}, nil
}

func (r *memoryRepo) ListAdjustments(ctx context.Context) ([]StockAdjustment, error) {
	return nil, nil
}

func (r *memoryRepo) InsertAdjustmentLine(ctx context.Context, arg sqlc.InsertAdjustmentLineParams) error {
	return nil
}

func (r *memoryRepo) GetAdjustmentLines(ctx context.Context, adjustmentID int64) ([]StockAdjustmentLine, error) {
	return nil, nil
}

func (r *memoryRepo) UpdateAdjustmentStatus(ctx context.Context, arg sqlc.UpdateAdjustmentStatusParams) error {
	return nil
}

func (r *memoryRepo) GetInboundHistory(ctx context.Context, productID int64, warehouseID int64) ([]sqlc.GetInboundHistoryRow, error) {
	return nil, nil
}
func (r *memoryRepo) GetStockBalance(ctx context.Context, arg sqlc.GetStockBalanceParams) (sqlc.InventoryBalance, error) {
	b, ok := r.balances[key(arg.WarehouseID, arg.ProductID)]
	if !ok {
		return sqlc.InventoryBalance{}, nil
	}
	var qty pgtype.Numeric
	if err := qty.Scan(fmt.Sprintf("%g", b.Qty)); err != nil {
		qty = pgtype.Numeric{} // fallback to zero
	}
	return sqlc.InventoryBalance{
		WarehouseID: b.WarehouseID,
		ProductID:   b.ProductID,
		Qty:         qty,
	}, nil
}


func (tx *memoryTx) InsertTransaction(ctx context.Context, _ Transaction) (int64, error) {
	tx.repo.nextID++
	return tx.repo.nextID, nil
}

func (tx *memoryTx) InsertTransactionLines(ctx context.Context, txID int64, lines []TransactionLine) error {
	return nil
}

func (tx *memoryTx) GetBalanceForUpdate(ctx context.Context, warehouseID, productID int64) (Balance, error) {
	key := tx.repo.balanceKey(warehouseID, productID)
	if bal, ok := tx.repo.balances[key]; ok {
		return bal, nil
	}
	return Balance{WarehouseID: warehouseID, ProductID: productID}, ErrBalanceNotFound
}

func (tx *memoryTx) UpsertBalance(ctx context.Context, balance Balance) error {
	key := tx.repo.balanceKey(balance.WarehouseID, balance.ProductID)
	tx.repo.balances[key] = balance
	return nil
}

func (tx *memoryTx) InsertCardEntry(ctx context.Context, card StockCardEntry, warehouseID, productID int64, txID int64) error {
	tx.repo.cards = append(tx.repo.cards, card)
	return nil
}

func (tx *memoryTx) InsertStockTake(ctx context.Context, arg sqlc.InsertStockTakeParams) (int64, error) {
	tx.repo.nextID++
	tx.repo.takes[tx.repo.nextID] = StockTake{
		ID:          tx.repo.nextID,
		Number:      arg.Number,
		WarehouseID: int64(arg.WarehouseID),
		Status:      StockTakeStatus(arg.Status),
	}
	return tx.repo.nextID, nil
}

func (tx *memoryTx) InsertStockTakeLine(ctx context.Context, arg sqlc.InsertStockTakeLineParams) error {
	take := tx.repo.takes[arg.StockTakeID]
	take.Lines = append(take.Lines, StockTakeLine{
		ProductID:   int64(arg.ProductID),
		SystemQty:   numericToFloat(arg.SystemQty),
		PhysicalQty: numericToFloat(arg.PhysicalQty),
	})
	tx.repo.takes[arg.StockTakeID] = take
	return nil
}

func (tx *memoryTx) InsertAdjustment(ctx context.Context, arg sqlc.InsertAdjustmentParams) (int64, error) {
	return 1, nil
}

func (tx *memoryTx) GetAdjustment(ctx context.Context, id int64) (StockAdjustment, error) {
	return StockAdjustment{}, nil
}

func (tx *memoryTx) InsertAdjustmentLine(ctx context.Context, arg sqlc.InsertAdjustmentLineParams) error {
	return nil
}

func (tx *memoryTx) UpdateAdjustmentStatus(ctx context.Context, arg sqlc.UpdateAdjustmentStatusParams) error {
	return nil
}

func TestAverageMovingCost(t *testing.T) {
	repo := newMemoryRepo()
	svc := NewService(repo, nil, nil, ServiceConfig{}, nil)
	ctx := context.Background()

	entry, err := svc.PostInbound(ctx, InboundInput{WarehouseID: 1, ProductID: 1, Qty: 10, UnitCost: 100000, Note: "GRN#1"})
	require.NoError(t, err)
	require.InDelta(t, 10.0, entry.BalanceQty, 0.0001)
	require.InDelta(t, 100000.0, entry.BalanceCost, 0.01)

	entry, err = svc.PostInbound(ctx, InboundInput{WarehouseID: 1, ProductID: 1, Qty: 5, UnitCost: 120000, Note: "GRN#2"})
	require.NoError(t, err)
	require.InDelta(t, 15.0, entry.BalanceQty, 0.0001)
	require.InDelta(t, 106666.6667, entry.BalanceCost, 0.1)

	entry, err = svc.PostAdjustment(ctx, AdjustmentInput{WarehouseID: 1, ProductID: 1, Qty: -8, Note: "Issue"})
	require.NoError(t, err)
	require.InDelta(t, 7.0, entry.BalanceQty, 0.0001)
	require.InDelta(t, 106666.6667, entry.UnitCost, 0.1)
	require.InDelta(t, 106666.6667, entry.BalanceCost, 0.1)
}

func TestTransfer(t *testing.T) {
	repo := newMemoryRepo()
	svc := NewService(repo, nil, nil, ServiceConfig{}, nil)
	ctx := context.Background()

	_, err := svc.PostInbound(ctx, InboundInput{WarehouseID: 1, ProductID: 1, Qty: 20, UnitCost: 50000, Note: "GRN"})
	require.NoError(t, err)

	out, in, err := svc.PostTransfer(ctx, TransferInput{SrcWarehouse: 1, DstWarehouse: 2, ProductID: 1, Qty: 5, UnitCost: 50000, Note: "Move"})
	require.NoError(t, err)
	require.InDelta(t, 15, out.BalanceQty, 0.0001)
	require.InDelta(t, 5, in.BalanceQty, 0.0001)

	_, _, err = svc.PostTransfer(ctx, TransferInput{SrcWarehouse: 1, DstWarehouse: 2, ProductID: 1, Qty: 50, UnitCost: 50000, Note: "Too much"})
	require.Error(t, err)
}

func TestNegativeStockGuard(t *testing.T) {
	repo := newMemoryRepo()
	svc := NewService(repo, nil, nil, ServiceConfig{}, nil)
	ctx := context.Background()

	_, err := svc.PostAdjustment(ctx, AdjustmentInput{WarehouseID: 1, ProductID: 1, Qty: -1, Note: "negative"})
	require.ErrorIs(t, err, ErrNegativeStock)
}
