//go:build integration

package e2e

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/odyssey-erp/odyssey-erp/internal/ap"
)

// apBarrierRepo wraps the real repository. The first transactional write of
// a matching run or an exception blocks until release is closed, so a
// concurrent duplicate ProcessInvoice call runs while the first one is
// mid-write.
type apBarrierRepo struct {
	ap.Repository
	once    sync.Once
	entered chan struct{}
	release chan struct{}
}

func newAPBarrierRepo(inner ap.Repository) *apBarrierRepo {
	return &apBarrierRepo{Repository: inner, entered: make(chan struct{}), release: make(chan struct{})}
}

func (b *apBarrierRepo) wait() error {
	first := false
	b.once.Do(func() { first = true; close(b.entered) })
	if !first {
		return nil
	}
	select {
	case <-b.release:
		return nil
	case <-time.After(30 * time.Second):
		return errors.New("ap barrier: release timeout")
	}
}

func (b *apBarrierRepo) WithTx(ctx context.Context, fn func(context.Context, ap.TxRepository) error) error {
	return b.Repository.WithTx(ctx, func(txctx context.Context, tx ap.TxRepository) error {
		return fn(txctx, &apBarrierTx{TxRepository: tx, b: b})
	})
}

type apBarrierTx struct {
	ap.TxRepository
	b *apBarrierRepo
}

func (t *apBarrierTx) CreateMatchingRun(ctx context.Context, run ap.MatchingRun) (int64, error) {
	if err := t.b.wait(); err != nil {
		return 0, err
	}
	return t.TxRepository.CreateMatchingRun(ctx, run)
}

func (t *apBarrierTx) CreateAPException(ctx context.Context, exc ap.APException) (int64, error) {
	if err := t.b.wait(); err != nil {
		return 0, err
	}
	return t.TxRepository.CreateAPException(ctx, exc)
}

// Keep the optional capabilities of the real tx repository visible to
// Service.PostAPInvoice.
func (t *apBarrierTx) PGXTx() pgx.Tx {
	return t.TxRepository.(interface{ PGXTx() pgx.Tx }).PGXTx()
}

func (t *apBarrierTx) PostAPInvoiceWithValuation(ctx context.Context, input ap.PostAPInvoiceInput, v ap.APInvoiceValuation) error {
	return t.TxRepository.(ap.TxValuatedAPInvoicePoster).PostAPInvoiceWithValuation(ctx, input, v)
}

type apFixture struct {
	creatorID, forgerID       int64
	supplierWithPolicy        int64
	supplierWithoutPolicy     int64
	productID, poID, poLineID int64
}

