package banking

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/odyssey-erp/odyssey-erp/internal/accounting/journals"
	"github.com/odyssey-erp/odyssey-erp/internal/sqlc"
)

// Repository describes database operations.
type Repository interface {
	CreateBankAccount(ctx context.Context, arg sqlc.CreateBankAccountParams) (sqlc.BankAccount, error)
	GetBankAccount(ctx context.Context, id int64) (sqlc.BankAccount, error)
	ListBankAccounts(ctx context.Context, companyID int64) ([]sqlc.BankAccount, error)
	UpdateBankAccount(ctx context.Context, arg sqlc.UpdateBankAccountParams) error
	CreateBankTransaction(ctx context.Context, arg sqlc.CreateBankTransactionParams) (sqlc.BankTransaction, error)
	GetBankTransaction(ctx context.Context, id pgtype.UUID) (sqlc.BankTransaction, error)
	ListBankTransactions(ctx context.Context, bankAccountID int64) ([]sqlc.BankTransaction, error)
	UpdateBankTransactionStatus(ctx context.Context, arg sqlc.UpdateBankTransactionStatusParams) error
}

// JournalPoster handles GL integration.
type JournalPoster interface {
	PostJournal(ctx context.Context, input journals.PostingInput) (journals.JournalEntry, error)
}

// Service handles banking logic.
type Service struct {
	repo   Repository
	logger *slog.Logger
	poster JournalPoster
}

// NewService creates a banking service.
func NewService(repo Repository, logger *slog.Logger, poster JournalPoster) *Service {
	return &Service{
		repo:   repo,
		logger: logger,
		poster: poster,
	}
}

// CreateAccountInput defines payload for creating an account.
type CreateAccountInput struct {
	CompanyID      int64
	Name           string
	AccountNumber  string
	Currency       string
	GLAccountID    int64
	InitialBalance float64
}

// CreateBankAccount creates a new bank account.
func (s *Service) CreateBankAccount(ctx context.Context, input CreateAccountInput) (sqlc.BankAccount, error) {
	if input.Name == "" || input.AccountNumber == "" {
		return sqlc.BankAccount{}, fmt.Errorf("name and account number are required")
	}

	arg := sqlc.CreateBankAccountParams{
		CompanyID:     input.CompanyID,
		Name:          input.Name,
		AccountNumber: input.AccountNumber,
		Currency:      input.Currency,
		GlAccountID:   input.GLAccountID,
		IsActive:      true,
	}
	// Handle InitialBalance conversion to Numeric
	var bal pgtype.Numeric
	bal.Scan(fmt.Sprintf("%f", input.InitialBalance))
	arg.InitialBalance = bal

	account, err := s.repo.CreateBankAccount(ctx, arg)
	if err != nil {
		s.logger.Error("failed to create bank account", slog.Any("error", err))
		return sqlc.BankAccount{}, err
	}

	return account, nil
}

// CreateTransactionInput defines payload for creating a transaction.
type CreateTransactionInput struct {
	BankAccountID   int64
	Date            time.Time
	Amount          float64
	Description     string
	Reference       string
	ContraAccountID int64
	PeriodID        int64 // Required for GL posting
	CreatedBy       int64
}

