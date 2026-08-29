//go:build integration

package procurement

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TestCreateGoodsReceiptSerializesCrossGRNAllocation is an opt-in database
// test for the allocation lock in txRepo.ValidateGRNQuantities. Two requests
// each try to reserve the only unit on an approved PO while an explicit lock
// holder keeps both requests queued. Once released, one request commits and
// the other must observe the committed receipt and fail validation.
func TestCreateGoodsReceiptSerializesCrossGRNAllocation(t *testing.T) {
	dsn := os.Getenv("ODYSSEY_PG_DSN")
	if dsn == "" {
		dsn = os.Getenv("PG_DSN")
	}
	if dsn == "" {
		t.Skip("ODYSSEY_PG_DSN or PG_DSN is required for the PostgreSQL concurrency test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Skipf("PostgreSQL DSN is not usable: %v", err)
	}
	// The blocker, two receipt transactions, and the lock-observation query
	// need independent connections. Raising the pool limit is local to this
	// opt-in test pool and does not alter application configuration.
	if config.MaxConns < 4 {
		config.MaxConns = 4
	}
	config.MinConns = 0
	suffix := strconv.FormatInt(time.Now().UTC().UnixNano(), 10)
	applicationName := "odyssey-grn-concurrency-" + suffix
	if config.ConnConfig.RuntimeParams == nil {
		config.ConnConfig.RuntimeParams = make(map[string]string)
	}
	config.ConnConfig.RuntimeParams["application_name"] = applicationName

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Skipf("PostgreSQL is unavailable: %v", err)
	}
	// Register pool shutdown before fixture cleanup so t.Cleanup's LIFO order
	// keeps the database available while the fixture rows are removed.
	t.Cleanup(func() { pool.Close() })
	if err := pool.Ping(ctx); err != nil {
		t.Skipf("PostgreSQL is unavailable: %v", err)
	}

	assertGRNConcurrencySchema(t, ctx, pool)
	fixture := seedGRNConcurrencyFixture(t, ctx, pool, suffix)
	t.Cleanup(func() { cleanupGRNConcurrencyFixture(fixture, pool) })

	repo := NewRepository(pool)
	service := NewService(nil, repo, nil, nil, nil, nil, nil)

	blocker, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Release()
	blockerTx, err := blocker.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	blockerReleased := false
	defer func() {
		if !blockerReleased {
			_ = blockerTx.Rollback(context.Background())
		}
	}()
	if _, err := blockerTx.Exec(ctx, `SELECT id FROM pos WHERE id = $1 FOR UPDATE`, fixture.poID); err != nil {
		t.Fatal(err)
	}

	inputs := []CreateGRNInput{
		{
			POID:        fixture.poID,
			WarehouseID: fixture.warehouseID,
			SupplierID:  fixture.supplierID,
			Number:      "GRN-CONC-A-" + suffix,
			Lines:       []GRNLineInput{{ProductID: fixture.productID, Qty: 1, UnitCost: 10, SerialNumbers: []string{}}},
		},
		{
			POID:        fixture.poID,
			WarehouseID: fixture.warehouseID,
			SupplierID:  fixture.supplierID,
			Number:      "GRN-CONC-B-" + suffix,
			Lines:       []GRNLineInput{{ProductID: fixture.productID, Qty: 1, UnitCost: 10, SerialNumbers: []string{}}},
		},
	}

	start := make(chan struct{})
	results := make(chan error, len(inputs))
	var workers sync.WaitGroup
	workers.Add(len(inputs))
	for _, input := range inputs {
		input := input
		go func() {
			defer workers.Done()
			<-start
			_, err := service.CreateGoodsReceipt(ctx, input)
			results <- err
		}()
	}
	close(start)

	// Holding the row lock above makes the test genuinely concurrent. Do not
	// release it until both receipt transactions are visible as waiters.
	if err := waitForGRNLockWaiters(ctx, pool, applicationName, 2); err != nil {
		_ = blockerTx.Rollback(context.Background())
		blockerReleased = true
		workers.Wait()
		t.Fatal(err)
	}
	if err := blockerTx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	blockerReleased = true

	workers.Wait()
	close(results)
	var successes, validationFailures int
	for err := range results {
		if err == nil {
			successes++
			continue
		}
		if errors.Is(err, ErrValidation) {
			validationFailures++
			continue
		}
		t.Errorf("receipt attempt failed for an unexpected reason: %v", err)
	}
	if successes != 1 || validationFailures != 1 {
		t.Fatalf("concurrent allocation outcome: successes=%d validation_failures=%d, want one of each", successes, validationFailures)
	}

	var grnCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM grns WHERE po_id = $1 AND number LIKE $2`, fixture.poID, "GRN-CONC-%"+suffix).Scan(&grnCount); err != nil {
		t.Fatal(err)
	}
	if grnCount != 1 {
		t.Fatalf("committed GRN count=%d, want 1", grnCount)
	}
	var lineCount int
	var received string
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*), COALESCE(SUM(gl.qty), 0)::numeric(14,4)::text
		FROM grn_lines gl
		JOIN grns g ON g.id = gl.grn_id
		WHERE g.po_id = $1 AND g.number LIKE $2`, fixture.poID, "GRN-CONC-%"+suffix).Scan(&lineCount, &received); err != nil {
		t.Fatal(err)
	}
	if lineCount != 1 || received != "1.0000" {
		t.Fatalf("committed GRN lines=%d quantity=%s, want one line with 1.0000", lineCount, received)
	}
}

