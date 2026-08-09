package banking

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/odyssey-erp/odyssey-erp/internal/accounting/journals"
	"github.com/odyssey-erp/odyssey-erp/internal/finance/automation"
)

// Repository describes persistence operations without exposing generated SQL types.
type Repository interface {
	CreateBankAccount(ctx context.Context, input BankAccountCreate) (BankAccount, error)
	GetBankAccount(ctx context.Context, id int64) (BankAccount, error)
	ListBankAccounts(ctx context.Context, companyID int64) ([]BankAccount, error)
	CreateBankTransaction(ctx context.Context, input BankTransactionCreate) (BankTransaction, error)
	ListBankTransactions(ctx context.Context, bankAccountID int64) ([]BankTransaction, error)
	UpdateBankTransactionStatus(ctx context.Context, update BankTransactionStatusUpdate) error
	CreateStatementImportRun(ctx context.Context, input StatementImportRunCreate) (StatementImportRun, error)
	CreateBankStatement(ctx context.Context, input BankStatementCreate) (BankStatement, error)
	CreateBankStatementLine(ctx context.Context, input BankStatementLineCreate) error
	FindOpenPeriod(ctx context.Context, companyID int64, date time.Time) (int64, error)
	BankTransactionExists(ctx context.Context, bankAccountID int64, externalRef, fingerprint string) (bool, error)
}

// BankAccountSummary pairs an account with its ledger balance.
type BankAccountSummary struct {
	Account BankAccount
	Balance float64
}

// BankTransactionSummary adds the running balance after a transaction.
type BankTransactionSummary struct {
	Transaction    BankTransaction
	RunningBalance float64
}