// CreateTransaction creates a bank transaction and posts to GL.
func (s *Service) CreateBankTransaction(ctx context.Context, input CreateTransactionInput) (sqlc.BankTransaction, error) {
	if input.BankAccountID == 0 || input.Amount == 0 || input.ContraAccountID == 0 {
		return sqlc.BankTransaction{}, fmt.Errorf("bank account, amount, and contra account are required")
	}

	// 1. Get Bank Account to find its GL Account
	bankAccount, err := s.repo.GetBankAccount(ctx, input.BankAccountID)
	if err != nil {
		return sqlc.BankTransaction{}, fmt.Errorf("invalid bank account: %w", err)
	}

	// 2. Prepare Transaction Record
	txnID := uuid.New()
	var amount pgtype.Numeric
	amount.Scan(fmt.Sprintf("%f", input.Amount))

	arg := sqlc.CreateBankTransactionParams{
		ID:            pgtype.UUID{Bytes: txnID, Valid: true},
		BankAccountID: input.BankAccountID,
		Date:          pgtype.Date{Time: input.Date, Valid: true},
		Amount:        amount,
		Description:   input.Description,
		Reference:     pgtype.Text{String: input.Reference, Valid: input.Reference != ""},
		Status:        "CLEARED", // Manual entry is considered cleared for now
	}

	// 3. Create Transaction in DB
	txn, err := s.repo.CreateBankTransaction(ctx, arg)
	if err != nil {
		return sqlc.BankTransaction{}, err
	}

	// 4. Post to GL
	// Logic:
	// If Amount > 0 (Deposit): Debit Bank GL, Credit Contra GL
	// If Amount < 0 (Withdrawal): Credit Bank GL, Debit Contra GL (using absolute amount)

	bankLine := journals.PostingLineInput{
		AccountID: bankAccount.GlAccountID,
	}
	contraLine := journals.PostingLineInput{
		AccountID: input.ContraAccountID,
	}

	absAmount := input.Amount
	if absAmount < 0 {
		absAmount = -absAmount
	}

	if input.Amount > 0 {
		// Deposit
		bankLine.Debit = absAmount
		contraLine.Credit = absAmount
	} else {
		// Withdrawal
		bankLine.Credit = absAmount
		contraLine.Debit = absAmount
	}

	glInput := journals.PostingInput{
		PeriodID:     input.PeriodID,
		Date:         input.Date,
		SourceModule: "FINANCE.BANKING",
		SourceID:     txnID,
		Memo:         input.Description,
		PostedBy:     input.CreatedBy,
		Lines:        []journals.PostingLineInput{bankLine, contraLine},
	}

	journal, err := s.poster.PostJournal(ctx, glInput)
	if err != nil {
		s.logger.Error("failed to post journal for bank txn", slog.Any("error", err), slog.String("txn_id", txnID.String()))
		// We created the transaction but failed to post. Mark as PENDING or rollback?
		// For simplicity in this non-tx method, we update status to PENDING
		_ = s.repo.UpdateBankTransactionStatus(ctx, sqlc.UpdateBankTransactionStatusParams{
			ID:     pgtype.UUID{Bytes: txnID, Valid: true},
			Status: "PENDING",
		})
		return txn, fmt.Errorf("transaction created but GL posting failed: %w", err)
	}

	// 5. Update Transaction with Journal ID
	err = s.repo.UpdateBankTransactionStatus(ctx, sqlc.UpdateBankTransactionStatusParams{
		ID:          pgtype.UUID{Bytes: txnID, Valid: true},
		Status:      "CLEARED",
		GlJournalID: pgtype.Int8{Int64: journal.ID, Valid: true},
	})
	if err != nil {
		s.logger.Error("failed to link journal to bank txn", slog.Any("error", err))
	}

	txn.GlJournalID = pgtype.Int8{Int64: journal.ID, Valid: true}
	return txn, nil
}

// UpdateBankTransactionStatus updates the status of a transaction.
func (s *Service) ReconcileTransaction(ctx context.Context, id uuid.UUID) error {
	arg := sqlc.UpdateBankTransactionStatusParams{
		ID:     pgtype.UUID{Bytes: id, Valid: true},
		Status: "RECONCILED",
	}
	return s.repo.UpdateBankTransactionStatus(ctx, arg)
}

// TransferInput defines payload for bank transfer.
type TransferInput struct {
	FromAccountID int64
	ToAccountID   int64
	Amount        float64
	Date          time.Time
	Description   string
	Reference     string
	PeriodID      int64
	CreatedBy     int64
}

