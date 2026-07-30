package payroll

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"strconv"

	"github.com/odyssey-erp/odyssey-erp/internal/accounting/journals"
	"github.com/odyssey-erp/odyssey-erp/internal/approvals"
)

var (
	ErrInvalidState  = errors.New("payroll: invalid run state")
	ErrConfiguration = errors.New("payroll: regulatory or account configuration incomplete")
	ErrUnauthorized  = errors.New("payroll: payslip access denied")
)

type Service struct {
	store     Store
	approvals ApprovalEngine
	ledger    JournalPoster
	delivery  PayslipDelivery
}

func NewService(store Store, approvalEngine ApprovalEngine, ledger JournalPoster, delivery PayslipDelivery) *Service {
	return &Service{store: store, approvals: approvalEngine, ledger: ledger, delivery: delivery}
}

func (s *Service) CreateDraft(ctx context.Context, companyID, periodID, actorID int64) (Run, error) {
	if companyID <= 0 || periodID <= 0 || actorID <= 0 {
		return Run{}, ErrInvalidInput
	}
	return s.store.CreateDraft(ctx, companyID, periodID, actorID)
}

func (s *Service) Calculate(ctx context.Context, runID int64) (Run, error) {
	if runID <= 0 {
		return Run{}, ErrInvalidInput
	}
	return s.store.Calculate(ctx, runID)
}

func (s *Service) Submit(ctx context.Context, runID, actorID int64) (Run, error) {
	run, err := s.store.GetRun(ctx, runID)
	if err != nil {
		return Run{}, err
	}
	if run.Status != StatusDraft || run.Gross <= 0 || s.approvals == nil {
		return Run{}, ErrInvalidState
	}
	request, err := s.approvals.Submit(ctx, approvals.Submission{Module: "PAYROLL", DocumentID: run.ID, RequesterID: actorID, CompanyID: &run.CompanyID, Amount: float64(run.Gross)})
	if err != nil {
		return Run{}, err
	}
	if err = s.store.SetApproval(ctx, run.ID, request.ID); err != nil {
		return Run{}, err
	}
	run.Status, run.ApprovalRequestID = StatusApproval, &request.ID
	return run, nil
}

// FinalizeApproval implements approvals.Finalizer. Approval posts and locks the run;
// rejection returns it to draft so corrections never mutate a posted run.
func (s *Service) FinalizeApproval(ctx context.Context, request approvals.Request, status string, actorID int64, note string) error {
	if status == approvals.StatusRejected {
		return s.store.ResetRejected(ctx, request.DocumentID)
	}
	if status != approvals.StatusApproved {
		return ErrInvalidState
	}
	_, err := s.Post(ctx, request.DocumentID, actorID)
	return err
}

func (s *Service) Post(ctx context.Context, runID, actorID int64) (Run, error) {
	current, err := s.store.GetRun(ctx, runID)
	if err != nil {
		return Run{}, err
	}
	if current.Status == StatusPosted {
		if err = s.enqueuePayslips(ctx, current.ID, nil); err != nil {
			return current, err
		}
		return current, nil
	}
	run, groups, mappings, accountingPeriodID, err := s.store.PostingData(ctx, runID)
	if err != nil {
		return Run{}, err
	}
	if run.Status != StatusApproval || s.ledger == nil {
		return Run{}, ErrInvalidState
	}
	if mappings.SalaryExpense == 0 || mappings.EmployerBPJSExpense == 0 || mappings.PayrollPayable == 0 || mappings.PPh21Payable == 0 || mappings.BPJSPayable == 0 || accountingPeriodID == 0 {
		return Run{}, ErrConfiguration
	}
	posting := journals.PostingInput{PeriodID: accountingPeriodID, Date: run.PayDate, SourceModule: "PAYROLL", SourceID: run.RunUUID, Memo: fmt.Sprintf("Payroll %s", run.PeriodCode), PostedBy: actorID}
	for _, group := range groups {
		companyID := run.CompanyID
		posting.Lines = append(posting.Lines,
			journals.PostingLineInput{AccountID: mappings.SalaryExpense, Debit: float64(group.Gross), CompanyID: &companyID, DepartmentID: group.DepartmentID, CostCenterID: group.CostCenterID},
			journals.PostingLineInput{AccountID: mappings.EmployerBPJSExpense, Debit: float64(group.EmployerBPJS), CompanyID: &companyID, DepartmentID: group.DepartmentID, CostCenterID: group.CostCenterID},
			journals.PostingLineInput{AccountID: mappings.PayrollPayable, Credit: float64(group.Net + group.OtherDeductions), CompanyID: &companyID, DepartmentID: group.DepartmentID, CostCenterID: group.CostCenterID},
			journals.PostingLineInput{AccountID: mappings.PPh21Payable, Credit: float64(group.Tax), CompanyID: &companyID, DepartmentID: group.DepartmentID, CostCenterID: group.CostCenterID},
			journals.PostingLineInput{AccountID: mappings.BPJSPayable, Credit: float64(group.EmployeeBPJS + group.EmployerBPJS), CompanyID: &companyID, DepartmentID: group.DepartmentID, CostCenterID: group.CostCenterID},
		)
	}
	journal, err := s.ledger.PostJournal(ctx, posting)
	if err != nil {
		return Run{}, err
	}
	lines, err := s.store.MarkPosted(ctx, run.ID, journal.ID)
	if err != nil {
		return Run{}, err
	}
	run.Status, run.JournalEntryID = StatusPosted, &journal.ID
	if err = s.enqueuePayslips(ctx, run.ID, lines); err != nil {
		return run, err
	}
	return run, nil
}

func (s *Service) enqueuePayslips(ctx context.Context, runID int64, lines []RunLine) error {
	if s.delivery == nil {
		return nil
	}
	var err error
	if lines == nil {
		lines, err = s.store.PendingPayslips(ctx, runID)
		if err != nil {
			return err
		}
	}
	for _, line := range lines {
		if err = s.delivery.EnqueuePayslip(ctx, line); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) Runs(ctx context.Context, companyID int64) ([]Run, error) {
	return s.store.ListRuns(ctx, companyID)
}
func (s *Service) Payslip(ctx context.Context, payslipID, actorID int64, payrollStaff bool) (RunLine, error) {
	if payslipID <= 0 || actorID <= 0 {
		return RunLine{}, ErrInvalidInput
	}
	return s.store.Payslip(ctx, payslipID, actorID, payrollStaff)
}

func (s *Service) BankCSV(ctx context.Context, runID int64) ([]byte, error) {
	run, err := s.store.GetRun(ctx, runID)
	if err != nil {
		return nil, err
	}
	if run.Status != StatusPosted {
		return nil, ErrInvalidState
	}
	items, err := s.store.PaymentInstructions(ctx, runID)
	if err != nil {
		return nil, err
	}
	var output bytes.Buffer
	writer := csv.NewWriter(&output)
	_ = writer.Write([]string{"employee_number", "employee_name", "bank_code", "account_number", "account_name", "amount", "currency"})
	for _, item := range items {
		if item.BankCode == "" || item.AccountNumber == "" {
			return nil, ErrConfiguration
		}
		_ = writer.Write([]string{item.EmployeeNumber, item.EmployeeName, item.BankCode, item.AccountNumber, item.AccountName, strconv.FormatInt(int64(item.Amount), 10), "IDR"})
	}
	writer.Flush()
	if err = writer.Error(); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}
