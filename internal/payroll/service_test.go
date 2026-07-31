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
	run          Run
	approvalID   int64
	posted       int
	lines        []RunLine
	groups       []PostingGroup
	payments     []PaymentInstruction
	rejectedBy   int64
	rejectNote   string
	payslipActor int64
	payslipStaff bool
	payslipLine  RunLine
	payslipErr   error
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
func (s *storeFake) ResetRejected(_ context.Context, _ int64, actorID int64, note string) error {
	s.run.Status = StatusDraft
	s.rejectedBy, s.rejectNote = actorID, note
	return nil
}
func (s *storeFake) PostingData(context.Context, int64) (Run, []PostingGroup, AccountMappings, int64, error) {
	groups := s.groups
	if groups == nil {
		groups = []PostingGroup{{Gross: 10000000, EmployerBPJS: 500000, EmployeeBPJS: 300000, Tax: 200000, OtherDeductions: 100000, Net: 9400000}}
	}
	return s.run, groups, AccountMappings{1, 2, 3, 4, 5}, 9, nil
}
func (s *storeFake) MarkPosted(_ context.Context, _ int64, journal int64) ([]RunLine, error) {
	s.posted++
	s.run.Status = StatusPosted
	s.run.JournalEntryID = &journal
	return s.lines, nil
}
func (s *storeFake) PendingPayslips(context.Context, int64) ([]RunLine, error) { return s.lines, nil }
func (s *storeFake) Payslip(_ context.Context, _ int64, actorID int64, staff bool) (RunLine, error) {
	s.payslipActor, s.payslipStaff = actorID, staff
	return s.payslipLine, s.payslipErr
}
func (s *storeFake) PaymentInstructions(context.Context, int64) ([]PaymentInstruction, error) {
	return s.payments, nil
}

type approvalFake struct{}

func (approvalFake) Submit(context.Context, approvals.Submission) (approvals.Request, error) {
	return approvals.Request{ID: 44}, nil
}

type approvalCapture struct {
	approvalFake
	submission approvals.Submission
}

func (a *approvalCapture) Submit(_ context.Context, submission approvals.Submission) (approvals.Request, error) {
	a.submission = submission
	return approvals.Request{ID: 44}, nil
}

type ledgerFake struct {
	calls int
	input journals.PostingInput
}
type deliveryFake struct {
	calls  int
	failAt int
}