// TransferFunds records a transfer between two bank accounts with a single GL entry.
func (s *Service) TransferFunds(ctx context.Context, input TransferInput) error {
	if input.FromAccountID == input.ToAccountID {
		return fmt.Errorf("source and destination accounts must be different")
	}
	if input.Amount <= 0 {
		return fmt.Errorf("transfer amount must be positive")
	}

	// 1. Get Accounts
	fromAcct, err := s.repo.GetBankAccount(ctx, input.FromAccountID)
	if err != nil {
		return fmt.Errorf("source account not found: %w", err)
	}
	toAcct, err := s.repo.GetBankAccount(ctx, input.ToAccountID)
	if err != nil {
		return fmt.Errorf("destination account not found: %w", err)
	}

	// 2. Prepare GL Posting
	// Logic: Debit Destination Bank GL, Credit Source Bank GL
	glInput := journals.PostingInput{
		PeriodID:     input.PeriodID,
		Date:         input.Date,
		SourceModule: "FINANCE.BANKING.TRANSFER",
		Memo:         input.Description,
		PostedBy:     input.CreatedBy,
		Lines: []journals.PostingLineInput{
			{AccountID: toAcct.GlAccountID, Debit: input.Amount},
			{AccountID: fromAcct.GlAccountID, Credit: input.Amount},
		},
	}

	journal, err := s.poster.PostJournal(ctx, glInput)
	if err != nil {
		return fmt.Errorf("failed to post GL entry for transfer: %w", err)
	}

	// 3. Create Bank Transactions
	// Withdrawal from source
	fromTxnID := uuid.New()
	var fromAmt pgtype.Numeric
	fromAmt.Scan(fmt.Sprintf("%f", -input.Amount))
	_, err = s.repo.CreateBankTransaction(ctx, sqlc.CreateBankTransactionParams{
		ID:            pgtype.UUID{Bytes: fromTxnID, Valid: true},
		BankAccountID: input.FromAccountID,
		Date:          pgtype.Date{Time: input.Date, Valid: true},
		Amount:        fromAmt,
		Description:   fmt.Sprintf("Transfer to %s: %s", toAcct.Name, input.Description),
		Reference:     pgtype.Text{String: input.Reference, Valid: input.Reference != ""},
		Status:        "CLEARED",
		GlJournalID:   pgtype.Int8{Int64: journal.ID, Valid: true},
	})
	if err != nil {
		return fmt.Errorf("failed to record withdrawal: %w", err)
	}

	// Deposit to destination
	toTxnID := uuid.New()
	var toAmt pgtype.Numeric
	toAmt.Scan(fmt.Sprintf("%f", input.Amount))
	_, err = s.repo.CreateBankTransaction(ctx, sqlc.CreateBankTransactionParams{
		ID:            pgtype.UUID{Bytes: toTxnID, Valid: true},
		BankAccountID: input.ToAccountID,
		Date:          pgtype.Date{Time: input.Date, Valid: true},
		Amount:        toAmt,
		Description:   fmt.Sprintf("Transfer from %s: %s", fromAcct.Name, input.Description),
		Reference:     pgtype.Text{String: input.Reference, Valid: input.Reference != ""},
		Status:        "CLEARED",
		GlJournalID:   pgtype.Int8{Int64: journal.ID, Valid: true},
	})
	if err != nil {
		return fmt.Errorf("failed to record deposit: %w", err)
	}

	return nil
}

// ListBankAccounts returns all accounts for a company.
func (s *Service) ListBankAccounts(ctx context.Context, companyID int64) ([]sqlc.BankAccount, error) {
	return s.repo.ListBankAccounts(ctx, companyID)
}

// ListBankTransactions returns transactions for an account.
func (s *Service) ListBankTransactions(ctx context.Context, bankAccountID int64) ([]sqlc.BankTransaction, error) {
	return s.repo.ListBankTransactions(ctx, bankAccountID)
}
