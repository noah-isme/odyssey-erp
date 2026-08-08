package banking

import (
	"context"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/require"

	"github.com/odyssey-erp/odyssey-erp/internal/accounting/journals"
	"github.com/odyssey-erp/odyssey-erp/internal/sqlc"
)

type mockRepo struct {
	accounts     map[int64]sqlc.BankAccount
	transactions map[string]sqlc.BankTransaction
	nextID       int64
}

func newMockRepo() *mockRepo {
	return &mockRepo{
		accounts:     make(map[int64]sqlc.BankAccount),
		transactions: make(map[string]sqlc.BankTransaction),
		nextID:       1,
	}
}

func (m *mockRepo) CreateBankAccount(ctx context.Context, arg sqlc.CreateBankAccountParams) (sqlc.BankAccount, error) {
	id := m.nextID
	m.nextID++
	acct := sqlc.BankAccount{
		ID:             id,
		CompanyID:      arg.CompanyID,
		Name:           arg.Name,
		AccountNumber:  arg.AccountNumber,
		Currency:       arg.Currency,
		GlAccountID:    arg.GlAccountID,
		InitialBalance: arg.InitialBalance,
		IsActive:       arg.IsActive,
	}
	m.accounts[id] = acct
	return acct, nil
}

func (m *mockRepo) GetBankAccount(ctx context.Context, id int64) (sqlc.BankAccount, error) {
	acct, ok := m.accounts[id]
	if !ok {
		return sqlc.BankAccount{}, fmt.Errorf("not found")
	}
	return acct, nil
}

func (m *mockRepo) ListBankAccounts(ctx context.Context, companyID int64) ([]sqlc.BankAccount, error) {
	var out []sqlc.BankAccount
	for _, a := range m.accounts {
		if a.CompanyID == companyID {
			out = append(out, a)
		}
	}
	return out, nil
}

func (m *mockRepo) UpdateBankAccount(ctx context.Context, arg sqlc.UpdateBankAccountParams) error {
	return nil
}

func (m *mockRepo) CreateBankTransaction(ctx context.Context, arg sqlc.CreateBankTransactionParams) (sqlc.BankTransaction, error) {
	id := uuid.UUID(arg.ID.Bytes).String()
	txn := sqlc.BankTransaction{
		ID:            arg.ID,
		BankAccountID: arg.BankAccountID,
		Date:          arg.Date,
		Amount:        arg.Amount,
		Description:   arg.Description,
		Reference:     arg.Reference,
		Status:        arg.Status,
	}
	m.transactions[id] = txn
	return txn, nil
}

func (m *mockRepo) GetBankTransaction(ctx context.Context, id pgtype.UUID) (sqlc.BankTransaction, error) {
	uid := uuid.UUID(id.Bytes).String()
	txn, ok := m.transactions[uid]
	if !ok {
		return sqlc.BankTransaction{}, fmt.Errorf("not found")
	}
	return txn, nil
}

func (m *mockRepo) ListBankTransactions(ctx context.Context, bankAccountID int64) ([]sqlc.BankTransaction, error) {
	var out []sqlc.BankTransaction
	for _, t := range m.transactions {
		if t.BankAccountID == bankAccountID {
			out = append(out, t)
		}
	}
	return out, nil
}

func (m *mockRepo) UpdateBankTransactionStatus(ctx context.Context, arg sqlc.UpdateBankTransactionStatusParams) error {
	uid := uuid.UUID(arg.ID.Bytes).String()
	txn, ok := m.transactions[uid]
	if !ok {
		return fmt.Errorf("not found")
	}
	txn.Status = arg.Status
	if arg.GlJournalID.Valid {
		txn.GlJournalID = arg.GlJournalID
	}
	m.transactions[uid] = txn
	return nil
}

func (m *mockRepo) FindOpenPeriod(context.Context, int64, time.Time) (int64, error) {
	return 1, nil
}

func (m *mockRepo) BankTransactionExists(_ context.Context, bankAccountID int64, externalRef string, fingerprint string) (bool, error) {
	for _, transaction := range m.transactions {
		if transaction.BankAccountID == bankAccountID {
			if externalRef != "" && transaction.ExternalReference.String == externalRef {
				return true, nil
			}
			if fingerprint != "" && transaction.Fingerprint.String == fingerprint {
				return true, nil
			}
		}
	}
	return false, nil
}

func (m *mockRepo) CreateStatementImportRun(ctx context.Context, arg sqlc.CreateStatementImportRunParams) (sqlc.StatementImportRun, error) {
	return sqlc.StatementImportRun{ID: 1}, nil
}