type grnConcurrencyFixture struct {
	companyID   int64
	branchID    int64
	warehouseID int64
	unitID      int64
	categoryID  int64
	supplierID  int64
	productID   int64
	poID        int64
}

func assertGRNConcurrencySchema(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	for _, table := range []string{
		"companies", "branches", "warehouses", "units", "categories", "suppliers", "products",
		"pos", "po_lines", "grns", "grn_lines",
	} {
		var exists bool
		if err := pool.QueryRow(ctx, `SELECT to_regclass($1) IS NOT NULL`, table).Scan(&exists); err != nil {
			t.Skipf("cannot inspect PostgreSQL schema: %v", err)
		}
		if !exists {
			t.Skipf("PostgreSQL schema is incomplete: table %s is missing; apply migrations first", table)
		}
	}
	for _, required := range [][2]string{
		{"pos", "company_id"},
		{"pos", "expected_warehouse_id"},
		{"grn_lines", "lot_number"},
		{"grn_lines", "expiry_date"},
		{"grn_lines", "serial_numbers"},
	} {
		var exists bool
		if err := pool.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM information_schema.columns
				WHERE table_schema = current_schema() AND table_name = $1 AND column_name = $2
			)`, required[0], required[1]).Scan(&exists); err != nil {
			t.Skipf("cannot inspect PostgreSQL schema: %v", err)
		}
		if !exists {
			t.Skipf("PostgreSQL schema is incomplete: column %s.%s is missing; apply migrations first", required[0], required[1])
		}
	}
}

func seedGRNConcurrencyFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool, suffix string) grnConcurrencyFixture {
	t.Helper()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	fixture := grnConcurrencyFixture{}
	fixture.companyID = insertGRNFixtureID(t, ctx, tx,
		`INSERT INTO companies(code, name) VALUES($1, $2) RETURNING id`,
		"GRN-CONC-C-"+suffix, "GRN concurrency company")
	fixture.branchID = insertGRNFixtureID(t, ctx, tx,
		`INSERT INTO branches(company_id, code, name) VALUES($1, $2, $3) RETURNING id`,
		fixture.companyID, "GRN-CONC-BR-"+suffix, "GRN concurrency branch")
	fixture.warehouseID = insertGRNFixtureID(t, ctx, tx,
		`INSERT INTO warehouses(branch_id, code, name) VALUES($1, $2, $3) RETURNING id`,
		fixture.branchID, "GRN-CONC-W-"+suffix, "GRN concurrency warehouse")
	fixture.unitID = insertGRNFixtureID(t, ctx, tx,
		`INSERT INTO units(code, name) VALUES($1, $2) RETURNING id`,
		"GRN-CONC-U-"+suffix, "Each")
	fixture.categoryID = insertGRNFixtureID(t, ctx, tx,
		`INSERT INTO categories(code, name) VALUES($1, $2) RETURNING id`,
		"GRN-CONC-CAT-"+suffix, "GRN concurrency category")
	fixture.supplierID = insertGRNFixtureID(t, ctx, tx,
		`INSERT INTO suppliers(code, name) VALUES($1, $2) RETURNING id`,
		"GRN-CONC-S-"+suffix, "GRN concurrency supplier")
	fixture.productID = insertGRNFixtureID(t, ctx, tx,
		`INSERT INTO products(sku, name, category_id, unit_id, price) VALUES($1, $2, $3, $4, $5) RETURNING id`,
		"GRN-CONC-P-"+suffix, "GRN concurrency product", fixture.categoryID, fixture.unitID, 10)
	fixture.poID = insertGRNFixtureID(t, ctx, tx,
		`INSERT INTO pos(number, supplier_id, status, currency, expected_warehouse_id, note, company_id)
		 VALUES($1, $2, 'APPROVED', 'IDR', $3, $4, $5) RETURNING id`,
		"PO-CONC-"+suffix, fixture.supplierID, fixture.warehouseID, "GRN concurrency PO", fixture.companyID)
	if _, err := tx.Exec(ctx,
		`INSERT INTO po_lines(po_id, product_id, qty, price) VALUES($1, $2, 1.0000, 10.0000)`,
		fixture.poID, fixture.productID); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return fixture
}

func insertGRNFixtureID(t *testing.T, ctx context.Context, tx pgx.Tx, query string, args ...any) int64 {
	t.Helper()
	var id int64
	if err := tx.QueryRow(ctx, query, args...).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func waitForGRNLockWaiters(ctx context.Context, pool *pgxpool.Pool, applicationName string, want int) error {
	deadline := time.Now().Add(5 * time.Second)
	for {
		var count int
		err := pool.QueryRow(ctx, `
			SELECT COUNT(*)
			FROM pg_stat_activity
			WHERE application_name = $1
			  AND pid <> pg_backend_pid()
			  AND state = 'active'
			  AND wait_event_type = 'Lock'
			  AND query LIKE '%SELECT status, supplier_id FROM pos WHERE id = $1 FOR UPDATE%'`, applicationName).Scan(&count)
		if err != nil {
			return fmt.Errorf("observe concurrent PostgreSQL lock waiters: %w", err)
		}
		if count >= want {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("observed %d PostgreSQL lock waiters, want %d", count, want)
		}
		timer := time.NewTimer(10 * time.Millisecond)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func cleanupGRNConcurrencyFixture(fixture grnConcurrencyFixture, pool *pgxpool.Pool) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, _ = pool.Exec(ctx, `DELETE FROM grn_lines WHERE grn_id IN (SELECT id FROM grns WHERE po_id = $1)`, fixture.poID)
	_, _ = pool.Exec(ctx, `DELETE FROM grns WHERE po_id = $1`, fixture.poID)
	_, _ = pool.Exec(ctx, `DELETE FROM po_lines WHERE po_id = $1`, fixture.poID)
	_, _ = pool.Exec(ctx, `DELETE FROM pos WHERE id = $1`, fixture.poID)
	_, _ = pool.Exec(ctx, `DELETE FROM products WHERE id = $1`, fixture.productID)
	_, _ = pool.Exec(ctx, `DELETE FROM suppliers WHERE id = $1`, fixture.supplierID)
	_, _ = pool.Exec(ctx, `DELETE FROM categories WHERE id = $1`, fixture.categoryID)
	_, _ = pool.Exec(ctx, `DELETE FROM units WHERE id = $1`, fixture.unitID)
	_, _ = pool.Exec(ctx, `DELETE FROM warehouses WHERE id = $1`, fixture.warehouseID)
	_, _ = pool.Exec(ctx, `DELETE FROM branches WHERE id = $1`, fixture.branchID)
	_, _ = pool.Exec(ctx, `DELETE FROM companies WHERE id = $1`, fixture.companyID)
}
