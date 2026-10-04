//go:build integration

package e2e

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hibiken/asynq"
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

// apCase is one processing path of the AP orchestrator.
type apCase struct {
	name           string
	supplierID     int64
	withPO         bool
	qty            float64
	wantRuns       int
	wantExceptions int
	wantExcType    string
	wantStatus     string
}

func apCases(fix apFixture) []apCase {
	return []apCase{
		{name: "missing mapping", supplierID: fix.supplierWithoutPolicy, qty: 1, wantRuns: 0, wantExceptions: 1, wantExcType: "MISSING_MAPPING", wantStatus: "DRAFT"},
		{name: "quantity mismatch", supplierID: fix.supplierWithPolicy, withPO: true, qty: 5, wantRuns: 1, wantExceptions: 1, wantExcType: "MISMATCH", wantStatus: "DRAFT"},
		{name: "matched auto-post", supplierID: fix.supplierWithPolicy, withPO: true, qty: 10, wantRuns: 1, wantExceptions: 0, wantStatus: "POSTED"},
	}
}

func createAPCaseInvoice(t *testing.T, svc *ap.Service, fix apFixture, tc apCase) ap.APInvoice {
	t.Helper()
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
	inv, err := svc.CreateAPInvoice(context.Background(), input)
	require.NoError(t, err)
	return inv
}

// assertAPOutcome asserts the converged state of an invoice for a path: the
// expected matching runs, exceptions, final status and, when posted, that the
// invoice creator is recorded as the poster.
func assertAPOutcome(t *testing.T, p *pgxpool.Pool, fix apFixture, invoiceID int64, tc apCase) {
	t.Helper()
	runs, excs := apCounts(t, p, invoiceID)
	require.Equal(t, tc.wantRuns, runs, "matching runs")
	require.Equal(t, tc.wantExceptions, excs, "exceptions")
	if tc.wantExcType != "" {
		var excType string
		require.NoError(t, p.QueryRow(context.Background(), `SELECT exception_type FROM ap_exceptions WHERE ap_invoice_id=$1`, invoiceID).Scan(&excType))
		require.Equal(t, tc.wantExcType, excType)
	}
	status, postedBy := apInvoiceState(t, p, invoiceID)
	require.Equal(t, tc.wantStatus, status)
	if tc.wantStatus == "POSTED" {
		require.NotNil(t, postedBy)
		require.Equal(t, fix.creatorID, *postedBy, "posted_by must be the invoice creator")
	}
}

func TestAPOrchestrationIntegration(t *testing.T) {
	p := openAPPool(t)
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	fix := seedAPFixture(t, p, suffix)

	for _, tc := range apCases(fix) {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			stack := newAPStack(p)
			inv := createAPCaseInvoice(t, stack.svc, fix, tc)

			// Forged actor before any processing: rejected with no writes.
			err := stack.orchestrator.ProcessInvoice(ctx, inv.ID, fix.forgerID)
			require.ErrorIs(t, err, ap.ErrActorMismatch)
			runs, excs := apCounts(t, p, inv.ID)
			require.Zero(t, runs)
			require.Zero(t, excs)
			status, _ := apInvoiceState(t, p, inv.ID)
			require.Equal(t, "DRAFT", status)

			// Two concurrent deliveries: the first blocks mid-write until the
			// second has returned. The second finds the lock busy and returns
			// the retryable busy error immediately, with no write (it must not
			// look like success: the holder could be an orphan).
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
				require.ErrorIs(t, err, ap.ErrInvoiceProcessingBusy)
				require.NotErrorIs(t, err, asynq.SkipRetry)
			case <-time.After(15 * time.Second):
				close(stack.barrier.release)
				t.Fatal("concurrent duplicate did not return while the first held the lock")
			}
			close(stack.barrier.release)
			require.NoError(t, <-firstErr, "the lock holder processes the invoice")

			// The busy duplicate's retry (or any later redelivery) finds the
			// work done and is a no-op.
			require.NoError(t, stack.orchestrator.ProcessInvoice(ctx, inv.ID, fix.creatorID))
			require.NoError(t, stack.orchestrator.ProcessInvoice(ctx, inv.ID, fix.creatorID))

			assertAPOutcome(t, p, fix, inv.ID, tc)

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

// sqlRecorder is a pgx query tracer that records the statements a pool runs,
// so a test can assert the lock transaction arms its idle bound.
type sqlRecorder struct {
	mu   sync.Mutex
	sqls []string
	args [][]any
}

func (r *sqlRecorder) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sqls = append(r.sqls, data.SQL)
	r.args = append(r.args, data.Args)
	return ctx
}

