package inventory

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

type memoryRepo struct {
	balances map[string]Balance
	cards    []StockCardEntry
	nextID   int64
}

type memoryTx struct {
	repo *memoryRepo
}

func newMemoryRepo() *memoryRepo {
	return &memoryRepo{balances: make(map[string]Balance)}
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