func seedAPFixture(t *testing.T, p *pgxpool.Pool, suffix string) apFixture {
	t.Helper()
	ctx := context.Background()
	var f apFixture
	var companyID, categoryID, unitID, branchID, warehouseID, grnID int64
	require.NoError(t, p.QueryRow(ctx, `INSERT INTO users(email,password_hash) VALUES($1,'integration') RETURNING id`, "ap-creator-"+suffix+"@example.test").Scan(&f.creatorID))
	require.NoError(t, p.QueryRow(ctx, `INSERT INTO users(email,password_hash) VALUES($1,'integration') RETURNING id`, "ap-forger-"+suffix+"@example.test").Scan(&f.forgerID))
	require.NoError(t, p.QueryRow(ctx, `INSERT INTO companies(code,name,base_currency) VALUES($1,'AP integration','IDR') RETURNING id`, "AP-"+suffix).Scan(&companyID))
	require.NoError(t, p.QueryRow(ctx, `INSERT INTO suppliers(code,name,company_id) VALUES($1,'Policy supplier',$2) RETURNING id`, "SP-"+suffix, companyID).Scan(&f.supplierWithPolicy))
	require.NoError(t, p.QueryRow(ctx, `INSERT INTO suppliers(code,name,company_id) VALUES($1,'Unmapped supplier',$2) RETURNING id`, "SN-"+suffix, companyID).Scan(&f.supplierWithoutPolicy))
	_, err := p.Exec(ctx, `INSERT INTO ap_matching_policies(name,supplier_id,effective_from) VALUES($1,$2,CURRENT_DATE - 1)`, "Exact "+suffix, f.supplierWithPolicy)
	require.NoError(t, err)
	require.NoError(t, p.QueryRow(ctx, `INSERT INTO categories(code,name,company_id) VALUES($1,'AP cat',$2) RETURNING id`, "CAT-"+suffix, companyID).Scan(&categoryID))
	require.NoError(t, p.QueryRow(ctx, `INSERT INTO units(code,name) VALUES($1,'Unit') RETURNING id`, "U-"+suffix).Scan(&unitID))
	require.NoError(t, p.QueryRow(ctx, `INSERT INTO products(sku,name,category_id,unit_id,price,company_id) VALUES($1,'Widget',$2,$3,100,$4) RETURNING id`, "SKU-"+suffix, categoryID, unitID, companyID).Scan(&f.productID))
	require.NoError(t, p.QueryRow(ctx, `INSERT INTO branches(company_id,code,name) VALUES($1,$2,'AP branch') RETURNING id`, companyID, "BR-"+suffix).Scan(&branchID))
	require.NoError(t, p.QueryRow(ctx, `INSERT INTO warehouses(branch_id,code,name) VALUES($1,$2,'AP wh') RETURNING id`, branchID, "WH-"+suffix).Scan(&warehouseID))
	require.NoError(t, p.QueryRow(ctx, `INSERT INTO pos(number,supplier_id,status,company_id) VALUES($1,$2,'APPROVED',$3) RETURNING id`, "PO-"+suffix, f.supplierWithPolicy, companyID).Scan(&f.poID))
	require.NoError(t, p.QueryRow(ctx, `INSERT INTO po_lines(po_id,product_id,qty,price) VALUES($1,$2,10,100) RETURNING id`, f.poID, f.productID).Scan(&f.poLineID))
	require.NoError(t, p.QueryRow(ctx, `INSERT INTO grns(number,po_id,supplier_id,warehouse_id,status,company_id) VALUES($1,$2,$3,$4,'POSTED',$5) RETURNING id`, "GRN-"+suffix, f.poID, f.supplierWithPolicy, warehouseID, companyID).Scan(&grnID))
	_, err = p.Exec(ctx, `INSERT INTO grn_lines(grn_id,product_id,qty,unit_cost) VALUES($1,$2,10,100)`, grnID, f.productID)
	require.NoError(t, err)
	return f
}

func openAPPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("PG_DSN")
	if dsn == "" {
		t.Skip("PG_DSN is required for AP Orchestration suite")
	}
	applyAllMigrations(t, dsn)
	p, err := pgxpool.New(context.Background(), dsn)
	require.NoError(t, err)
	t.Cleanup(p.Close)
	return p
}

type apStack struct {
	repo         ap.Repository
	barrier      *apBarrierRepo
	svc          *ap.Service
	orchestrator *ap.Orchestrator
}

func newAPStack(p *pgxpool.Pool) apStack {
	base := ap.NewRepository(p)
	barrier := newAPBarrierRepo(base)
	svc := ap.NewService(barrier, nil)
	return apStack{
		repo:         base,
		barrier:      barrier,
		svc:          svc,
		orchestrator: ap.NewOrchestrator(ap.NewMatchingService(barrier), ap.NewExceptionService(barrier), svc, barrier),
	}
}

func apCounts(t *testing.T, p *pgxpool.Pool, invoiceID int64) (runs, exceptions int) {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, p.QueryRow(ctx, `SELECT COUNT(*) FROM ap_matching_runs WHERE ap_invoice_id=$1`, invoiceID).Scan(&runs))
	require.NoError(t, p.QueryRow(ctx, `SELECT COUNT(*) FROM ap_exceptions WHERE ap_invoice_id=$1`, invoiceID).Scan(&exceptions))
	return runs, exceptions
}

func apInvoiceState(t *testing.T, p *pgxpool.Pool, invoiceID int64) (status string, postedBy *int64) {
	t.Helper()
	require.NoError(t, p.QueryRow(context.Background(), `SELECT status, posted_by FROM ap_invoices WHERE id=$1`, invoiceID).Scan(&status, &postedBy))
	return status, postedBy
}