func (r *sqlRecorder) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func (r *sqlRecorder) index(substr string) (int, []any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, q := range r.sqls {
		if strings.Contains(q, substr) {
			return i, r.args[i]
		}
	}
	return -1, nil
}

// openAPOrphanPool opens a pool that stands for a different worker host: its
// sessions are tagged with application_name so the test can find (and
// terminate) the backend that holds the processing lock.
func openAPOrphanPool(t *testing.T, appName string, rec *sqlRecorder) *pgxpool.Pool {
	t.Helper()
	cfg, err := pgxpool.ParseConfig(os.Getenv("PG_DSN"))
	require.NoError(t, err)
	cfg.ConnConfig.RuntimeParams["application_name"] = appName
	cfg.ConnConfig.Tracer = rec
	p, err := pgxpool.NewWithConfig(context.Background(), cfg)
	require.NoError(t, err)
	t.Cleanup(p.Close)
	return p
}

// holdOrphanedAPLock takes the invoice's processing lock from a separate
// session (the lock of a crashed worker's orphaned backend: its transaction is
// idle, its Go owner is gone, nobody will release it) and returns a release
// func that is safe to call more than once.
func holdOrphanedAPLock(t *testing.T, appName string, invoiceID int64) (rec *sqlRecorder, release func()) {
	t.Helper()
	rec = &sqlRecorder{}
	holder := ap.NewRepository(openAPOrphanPool(t, appName, rec))
	rel, acquired, err := holder.AcquireProcessingLock(context.Background(), invoiceID)
	require.NoError(t, err)
	require.True(t, acquired, "the orphan must get the lock")
	var once sync.Once
	release = func() { once.Do(rel) }
	t.Cleanup(release)
	return rec, release
}

// An orphaned lock (a crashed worker's backend, still idle in transaction)
// makes ProcessInvoice return the retryable busy error with no write, never
// nil: a nil would complete the redelivered asynq task and leave the invoice
// DRAFT with no matching run forever. When the orphan goes away, the retry
// converges to exactly the expected outcome for each path.
func TestAPOrchestrationOrphanedLockIsRetryable(t *testing.T) {
	p := openAPPool(t)
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	fix := seedAPFixture(t, p, suffix)

	for i, tc := range apCases(fix) {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			stack := newAPStack(p)
			close(stack.barrier.release) // no barrier in this test
			inv := createAPCaseInvoice(t, stack.svc, fix, tc)

			appName := fmt.Sprintf("ap-orphan-%s-%d", suffix, i)
			rec, release := holdOrphanedAPLock(t, appName, inv.ID)

			// The orphan is a real idle-in-transaction backend that holds an
			// advisory lock.
			var orphans int
			require.NoError(t, p.QueryRow(ctx, `SELECT COUNT(*) FROM pg_stat_activity a JOIN pg_locks l ON l.pid = a.pid
				WHERE a.application_name = $1 AND a.state = 'idle in transaction' AND l.locktype = 'advisory' AND l.granted`, appName).Scan(&orphans))
			require.Equal(t, 1, orphans)

			// The lock transaction arms the server-side idle bound before it
			// takes the lock, so PostgreSQL terminates an orphaned backend.
			setIdx, setArgs := rec.index("idle_in_transaction_session_timeout")
			lockIdx, _ := rec.index("pg_try_advisory_xact_lock")
			require.GreaterOrEqual(t, setIdx, 0, "lock tx must set idle_in_transaction_session_timeout")
			require.Less(t, setIdx, lockIdx, "the bound must be armed before the lock is taken")
			require.Equal(t, []any{"360000ms"}, setArgs)
			require.Contains(t, rec.sqls[setIdx], "true)", "the bound must be transaction-local")

			// Redeliveries while the orphan holds the lock: retryable busy
			// error, no run, no exception, invoice untouched.
			for attempt := 1; attempt <= 3; attempt++ {
				err := stack.orchestrator.ProcessInvoice(ctx, inv.ID, fix.creatorID)
				require.Error(t, err, "attempt %d: a busy lock must never look like success", attempt)
				require.ErrorIs(t, err, ap.ErrInvoiceProcessingBusy, "attempt %d", attempt)
				require.NotErrorIs(t, err, asynq.SkipRetry, "attempt %d: busy must be retried", attempt)
				runs, excs := apCounts(t, p, inv.ID)
				require.Zero(t, runs, "attempt %d: no matching run while busy", attempt)
				require.Zero(t, excs, "attempt %d: no exception while busy", attempt)
				status, _ := apInvoiceState(t, p, inv.ID)
				require.Equal(t, "DRAFT", status)
			}

			// The orphan goes away; the next retry converges.
			release()
			require.NoError(t, stack.orchestrator.ProcessInvoice(ctx, inv.ID, fix.creatorID))
			assertAPOutcome(t, p, fix, inv.ID, tc)

			// A further redelivery is a no-op.
			require.NoError(t, stack.orchestrator.ProcessInvoice(ctx, inv.ID, fix.creatorID))
			assertAPOutcome(t, p, fix, inv.ID, tc)
		})
	}
}

