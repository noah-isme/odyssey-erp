package banking

import (
	"context"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/odyssey-erp/odyssey-erp/internal/accounting/journals"
)

type mockRepo struct {
	accounts     map[int64]BankAccount
	transactions map[string]BankTransaction
	nextID       int64
}

func newMockRepo() *mockRepo {
	return &mockRepo{
		accounts:     make(map[int64]BankAccount),
		transactions: make(map[string]BankTransaction),
		nextID:       1,
	}
}

func (m *mockRepo) CreateBankAccount(_ context.Context, input BankAccountCreate) (BankAccount, error) {
	id := m.nextID
	m.nextID++
	account := BankAccount{
		ID:             id,
		CompanyID:      input.CompanyID,
		Name:           input.Name,
		AccountNumber:  input.AccountNumber,
		Currency:       input.Currency,
		GLAccountID:    input.GLAccountID,
		InitialBalance: input.InitialBalance,
		IsActive:       input.IsActive,
	}
	m.accounts[id] = account
	return account, nil
}

func (m *mockRepo) GetBankAccount(_ context.Context, id int64) (BankAccount, error) {
	account, ok := m.accounts[id]
	if !ok {
		return BankAccount{}, fmt.Errorf("not found")
	}
	return account, nil
}

func (m *mockRepo) ListBankAccounts(_ context.Context, companyID int64) ([]BankAccount, error) {
	var result []BankAccount
	for _, account := range m.accounts {
		if account.CompanyID == companyID {
			result = append(result, account)
		}
	}
	return result, nil
}

func (m *mockRepo) CreateBankTransaction(_ context.Context, input BankTransactionCreate) (BankTransaction, error) {
	transaction := BankTransaction{
		ID:                input.ID,
		BankAccountID:     input.BankAccountID,
		Date:              input.Date,
		Amount:            input.Amount,
		Description:       input.Description,
		Reference:         input.Reference,
		Status:            input.Status,
		GLJournalID:       input.GLJournalID,
		ImportRunID:       input.ImportRunID,
		ExternalReference: input.ExternalReference,
		Fingerprint:       input.Fingerprint,
		SkipReason:        input.SkipReason,
	}
	m.transactions[input.ID.String()] = transaction
	return transaction, nil
}

func (m *mockRepo) ListBankTransactions(_ context.Context, bankAccountID int64) ([]BankTransaction, error) {
	var result []BankTransaction
	for _, transaction := range m.transactions {
		if transaction.BankAccountID == bankAccountID {
			result = append(result, transaction)
		}
	}
	return result, nil
}

func (m *mockRepo) UpdateBankTransactionStatus(_ context.Context, update BankTransactionStatusUpdate) error {
	transaction, ok := m.transactions[update.ID.String()]
	if !ok {
		return fmt.Errorf("not found")
	}
	transaction.Status = update.Status
	if update.GLJournalID != nil {
		transaction.GLJournalID = update.GLJournalID
	}
	m.transactions[update.ID.String()] = transaction
	return nil
}

func (m *mockRepo) FindOpenPeriod(context.Context, int64, time.Time) (int64, error) {
	return 1, nil
}

func (m *mockRepo) BankTransactionExists(_ context.Context, bankAccountID int64, externalRef, fingerprint string) (bool, error) {
	for _, transaction := range m.transactions {
		if transaction.BankAccountID == bankAccountID && ((externalRef != "" && transaction.ExternalReference == externalRef) || (fingerprint != "" && transaction.Fingerprint == fingerprint)) {
			return true, nil
		}
	}
	return false, nil
}

func (m *mockRepo) CreateStatementImportRun(_ context.Context, input StatementImportRunCreate) (StatementImportRun, error) {
	return StatementImportRun{ID: 1, CompanyID: input.CompanyID, BankAccountID: input.BankAccountID}, nil
}

func (m *mockRepo) CreateBankStatement(_ context.Context, input BankStatementCreate) (BankStatement, error) {
	return BankStatement{ID: 1, BankAccountID: input.BankAccountID, StatementDate: input.StatementDate, Status: input.Status}, nil
}

func (m *mockRepo) CreateBankStatementLine(context.Context, BankStatementLineCreate) error {
	return nil
}

type mockPoster struct {
	nextID int64
}

func (m *mockPoster) PostJournal(context.Context, journals.PostingInput) (journals.JournalEntry, error) {
	m.nextID++
	return journals.JournalEntry{ID: m.nextID}, nil
}

func TestCreateBankAccount(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo, slog.Default(), &mockPoster{})

	account, err := svc.CreateBankAccount(context.Background(), CreateAccountInput{
		CompanyID:      1,
		Name:           "Main Bank",
		AccountNumber:  "123456",
		Currency:       "USD",
		GLAccountID:    10,
		InitialBalance: 1000,
	})
	require.NoError(t, err)
	require.Equal(t, "Main Bank", account.Name)
	require.Equal(t, int64(1), account.ID)
}

func TestCreateBankTransaction(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo, slog.Default(), &mockPoster{})
	account, err := svc.CreateBankAccount(context.Background(), CreateAccountInput{CompanyID: 1, Name: "Test", AccountNumber: "ACCT-123", GLAccountID: 10})
	require.NoError(t, err)

	transaction, err := svc.CreateBankTransaction(context.Background(), CreateTransactionInput{
		BankAccountID:   account.ID,
		Date:            time.Now(),
		Amount:          500,
		Description:     "Test deposit",
		ContraAccountID: 20,
		PeriodID:        1,
		CreatedBy:       1,
	})
	require.NoError(t, err)
	require.Equal(t, "CLEARED", transaction.Status)
	require.NotNil(t, transaction.GLJournalID)
	require.Equal(t, "CLEARED", repo.transactions[transaction.ID.String()].Status)
}

func TestReconcileTransaction(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo, slog.Default(), &mockPoster{})
	id := uuid.New()
	repo.transactions[id.String()] = BankTransaction{ID: id, Status: "CLEARED"}

	require.NoError(t, svc.ReconcileTransaction(context.Background(), id))
	require.Equal(t, "RECONCILED", repo.transactions[id.String()].Status)
}

func TestTransferFunds(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo, slog.Default(), &mockPoster{})
	account1, _ := svc.CreateBankAccount(context.Background(), CreateAccountInput{CompanyID: 1, Name: "Bank A", AccountNumber: "A1", GLAccountID: 10})
	account2, _ := svc.CreateBankAccount(context.Background(), CreateAccountInput{CompanyID: 1, Name: "Bank B", AccountNumber: "B1", GLAccountID: 20})

	err := svc.TransferFunds(context.Background(), TransferInput{
		FromAccountID: account1.ID,
		ToAccountID:   account2.ID,
		Amount:        300,
		Date:          time.Now(),
		Description:   "Inter-bank transfer",
		PeriodID:      1,
		CreatedBy:     1,
	})
	require.NoError(t, err)
	require.Len(t, repo.transactions, 2)
	for _, transaction := range repo.transactions {
		require.Equal(t, "CLEARED", transaction.Status)
	}
}