func TestAPOrchestrationIntegration(t *testing.T) {
	p := openAPPool(t)
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	fix := seedAPFixture(t, p, suffix)

	tests := []struct {
		name           string
		supplierID     int64
		withPO         bool
		qty            float64
		wantRuns       int
		wantExceptions int
		wantExcType    string
		wantStatus     string
	}{
		{name: "missing mapping", supplierID: fix.supplierWithoutPolicy, qty: 1, wantRuns: 0, wantExceptions: 1, wantExcType: "MISSING_MAPPING", wantStatus: "DRAFT"},
		{name: "quantity mismatch", supplierID: fix.supplierWithPolicy, withPO: true, qty: 5, wantRuns: 1, wantExceptions: 1, wantExcType: "MISMATCH", wantStatus: "DRAFT"},
		{name: "matched auto-post", supplierID: fix.supplierWithPolicy, withPO: true, qty: 10, wantRuns: 1, wantExceptions: 0, wantStatus: "POSTED"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			stack := newAPStack(p)
			line := ap.CreateAPInvoiceLineInput{ProductID: fix.productID, Description: "Widget", Quantity: tc.qty, UnitPrice: 100}
			input := ap.CreateAPInvoiceInput{
				SupplierID: tc.supplierID,
				Currency:   "IDR",
				DueDate:    time.Now().AddDate(0, 1, 0),
				CreatedBy:  fix.creatorID,
			}
			if tc.withPO {
				input.POID = &fix.poID
				line.POLineID = &fix.poLineID
			}
			input.Lines = []ap.CreateAPInvoiceLineInput{line}
			inv, err := stack.svc.CreateAPInvoice(ctx, input)
			require.NoError(t, err)

			// Forged actor before any processing: rejected with no writes.
			err = stack.orchestrator.ProcessInvoice(ctx, inv.ID, fix.forgerID)
			require.ErrorIs(t, err, ap.ErrActorMismatch)
			runs, excs := apCounts(t, p, inv.ID)
			require.Zero(t, runs)
			require.Zero(t, excs)
			status, _ := apInvoiceState(t, p, inv.ID)
			require.Equal(t, "DRAFT", status)

			// Two concurrent deliveries: the first blocks mid-write until the
			// second has returned.
			firstErr := make(chan error, 1)
			go func() { firstErr <- stack.orchestrator.ProcessInvoice(ctx, inv.ID, fix.creatorID) }()
			select {
			case <-stack.barrier.entered:
			case err := <-firstErr:
				t.Fatalf("first delivery returned before reaching the barrier: %v", err)
			case <-time.After(15 * time.Second):
				t.Fatal("first delivery never reached the barrier")
			}
			secondErr := make(chan error, 1)
			go func() { secondErr <- stack.orchestrator.ProcessInvoice(ctx, inv.ID, fix.creatorID) }()
			select {
			case err := <-secondErr:
				require.NoError(t, err)
			case <-time.After(15 * time.Second):
				close(stack.barrier.release)
				t.Fatal("concurrent duplicate did not return while the first held the lock")
			}
			close(stack.barrier.release)
			require.NoError(t, <-firstErr)

			// Sequential redelivery.
			require.NoError(t, stack.orchestrator.ProcessInvoice(ctx, inv.ID, fix.creatorID))

			runs, excs = apCounts(t, p, inv.ID)
			require.Equal(t, tc.wantRuns, runs, "matching runs")
			require.Equal(t, tc.wantExceptions, excs, "exceptions")
			if tc.wantExcType != "" {
				var excType string
				require.NoError(t, p.QueryRow(ctx, `SELECT exception_type FROM ap_exceptions WHERE ap_invoice_id=$1`, inv.ID).Scan(&excType))
				require.Equal(t, tc.wantExcType, excType)
			}
			status, postedBy := apInvoiceState(t, p, inv.ID)
			require.Equal(t, tc.wantStatus, status)
			if tc.wantStatus == "POSTED" {
				require.NotNil(t, postedBy)
				require.Equal(t, fix.creatorID, *postedBy, "posted_by must be the invoice creator")
			}

			// Forged actor after processing (including on a POSTED invoice).
			err = stack.orchestrator.ProcessInvoice(ctx, inv.ID, fix.forgerID)
			require.ErrorIs(t, err, ap.ErrActorMismatch)

			// Redelivery after a user resolved the exception creates nothing.
			_, err = p.Exec(ctx, `UPDATE ap_exceptions SET status='RESOLVED', resolved_at=NOW() WHERE ap_invoice_id=$1`, inv.ID)
			require.NoError(t, err)
			require.NoError(t, stack.orchestrator.ProcessInvoice(ctx, inv.ID, fix.creatorID))
			runs, excs = apCounts(t, p, inv.ID)
			require.Equal(t, tc.wantRuns, runs, "matching runs after resolved redelivery")
			require.Equal(t, tc.wantExceptions, excs, "exceptions after resolved redelivery")
			status, _ = apInvoiceState(t, p, inv.ID)
			require.Equal(t, tc.wantStatus, status)
		})
	}
}