func (d *deliveryFake) EnqueuePayslip(context.Context, RunLine) error {
	d.calls++
	if d.calls == d.failAt {
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

func TestPayrollApprovalAndPostingRecordActors(t *testing.T) {
	run := Run{ID: 7, CompanyID: 1, PeriodID: 2, RunUUID: uuid.New(), Status: StatusDraft, Gross: 10000000, PeriodCode: "2026-07", PayDate: time.Now()}
	store := &storeFake{run: run}
	approval := &approvalCapture{}
	ledger := &ledgerFake{}
	service := NewService(store, approval, ledger, nil)
	_, err := service.Submit(context.Background(), 7, 55)
	require.NoError(t, err)
	require.Equal(t, int64(55), approval.submission.RequesterID)
	require.Equal(t, "PAYROLL", approval.submission.Module)
	require.Equal(t, float64(10000000), approval.submission.Amount)
	_, err = service.Post(context.Background(), 7, 77)
	require.NoError(t, err)
	require.Equal(t, int64(77), ledger.input.PostedBy)
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

func TestPayrollJournalBalancesEachDepartmentAndCostCenter(t *testing.T) {
	department1, department2 := int64(10), int64(20)
	costCenter1, costCenter2 := int64(11), int64(21)
	store := &storeFake{
		run: Run{ID: 7, CompanyID: 1, RunUUID: uuid.New(), Status: StatusApproval, PeriodCode: "P", PayDate: time.Now()},
		groups: []PostingGroup{
			{DepartmentID: &department1, CostCenterID: &costCenter1, Gross: 10000000, EmployerBPJS: 500000, EmployeeBPJS: 300000, Tax: 200000, OtherDeductions: 100000, Net: 9400000},
			{DepartmentID: &department2, CostCenterID: &costCenter2, Gross: 6000000, EmployerBPJS: 300000, EmployeeBPJS: 180000, Tax: 120000, OtherDeductions: 50000, Net: 5650000},
		},
	}
	ledger := &ledgerFake{}
	_, err := NewService(store, nil, ledger, nil).Post(context.Background(), 7, 3)
	require.NoError(t, err)
	require.Len(t, ledger.input.Lines, 10)
	for groupIndex, group := range store.groups {
		lines := ledger.input.Lines[groupIndex*5 : groupIndex*5+5]
		var debit, credit float64
		for _, line := range lines {
			debit += line.Debit
			credit += line.Credit
			require.Equal(t, group.DepartmentID, line.DepartmentID)
			require.Equal(t, group.CostCenterID, line.CostCenterID)
		}
		require.Equal(t, debit, credit)
	}
}

func TestPostedPayslipEnqueueCanBeRetried(t *testing.T) {
	store := &storeFake{run: Run{ID: 7, CompanyID: 1, RunUUID: uuid.New(), Status: StatusApproval, PeriodCode: "P", PayDate: time.Now()}, lines: []RunLine{{PayslipID: 9}, {PayslipID: 10}}}
	delivery := &deliveryFake{failAt: 1}
	service := NewService(store, nil, &ledgerFake{}, delivery)
	_, err := service.Post(context.Background(), 7, 3)
	require.NoError(t, err)
	require.Equal(t, StatusPosted, store.run.Status)
	require.Equal(t, 2, delivery.calls)
}

func TestRejectedApprovalPersistsActorAndNote(t *testing.T) {
	store := &storeFake{run: Run{ID: 7, Status: StatusApproval}}
	err := NewService(store, nil, nil, nil).FinalizeApproval(context.Background(), approvals.Request{DocumentID: 7}, approvals.StatusRejected, 12, "incorrect allowance")
	require.NoError(t, err)
	require.Equal(t, int64(12), store.rejectedBy)
	require.Equal(t, "incorrect allowance", store.rejectNote)
}

func TestBankCSVRequiresPostedRunAndConcealsNoEmployeeData(t *testing.T) {
	store := &storeFake{run: Run{ID: 7, Status: StatusPosted}, payments: []PaymentInstruction{{
		EmployeeNumber: "E-01", EmployeeName: "Ayu", BankCode: "BCA", AccountNumber: "123", AccountName: "Ayu", Amount: 9000000,
	}}}
	data, err := NewService(store, nil, nil, nil).BankCSV(context.Background(), 7)
	require.NoError(t, err)
	require.Contains(t, string(data), "employee_number,employee_name,bank_code,account_number,account_name,amount,currency")
	require.Contains(t, string(data), "E-01,Ayu,BCA,123,Ayu,9000000,IDR")

	store.run.Status = StatusApproval
	_, err = NewService(store, nil, nil, nil).BankCSV(context.Background(), 7)
	require.ErrorIs(t, err, ErrInvalidState)

	store.run.Status = StatusPosted
	store.payments[0].AccountNumber = ""
	_, err = NewService(store, nil, nil, nil).BankCSV(context.Background(), 7)
	require.ErrorIs(t, err, ErrConfiguration)
}

func TestPayslipAccessPassesActorAndStaffScopeToStore(t *testing.T) {
	store := &storeFake{payslipLine: RunLine{EmployeeID: 8}}
	line, err := NewService(store, nil, nil, nil).Payslip(context.Background(), 5, 42, false)
	require.NoError(t, err)
	require.Equal(t, int64(8), line.EmployeeID)
	require.Equal(t, int64(42), store.payslipActor)
	require.False(t, store.payslipStaff)

	_, err = NewService(store, nil, nil, nil).Payslip(context.Background(), 5, 99, true)
	require.NoError(t, err)
	require.Equal(t, int64(99), store.payslipActor)
	require.True(t, store.payslipStaff)
}

func TestPayslipOutboxContinuesAfterFailure(t *testing.T) {
	store := &storeFake{lines: []RunLine{{PayslipID: 1}, {PayslipID: 2}}}
	delivery := &deliveryFake{failAt: 1}
	err := NewOutboxDispatcher(store, delivery).DispatchPending(context.Background())
	require.Error(t, err)
	require.Equal(t, 2, delivery.calls)
}