// When PostgreSQL terminates the orphaned backend (what the idle bound makes it
// do), the xact-scoped advisory lock disappears with it and the next retry
// converges without any cleanup.
func TestAPOrchestrationTerminatedOrphanConverges(t *testing.T) {
	p := openAPPool(t)
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	fix := seedAPFixture(t, p, suffix)
	tc := apCases(fix)[2] // matched auto-post
	stack := newAPStack(p)
	close(stack.barrier.release)
	inv := createAPCaseInvoice(t, stack.svc, fix, tc)

	appName := "ap-orphan-term-" + suffix
	_, _ = holdOrphanedAPLock(t, appName, inv.ID)

	err := stack.orchestrator.ProcessInvoice(ctx, inv.ID, fix.creatorID)
	require.ErrorIs(t, err, ap.ErrInvoiceProcessingBusy)

	var terminated bool
	require.NoError(t, p.QueryRow(ctx, `SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE application_name = $1 AND state = 'idle in transaction'`, appName).Scan(&terminated))
	require.True(t, terminated)
	require.Eventually(t, func() bool {
		var n int
		return p.QueryRow(ctx, `SELECT COUNT(*) FROM pg_stat_activity WHERE application_name = $1`, appName).Scan(&n) == nil && n == 0
	}, 10*time.Second, 50*time.Millisecond)

	require.NoError(t, stack.orchestrator.ProcessInvoice(ctx, inv.ID, fix.creatorID))
	assertAPOutcome(t, p, fix, inv.ID, tc)
}

// The mechanism the lock relies on: a transaction-local
// idle_in_transaction_session_timeout makes PostgreSQL terminate an idle
// session and release its xact-scoped advisory lock, without changing the
// session default.
func TestAPOrchestrationIdleTimeoutReleasesXactLock(t *testing.T) {
	p := openAPPool(t)
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, os.Getenv("PG_DSN"))
	require.NoError(t, err)
	defer func() { _ = conn.Close(context.Background()) }()

	const key = int64(918273645)
	tx, err := conn.Begin(ctx)
	require.NoError(t, err)
	_, err = tx.Exec(ctx, `SELECT set_config('idle_in_transaction_session_timeout', $1, true)`, "1000ms")
	require.NoError(t, err)
	var got bool
	require.NoError(t, tx.QueryRow(ctx, `SELECT pg_try_advisory_xact_lock($1)`, key).Scan(&got))
	require.True(t, got)

	tryLock := func() bool {
		probe, probeErr := p.Begin(ctx)
		if probeErr != nil {
			return false
		}
		defer func() { _ = probe.Rollback(ctx) }()
		var free bool
		if scanErr := probe.QueryRow(ctx, `SELECT pg_try_advisory_xact_lock($1)`, key).Scan(&free); scanErr != nil {
			return false
		}
		return free
	}
	require.False(t, tryLock(), "the lock is held while the session is idle in transaction")
	require.Eventually(t, tryLock, 10*time.Second, 100*time.Millisecond,
		"the idle session must be terminated and release its lock")

	// The dead session reports the termination on its next use.
	_, err = tx.Exec(ctx, `SELECT 1`)
	require.Error(t, err)
}
