package payroll

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/odyssey-erp/odyssey-erp/internal/accounting/journals"
	"github.com/odyssey-erp/odyssey-erp/internal/approvals"
)

const (
	StatusDraft    = "DRAFT"
	StatusApproval = "APPROVAL"
	StatusPosted   = "POSTED"
)

type Run struct {
	ID, CompanyID, PeriodID                       int64
	RunUUID                                       uuid.UUID
	RunType, PeriodCode, Status                   string
	TaxRuleVersionID, BPJSRuleVersionID, PolicyID int64
	ApprovalRequestID, JournalEntryID             *int64
	PayDate                                       time.Time
	CreatedBy                                     int64
	Gross, NetPay                                 Money
	CreatedAt                                     time.Time
}

type RunLine struct {
	ID, PayslipID, RunID, EmployeeID int64
	EmployeeName, Email, PeriodCode  string
	UserID, ManagerUserID            *int64
	DepartmentID                     *int64
	CostCenterID                     *int64
	Result                           Result
}

type PaymentInstruction struct {
	EmployeeNumber, EmployeeName, BankCode, AccountNumber, AccountName string
	Amount                                                             Money
}

type AccountMappings struct {
	SalaryExpense, EmployerBPJSExpense, PayrollPayable, PPh21Payable, BPJSPayable int64
}

type PostingGroup struct {
	DepartmentID, CostCenterID                                   *int64
	Gross, EmployerBPJS, EmployeeBPJS, Tax, OtherDeductions, Net Money
}

type Store interface {
	CreateDraft(context.Context, int64, int64, int64) (Run, error)
	Calculate(context.Context, int64) (Run, error)
	GetRun(context.Context, int64) (Run, error)
	ListRuns(context.Context, int64) ([]Run, error)
	SetApproval(context.Context, int64, int64) error
	ResetRejected(context.Context, int64, int64, string) error
	PostingData(context.Context, int64) (Run, []PostingGroup, AccountMappings, int64, error)
	MarkPosted(context.Context, int64, int64) ([]RunLine, error)
	PendingPayslips(context.Context, int64) ([]RunLine, error)
	Payslip(context.Context, int64, int64, bool) (RunLine, error)
	PaymentInstructions(context.Context, int64) ([]PaymentInstruction, error)
}

type ApprovalEngine interface {
	Submit(context.Context, approvals.Submission) (approvals.Request, error)
}

type JournalPoster interface {
	PostJournal(context.Context, journals.PostingInput) (journals.JournalEntry, error)
}

type PayslipDelivery interface {
	EnqueuePayslip(context.Context, RunLine) error
}

type PayslipRecord struct {
	ID         int64
	Line       RunLine
	PeriodCode string
}

type PayslipStore interface {
	DeliveryPayslip(context.Context, int64) (PayslipRecord, error)
	MarkPayslipDelivered(context.Context, int64) error
}