// ImportResult reports imported and skipped statement rows.
type ImportResult struct {
	Imported int
	Skipped  int
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
	return &Service{repo: repo, logger: logger, poster: poster}
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
func (s *Service) CreateBankAccount(ctx context.Context, input CreateAccountInput) (BankAccount, error) {
	if input.Name == "" || input.AccountNumber == "" {
		return BankAccount{}, fmt.Errorf("name and account number are required")
	}

	account, err := s.repo.CreateBankAccount(ctx, BankAccountCreate{
		CompanyID:      input.CompanyID,
		Name:           input.Name,
		AccountNumber:  input.AccountNumber,
		Currency:       input.Currency,
		GLAccountID:    input.GLAccountID,
		InitialBalance: input.InitialBalance,
		IsActive:       true,
	})
	if err != nil {
		s.logError("failed to create bank account", err)
		return BankAccount{}, err
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

// CreateBankTransaction creates a bank transaction and posts to GL.
func (s *Service) CreateBankTransaction(ctx context.Context, input CreateTransactionInput) (BankTransaction, error) {
	if input.BankAccountID == 0 || input.Amount == 0 || input.ContraAccountID == 0 {
		return BankTransaction{}, fmt.Errorf("bank account, amount, and contra account are required")
	}

	bankAccount, err := s.repo.GetBankAccount(ctx, input.BankAccountID)
	if err != nil {
		return BankTransaction{}, fmt.Errorf("invalid bank account: %w", err)
	}

	txnID := uuid.New()
	txn, err := s.repo.CreateBankTransaction(ctx, BankTransactionCreate{
		ID:            txnID,
		BankAccountID: input.BankAccountID,
		Date:          input.Date,
		Amount:        input.Amount,
		Description:   input.Description,
		Reference:     input.Reference,
		Status:        "CLEARED",
	})
	if err != nil {
		return BankTransaction{}, err
	}

	bankLine := journals.PostingLineInput{AccountID: bankAccount.GLAccountID}
	contraLine := journals.PostingLineInput{AccountID: input.ContraAccountID}
	absAmount := input.Amount
	if absAmount < 0 {
		absAmount = -absAmount
	}
	if input.Amount > 0 {
		bankLine.Debit = absAmount
		contraLine.Credit = absAmount
	} else {
		bankLine.Credit = absAmount
		contraLine.Debit = absAmount
	}

	journal, err := s.poster.PostJournal(ctx, journals.PostingInput{
		PeriodID:     input.PeriodID,
		Date:         input.Date,
		SourceModule: "FINANCE.BANKING",
		SourceID:     txnID,
		Memo:         input.Description,
		PostedBy:     input.CreatedBy,
		Lines:        []journals.PostingLineInput{bankLine, contraLine},
	})
	if err != nil {
		s.logError("failed to post journal for bank txn", err)
		_ = s.repo.UpdateBankTransactionStatus(ctx, BankTransactionStatusUpdate{ID: txnID, Status: "PENDING"})
		return txn, fmt.Errorf("transaction created but GL posting failed: %w", err)
	}

	journalID := journal.ID
	if err := s.repo.UpdateBankTransactionStatus(ctx, BankTransactionStatusUpdate{
		ID:          txnID,
		Status:      "CLEARED",
		GLJournalID: &journalID,
	}); err != nil {
		s.logError("failed to link journal to bank txn", err)
	}
	txn.GLJournalID = &journalID
	return txn, nil
}

// ReconcileTransaction updates the status of a transaction.
func (s *Service) ReconcileTransaction(ctx context.Context, id uuid.UUID) error {
	return s.repo.UpdateBankTransactionStatus(ctx, BankTransactionStatusUpdate{ID: id, Status: "RECONCILED"})
}

// TransferInput defines payload for a bank transfer.
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

	fromAcct, err := s.repo.GetBankAccount(ctx, input.FromAccountID)
	if err != nil {
		return fmt.Errorf("source account not found: %w", err)
	}
	toAcct, err := s.repo.GetBankAccount(ctx, input.ToAccountID)
	if err != nil {
		return fmt.Errorf("destination account not found: %w", err)
	}

	journal, err := s.poster.PostJournal(ctx, journals.PostingInput{
		PeriodID:     input.PeriodID,
		Date:         input.Date,
		SourceModule: "FINANCE.BANKING.TRANSFER",
		Memo:         input.Description,
		PostedBy:     input.CreatedBy,
		Lines: []journals.PostingLineInput{
			{AccountID: toAcct.GLAccountID, Debit: input.Amount},
			{AccountID: fromAcct.GLAccountID, Credit: input.Amount},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to post GL entry for transfer: %w", err)
	}

	journalID := journal.ID
	if _, err = s.repo.CreateBankTransaction(ctx, BankTransactionCreate{
		ID:            uuid.New(),
		BankAccountID: input.FromAccountID,
		Date:          input.Date,
		Amount:        -input.Amount,
		Description:   fmt.Sprintf("Transfer to %s: %s", toAcct.Name, input.Description),
		Reference:     input.Reference,
		Status:        "CLEARED",
		GLJournalID:   &journalID,
	}); err != nil {
		return fmt.Errorf("failed to record withdrawal: %w", err)
	}
	if _, err = s.repo.CreateBankTransaction(ctx, BankTransactionCreate{
		ID:            uuid.New(),
		BankAccountID: input.ToAccountID,
		Date:          input.Date,
		Amount:        input.Amount,
		Description:   fmt.Sprintf("Transfer from %s: %s", fromAcct.Name, input.Description),
		Reference:     input.Reference,
		Status:        "CLEARED",
		GLJournalID:   &journalID,
	}); err != nil {
		return fmt.Errorf("failed to record deposit: %w", err)
	}
	return nil
}

// ListBankAccounts returns all accounts for a company.
func (s *Service) ListBankAccounts(ctx context.Context, companyID int64) ([]BankAccount, error) {
	return s.repo.ListBankAccounts(ctx, companyID)
}

// ListBankAccountSummaries calculates each account balance from its opening balance
// plus every recorded bank transaction.
func (s *Service) ListBankAccountSummaries(ctx context.Context, companyID int64) ([]BankAccountSummary, error) {
	accounts, err := s.repo.ListBankAccounts(ctx, companyID)
	if err != nil {
		return nil, err
	}
	summaries := make([]BankAccountSummary, 0, len(accounts))
	for _, account := range accounts {
		balance := account.InitialBalance
		transactions, err := s.repo.ListBankTransactions(ctx, account.ID)
		if err != nil {
			return nil, err
		}
		for _, transaction := range transactions {
			balance += transaction.Amount
		}
		summaries = append(summaries, BankAccountSummary{Account: account, Balance: balance})
	}
	return summaries, nil
}

// ListBankTransactions returns transactions for an account.
func (s *Service) ListBankTransactions(ctx context.Context, bankAccountID int64) ([]BankTransaction, error) {
	return s.repo.ListBankTransactions(ctx, bankAccountID)
}

// ListBankTransactionSummaries returns transactions with their running balance.
func (s *Service) ListBankTransactionSummaries(ctx context.Context, account BankAccount) ([]BankTransactionSummary, float64, error) {
	transactions, err := s.repo.ListBankTransactions(ctx, account.ID)
	if err != nil {
		return nil, 0, err
	}
	balance := account.InitialBalance
	for _, transaction := range transactions {
		balance += transaction.Amount
	}
	running := balance
	summaries := make([]BankTransactionSummary, 0, len(transactions))
	for _, transaction := range transactions {
		summaries = append(summaries, BankTransactionSummary{Transaction: transaction, RunningBalance: running})
		running -= transaction.Amount
	}
	return summaries, balance, nil
}

// ResolveOpenPeriod finds the accounting period that can accept a dated bank entry.
func (s *Service) ResolveOpenPeriod(ctx context.Context, companyID int64, date time.Time) (int64, error) {
	return s.repo.FindOpenPeriod(ctx, companyID, date)
}

// ImportStatement records statement rows as pending transactions. Pending imports
// deliberately do not post GL entries until a finance user chooses a contra account.
func (s *Service) ImportStatement(ctx context.Context, account BankAccount, entries []NormalizedStatementEntry, filename string, contentHash string) (ImportResult, error) {
	result := ImportResult{}
	importRun, err := s.repo.CreateStatementImportRun(ctx, StatementImportRunCreate{
		CompanyID:     account.CompanyID,
		BankAccountID: account.ID,
		Filename:      filename,
		ContentHash:   contentHash,
	})
	if err != nil {
		return result, fmt.Errorf("failed to create import run: %w", err)
	}

	statementDate := time.Now()
	if len(entries) > 0 {
		statementDate = entries[0].Date
	}
	for _, entry := range entries {
		if entry.Date.After(statementDate) {
			statementDate = entry.Date
		}
	}
	bankStatement, err := s.repo.CreateBankStatement(ctx, BankStatementCreate{
		BankAccountID: account.ID,
		StatementDate: statementDate,
		Status:        "DRAFT",
	})
	if err != nil {
		return result, fmt.Errorf("failed to create bank statement: %w", err)
	}

	for _, entry := range entries {
		exists, err := s.repo.BankTransactionExists(ctx, account.ID, entry.Reference, entry.Fingerprint)
		if err != nil {
			return result, err
		}
		if exists {
			result.Skipped++
			continue
		}
		amount, err := exactAmountFloat(entry.Amount)
		if err != nil {
			return result, fmt.Errorf("invalid statement amount: %w", err)
		}
		_, err = s.repo.CreateBankTransaction(ctx, BankTransactionCreate{
			ID:                uuid.New(),
			BankAccountID:     account.ID,
			Date:              entry.Date,
			Amount:            amount,
			Description:       entry.Description,
			Reference:         entry.Reference,
			Status:            "PENDING",
			ImportRunID:       int64Ptr(importRun.ID),
			ExternalReference: entry.Reference,
			Fingerprint:       entry.Fingerprint,
		})
		if err != nil {
			return result, err
		}
		if err := s.repo.CreateBankStatementLine(ctx, BankStatementLineCreate{
			StatementID:     bankStatement.ID,
			Date:            entry.Date,
			Description:     entry.Description,
			Amount:          amount,
			ReferenceNumber: entry.Reference,
			Status:          "UNMATCHED",
		}); err != nil {
			return result, err
		}
		result.Imported++
	}
	return result, nil
}

func exactAmountFloat(amount automation.ExactAmount) (float64, error) {
	return strconv.ParseFloat(amount.Amount.Amount, 64)
}

func int64Ptr(value int64) *int64 { return &value }

func (s *Service) logError(message string, err error) {
	if s.logger != nil {
		s.logger.Error(message, slog.Any("error", err))
	}
}