func (m *mockRepo) CreateBankStatement(ctx context.Context, arg sqlc.CreateBankStatementParams) (sqlc.BankStatement, error) {
	return sqlc.BankStatement{ID: 1}, nil
}

func (m *mockRepo) CreateBankStatementLine(ctx context.Context, arg sqlc.CreateBankStatementLineParams) (sqlc.BankStatementLine, error) {
	return sqlc.BankStatementLine{ID: 1}, nil
}

type mockPoster struct {
	nextID int64
}

func (m *mockPoster) PostJournal(ctx context.Context, input journals.PostingInput) (journals.JournalEntry, error) {
	m.nextID++
	return journals.JournalEntry{
		ID: m.nextID,
	}, nil
}

func TestCreateBankAccount(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo, slog.Default(), &mockPoster{})

	input := CreateAccountInput{
		CompanyID:      1,
		Name:           "Main Bank",
		AccountNumber:  "123456",
		Currency:       "USD",
		GLAccountID:    10,
		InitialBalance: 1000,
	}

	acct, err := svc.CreateBankAccount(context.Background(), input)
	require.NoError(t, err)
	require.Equal(t, "Main Bank", acct.Name)
	require.Equal(t, int64(1), acct.ID)
}

func TestCreateBankTransaction(t *testing.T) {
	repo := newMockRepo()
	poster := &mockPoster{}
	svc := NewService(repo, slog.Default(), poster)

	// Setup account
	acct, err := svc.CreateBankAccount(context.Background(), CreateAccountInput{
		CompanyID:     1,
		Name:          "Test",
		AccountNumber: "ACCT-123",
		GLAccountID:   10,
	})
	require.NoError(t, err)

	input := CreateTransactionInput{
		BankAccountID:   acct.ID,
		Date:            time.Now(),
		Amount:          500.0,
		Description:     "Test deposit",
		ContraAccountID: 20,
		PeriodID:        1,
		CreatedBy:       1,
	}

	txn, err := svc.CreateBankTransaction(context.Background(), input)
	require.NoError(t, err)
	require.Equal(t, "CLEARED", txn.Status)
	require.True(t, txn.GlJournalID.Valid)

	// Verify repo state
	uid := uuid.UUID(txn.ID.Bytes).String()
	require.Equal(t, "CLEARED", repo.transactions[uid].Status)
}

func TestReconcileTransaction(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo, slog.Default(), &mockPoster{})

	id := uuid.New()
	repo.transactions[id.String()] = sqlc.BankTransaction{
		ID:     pgtype.UUID{Bytes: id, Valid: true},
		Status: "CLEARED",
	}

	err := svc.ReconcileTransaction(context.Background(), id)
	require.NoError(t, err)
	require.Equal(t, "RECONCILED", repo.transactions[id.String()].Status)
}

func TestTransferFunds(t *testing.T) {
	repo := newMockRepo()
	poster := &mockPoster{}
	svc := NewService(repo, slog.Default(), poster)

	acct1, _ := svc.CreateBankAccount(context.Background(), CreateAccountInput{
		CompanyID:     1,
		Name:          "Bank A",
		AccountNumber: "A1",
		GLAccountID:   10,
	})
	acct2, _ := svc.CreateBankAccount(context.Background(), CreateAccountInput{
		CompanyID:     1,
		Name:          "Bank B",
		AccountNumber: "B1",
		GLAccountID:   20,
	})

	input := TransferInput{
		FromAccountID: acct1.ID,
		ToAccountID:   acct2.ID,
		Amount:        300.0,
		Date:          time.Now(),
		Description:   "Inter-bank transfer",
		PeriodID:      1,
		CreatedBy:     1,
	}

	err := svc.TransferFunds(context.Background(), input)
	require.NoError(t, err)

	// Verify repo state - should have 2 transactions
	require.Len(t, repo.transactions, 2)

	var withdrawal, deposit sqlc.BankTransaction
	for _, t := range repo.transactions {
		if t.BankAccountID == acct1.ID {
			withdrawal = t
		} else if t.BankAccountID == acct2.ID {
			deposit = t
		}
	}

	require.Equal(t, acct1.ID, withdrawal.BankAccountID)
	// Amount is stored as Numeric, we can check by converting back or just seeing if it's there
	// In our mock we store it as provided in arg.Amount

	require.Equal(t, acct2.ID, deposit.BankAccountID)
	require.Equal(t, "CLEARED", withdrawal.Status)
	require.Equal(t, "CLEARED", deposit.Status)
}
