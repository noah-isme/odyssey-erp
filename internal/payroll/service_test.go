package payroll

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/odyssey-erp/odyssey-erp/internal/accounting/journals"
	"github.com/odyssey-erp/odyssey-erp/internal/approvals"
	"github.com/stretchr/testify/require"
)

type storeFake struct {
	run        Run
	approvalID int64
	posted     int
	lines      []RunLine
}

func (s *storeFake) CreateDraft(context.Context, int64, int64, int64) (Run, error) { return s.run, nil }
func (s *storeFake) Calculate(context.Context, int64) (Run, error)                 { return s.run, nil }
func (s *storeFake) GetRun(context.Context, int64) (Run, error)                    { return s.run, nil }
func (s *storeFake) ListRuns(context.Context, int64) ([]Run, error)                { return []Run{s.run}, nil }
func (s *storeFake) SetApproval(_ context.Context, _ int64, id int64) error {
	s.approvalID = id
	s.run.Status = StatusApproval
	return nil
}
func (s *storeFake) ResetRejected(context.Context, int64) error {
	s.run.Status = StatusDraft
	return nil
}
func (s *storeFake) PostingData(context.Context, int64) (Run, []PostingGroup, AccountMappings, int64, error) {
	return s.run, []PostingGroup{{Gross: 10000000, EmployerBPJS: 500000, EmployeeBPJS: 300000, Tax: 200000, OtherDeductions: 100000, Net: 9400000}}, AccountMappings{1, 2, 3, 4, 5}, 9, nil
}
func (s *storeFake) MarkPosted(_ context.Context, _ int64, journal int64) ([]RunLine, error) {
	s.posted++
	s.run.Status = StatusPosted
	s.run.JournalEntryID = &journal
	return s.lines, nil
}
func (s *storeFake) PendingPayslips(context.Context, int64) ([]RunLine, error) { return s.lines, nil }
func (s *storeFake) Payslip(context.Context, int64, int64, bool) (RunLine, error) {
	return RunLine{}, nil
}
func (s *storeFake) PaymentInstructions(context.Context, int64) ([]PaymentInstruction, error) {
	return nil, nil
}

type approvalFake struct{}

func (approvalFake) Submit(context.Context, approvals.Submission) (approvals.Request, error) {
	return approvals.Request{ID: 44}, nil
}

type ledgerFake struct {
	calls int
	input journals.PostingInput
}
type deliveryFake struct {
	calls int
	fail  bool
}

func (d *deliveryFake) EnqueuePayslip(context.Context, RunLine) error {
	d.calls++
	if d.fail {
		return errors.New("queue unavailable")
	}
	return nil
}

func (l *ledgerFake) PostJournal(_ context.Context, in journals.PostingInput) (journals.JournalEntry, error) {
	l.calls++
	l.input = in
	return journals.JournalEntry{ID: 88}, nil
}

func TestPayrollApprovalAndRepeatedPostIdempotency(t *testing.T) {
	runUUID := uuid.NewSHA1(uuid.Nil, []byte("payroll:1:2"))
	store := &storeFake{run: Run{ID: 7, CompanyID: 1, PeriodID: 2, RunUUID: runUUID, Status: StatusDraft, Gross: 10000000, PeriodCode: "2026-07", PayDate: time.Date(2026, 7, 25, 0, 0, 0, 0, time.UTC)}}
	ledger := &ledgerFake{}
	service := NewService(store, approvalFake{}, ledger, nil)
	run, err := service.Submit(context.Background(), 7, 3)
	require.NoError(t, err)
	require.Equal(t, StatusApproval, run.Status)
	require.Equal(t, int64(44), store.approvalID)
	run, err = service.Post(context.Background(), 7, 3)
	require.NoError(t, err)
	require.Equal(t, StatusPosted, run.Status)
	_, err = service.Post(context.Background(), 7, 3)
	require.NoError(t, err)
	require.Equal(t, 1, ledger.calls)
	require.Equal(t, 1, store.posted)
	require.Equal(t, runUUID, ledger.input.SourceID)
}

func TestPayrollJournalIsBalanced(t *testing.T) {
	store := &storeFake{run: Run{ID: 7, CompanyID: 1, RunUUID: uuid.New(), Status: StatusApproval, PeriodCode: "P", PayDate: time.Now()}}
	ledger := &ledgerFake{}
	_, err := NewService(store, nil, ledger, nil).Post(context.Background(), 7, 3)
	require.NoError(t, err)
	var debit, credit float64
	for _, line := range ledger.input.Lines {
		debit += line.Debit
		credit += line.Credit
	}
	require.Equal(t, debit, credit)
}

func TestPostedPayslipEnqueueCanBeRetried(t *testing.T) {
	store := &storeFake{run: Run{ID: 7, CompanyID: 1, RunUUID: uuid.New(), Status: StatusApproval, PeriodCode: "P", PayDate: time.Now()}, lines: []RunLine{{PayslipID: 9}}}
	delivery := &deliveryFake{fail: true}
	service := NewService(store, nil, &ledgerFake{}, delivery)
	_, err := service.Post(context.Background(), 7, 3)
	require.Error(t, err)
	require.Equal(t, StatusPosted, store.run.Status)
	delivery.fail = false
	_, err = service.Post(context.Background(), 7, 3)
	require.NoError(t, err)
	require.Equal(t, 2, delivery.calls)
}
