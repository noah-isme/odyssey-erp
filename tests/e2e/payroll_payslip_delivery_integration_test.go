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

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/odyssey-erp/odyssey-erp/internal/payroll"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
)

type payslipPDFStub struct{}

func (payslipPDFStub) RenderHTML(context.Context, string) ([]byte, error) {
	return []byte("%PDF-integration"), nil
}

// barrierMailer blocks the first SendEmail until release is closed; later
// calls return immediately. It counts every call.
type barrierMailer struct {
	mu      sync.Mutex
	calls   int
	to      []string
	entered chan struct{}
	release chan struct{}
	err     error
}

func newBarrierMailer() *barrierMailer {
	return &barrierMailer{entered: make(chan struct{}), release: make(chan struct{})}
}

func (m *barrierMailer) SendEmail(_ context.Context, to, _, _ string, _ *shared.Attachment) error {
	m.mu.Lock()
	m.calls++
	m.to = append(m.to, to)
	first := m.calls == 1
	m.mu.Unlock()
	if first && m.entered != nil {
		close(m.entered)
		select {
		case <-m.release:
		case <-time.After(30 * time.Second):
			return errors.New("barrier mailer: release timeout")
		}
	}
	return m.err
}

func (m *barrierMailer) Calls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

func seedPayslip(t *testing.T, p *pgxpool.Pool, suffix string) int64 {
	t.Helper()
	ctx := context.Background()
	var userID, companyID, employeeID, policyID, periodID, runID, lineID, payslipID int64
	require.NoError(t, p.QueryRow(ctx, `INSERT INTO users(email,password_hash) VALUES($1,'integration') RETURNING id`, "payslip-"+suffix+"@example.test").Scan(&userID))
	require.NoError(t, p.QueryRow(ctx, `INSERT INTO companies(code,name,base_currency) VALUES($1,'Payslip integration','IDR') RETURNING id`, "PS-"+suffix).Scan(&companyID))
	require.NoError(t, p.QueryRow(ctx, `INSERT INTO hr_employees(company_id,employee_number,name,email,hire_date) VALUES($1,$2,'Ayu',$3,DATE '2025-01-01') RETURNING id`, companyID, "E-"+suffix, "ayu-"+suffix+"@example.test").Scan(&employeeID))
	require.NoError(t, p.QueryRow(ctx, `INSERT INTO payroll_company_policies(rule_version_id,company_id,effective_from) SELECT id,$1,DATE '2026-01-01' FROM payroll_rule_versions WHERE rule_type='TAX' ORDER BY id LIMIT 1 RETURNING id`, companyID).Scan(&policyID))
	require.NoError(t, p.QueryRow(ctx, `INSERT INTO payroll_periods(company_id,code,starts_on,ends_on,pay_date) VALUES($1,'2026-07',DATE '2026-07-01',DATE '2026-07-31',DATE '2026-07-31') RETURNING id`, companyID).Scan(&periodID))
	require.NoError(t, p.QueryRow(ctx, `INSERT INTO payroll_runs(run_uuid,company_id,period_id,tax_rule_version_id,bpjs_rule_version_id,company_policy_id,created_by)
		SELECT gen_random_uuid(),$1,$2,(SELECT id FROM payroll_rule_versions WHERE rule_type='TAX' ORDER BY id LIMIT 1),(SELECT id FROM payroll_rule_versions WHERE rule_type='BPJS' ORDER BY id LIMIT 1),$3,$4 RETURNING id`, companyID, periodID, policyID, userID).Scan(&runID))
	require.NoError(t, p.QueryRow(ctx, `INSERT INTO payroll_run_lines(run_id,employee_id,ptkp_code,ter_category,base_salary,gross,net_pay,breakdown) VALUES($1,$2,'TK/0','A',10000000,10000000,9000000,'{}'::jsonb) RETURNING id`, runID, employeeID).Scan(&lineID))
	require.NoError(t, p.QueryRow(ctx, `INSERT INTO payroll_payslips(run_line_id,document_key,checksum) VALUES($1,$2,'integration') RETURNING id`, lineID, "payslip-"+suffix).Scan(&payslipID))
	return payslipID
}

func payslipDeliveredAt(t *testing.T, p *pgxpool.Pool, payslipID int64) *time.Time {
	t.Helper()
	var deliveredAt *time.Time
	require.NoError(t, p.QueryRow(context.Background(), `SELECT delivered_at FROM payroll_payslips WHERE id=$1`, payslipID).Scan(&deliveredAt))
	return deliveredAt
}

func openPayslipPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("PG_DSN")
	if dsn == "" {
		t.Skip("PG_DSN is required for payslip delivery integration suite")
	}
	applyAllMigrations(t, dsn)
	p, err := pgxpool.New(context.Background(), dsn)
	require.NoError(t, err)
	t.Cleanup(p.Close)
	return p
}

func TestPayslipDeliveryConcurrentDuplicateSendsOnce(t *testing.T) {
	p := openPayslipPool(t)
	ctx := context.Background()
	payslipID := seedPayslip(t, p, fmt.Sprintf("dup-%d", time.Now().UnixNano()))
	repo := payroll.NewRepository(p)
	mailer := newBarrierMailer()
	processor := payroll.NewPayslipProcessor(repo, payslipPDFStub{}, mailer)

	firstErr := make(chan error, 1)
	go func() { firstErr <- processor.DeliverPayslip(ctx, payslipID) }()

	select {
	case <-mailer.entered:
	case err := <-firstErr:
		t.Fatalf("first delivery returned before reaching the mailer: %v", err)
	case <-time.After(15 * time.Second):
		t.Fatal("first delivery never reached the mailer")
	}

	// Concurrent duplicate while the first delivery is mid-send.
	secondDone := make(chan error, 1)
	go func() { secondDone <- processor.DeliverPayslip(ctx, payslipID) }()
	select {
	case err := <-secondDone:
		require.NoError(t, err, "duplicate delivery must skip, not fail")
	case <-time.After(10 * time.Second):
		close(mailer.release)
		t.Fatal("duplicate delivery blocked instead of skipping")
	}
	require.Equal(t, 1, mailer.Calls(), "duplicate delivery must not send a second email")
	require.Nil(t, payslipDeliveredAt(t, p, payslipID), "not delivered until the first send commits")

	close(mailer.release)
	require.NoError(t, <-firstErr)
	require.Equal(t, 1, mailer.Calls(), "exactly one email")

	deliveredAt := payslipDeliveredAt(t, p, payslipID)
	require.NotNil(t, deliveredAt)

	// Third, sequential redelivery: nothing rendered or sent, delivered_at unchanged.
	invoked := false
	delivered, err := repo.DeliverPayslipOnce(ctx, payslipID, func(context.Context, payroll.PayslipRecord) error {
		invoked = true
		return nil
	})
	require.NoError(t, err)
	require.False(t, delivered)
	require.False(t, invoked)
	require.NoError(t, processor.DeliverPayslip(ctx, payslipID))
	require.Equal(t, 1, mailer.Calls())
	require.True(t, deliveredAt.Equal(*payslipDeliveredAt(t, p, payslipID)), "delivered_at set exactly once")
}

func TestPayslipDeliveryFailedSendLeavesPayslipUndeliveredAndUnlocked(t *testing.T) {
	p := openPayslipPool(t)
	ctx := context.Background()
	payslipID := seedPayslip(t, p, fmt.Sprintf("fail-%d", time.Now().UnixNano()))
	repo := payroll.NewRepository(p)

	failing := &barrierMailer{err: errors.New("smtp unavailable")}
	err := payroll.NewPayslipProcessor(repo, payslipPDFStub{}, failing).DeliverPayslip(ctx, payslipID)
	require.ErrorContains(t, err, "smtp unavailable")
	require.Equal(t, 1, failing.Calls())
	require.Nil(t, payslipDeliveredAt(t, p, payslipID))

	// The row lock was released by the rollback.
	tx, err := p.Begin(ctx)
	require.NoError(t, err)
	var id int64
	require.NoError(t, tx.QueryRow(ctx, `SELECT id FROM payroll_payslips WHERE id=$1 FOR UPDATE NOWAIT`, payslipID).Scan(&id))
	require.NoError(t, tx.Rollback(ctx))

	// The retry delivers and marks it.
	ok := &barrierMailer{}
	require.NoError(t, payroll.NewPayslipProcessor(repo, payslipPDFStub{}, ok).DeliverPayslip(ctx, payslipID))
	require.Equal(t, 1, ok.Calls())
	require.NotNil(t, payslipDeliveredAt(t, p, payslipID))
}

func TestPayslipDeliveryNotFound(t *testing.T) {
	p := openPayslipPool(t)
	repo := payroll.NewRepository(p)
	delivered, err := repo.DeliverPayslipOnce(context.Background(), 9_000_000_000, func(context.Context, payroll.PayslipRecord) error {
		t.Fatal("deliver must not be called for a missing payslip")
		return nil
	})
	require.ErrorIs(t, err, payroll.ErrPayslipNotFound)
	require.False(t, delivered)
}
