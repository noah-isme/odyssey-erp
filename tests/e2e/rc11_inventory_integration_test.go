//go:build integration

package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/odyssey-erp/odyssey-erp/internal/inventory"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
)

// TestRC11StockTakePostAuditsAdjustment replays the J-INV-001 stock take of
// tests/staging/certification_test.go at the service layer: posting a stock
// take with a variance writes one STOCK_TAKE movement and J-INV-001 requires
// a durable inventory_tx audit event carrying its note. At tag
// v0.10.0-rc.10 PostStockTake wrote the movement but no audit row.
func TestRC11StockTakePostAuditsAdjustment(t *testing.T) {
	p, f := newRC11Fixture(t)
	ctx := context.Background()
	_, err := p.Exec(ctx, `INSERT INTO inventory_balances(warehouse_id,product_id,qty,avg_cost) VALUES($1,$2,10,100)`, f.WarehouseID, f.ProductID)
	require.NoError(t, err)
	// cmd/odyssey wiring minus the ledger hooks, which this test does not cover.
	svc := inventory.NewService(inventory.NewRepository(p), shared.NewAuditLogger(p), shared.NewIdempotencyStore(p), inventory.ServiceConfig{}, nil)

	id, err := svc.CreateStockTake(ctx, inventory.CreateStockTakeInput{WarehouseID: f.WarehouseID, Note: "rc11 count", TakenAt: time.Now().UTC(), CreatedBy: f.AdminID})
	require.NoError(t, err)
	require.NoError(t, svc.AddStockTakeLine(ctx, inventory.AddStockTakeLineInput{StockTakeID: id, ProductID: f.ProductID, PhysicalQty: 11, Note: "rc11 line"}))
	require.NoError(t, svc.PostStockTake(ctx, id, f.AdminID))

	var number string
	require.NoError(t, p.QueryRow(ctx, `SELECT number FROM inventory_stock_takes WHERE id=$1`, id).Scan(&number))
	note := "Stock Take adjustment: " + number
	var movements, audits int
	require.NoError(t, p.QueryRow(ctx, `SELECT COUNT(*) FROM inventory_tx WHERE ref_module='STOCK_TAKE' AND note=$1`, note).Scan(&movements))
	require.Equal(t, 1, movements)
	require.NoError(t, p.QueryRow(ctx, `SELECT COUNT(*) FROM audit_logs WHERE entity='inventory_tx' AND meta->>'note'=$1 AND actor_id=$2`, note, f.AdminID).Scan(&audits))
	require.Equal(t, 1, audits, "the stock take adjustment must leave one durable audit event by the posting user")
}
