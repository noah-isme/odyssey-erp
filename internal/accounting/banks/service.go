package banks

import (
	"context"
	"time"
)

const (
	BankStatementDraft      = "DRAFT"
	BankStatementReconciled = "RECONCILED"
	BankLineUnmatched       = "UNMATCHED"
	BankLineSuggested       = "SUGGESTED"
	BankLineMatched         = "MATCHED"
)

// BankStatement is the database-neutral representation shown by the banking
// service and its HTTP handlers.
type BankStatement struct {
	ID              int64
	BankAccountID   int64
	StatementDate   time.Time
	StartingBalance float64
	EndingBalance   float64
	Status          string
	ImportedAt      time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// BankStatementLine is the database-neutral representation of a statement
// line and its reconciliation suggestion.
type BankStatementLine struct {
	ID              int64
	StatementID     int64
	TrxDate         time.Time
	Description     string
	Amount          float64
	ReferenceNumber string
	Status          string
	MatchedDocType  string
	MatchedDocID    *int64
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// Repository owns all SQLC and database-specific details for bank statements.
type Repository interface {
	ImportStatement(ctx context.Context, accountID int64, statementDate time.Time, lines []ParsedStatementLine) (int64, error)
	ConfirmStatement(ctx context.Context, id int64) error
	PerformTransfer(ctx context.Context, req TransferRequest) error
	BankAccountIDForCompany(ctx context.Context, companyID int64) (int64, error)
	ListStatements(ctx context.Context, accountID int64, limit, offset int32) ([]BankStatement, error)
	GetStatement(ctx context.Context, id int64) (BankStatement, error)
	ListStatementLines(ctx context.Context, statementID int64) ([]BankStatementLine, error)
}

// Service implements bank statement import, reconciliation, and transfers.
type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// ImportStatement creates a bank statement from parsed CSV lines and runs auto-matching.
func (s *Service) ImportStatement(ctx context.Context, accountID int64, statementDate time.Time, lines []ParsedStatementLine) (int64, error) {
	return s.repo.ImportStatement(ctx, accountID, statementDate, lines)
}

// ConfirmStatement marks a statement as reconciled and accepts all suggested matches.
func (s *Service) ConfirmStatement(ctx context.Context, id int64) error {
	return s.repo.ConfirmStatement(ctx, id)
}

// PerformTransfer executes a bank transfer by creating a balanced journal entry.
func (s *Service) PerformTransfer(ctx context.Context, req TransferRequest) error {
	if req.FromBankAccountID == req.ToBankAccountID {
		return ErrSameTransferAccount
	}
	if req.Amount <= 0 {
		return ErrInvalidTransferAmount
	}
	return s.repo.PerformTransfer(ctx, req)
}

func (s *Service) BankAccountIDForCompany(ctx context.Context, companyID int64) (int64, error) {
	return s.repo.BankAccountIDForCompany(ctx, companyID)
}

func (s *Service) ListStatements(ctx context.Context, accountID int64, limit, offset int32) ([]BankStatement, error) {
	return s.repo.ListStatements(ctx, accountID, limit, offset)
}

func (s *Service) GetStatement(ctx context.Context, id int64) (BankStatement, error) {
	return s.repo.GetStatement(ctx, id)
}

func (s *Service) ListStatementLines(ctx context.Context, statementID int64) ([]BankStatementLine, error) {
	return s.repo.ListStatementLines(ctx, statementID)
}
