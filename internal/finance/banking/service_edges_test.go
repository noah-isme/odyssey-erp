package banking

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/odyssey-erp/odyssey-erp/internal/sqlc"
	"github.com/stretchr/testify/require"
)

type orderedBankRepo struct {
	*mockRepo
	ordered []sqlc.BankTransaction
}

func (r *orderedBankRepo) ListBankTransactions(context.Context, int64) ([]sqlc.BankTransaction, error) {
	return r.ordered, nil
}

func TestCreateOperationsRejectInvalidInputsBeforeRepository(t *testing.T) {
	svc := NewService(newMockRepo(), nil, &mockPoster{})
	ctx := context.Background()
	if _, err := svc.CreateBankAccount(ctx, CreateAccountInput{Name: "", AccountNumber: "A"}); err == nil {
		t.Fatal("CreateBankAccount() accepted a missing name")
	}
	if _, err := svc.CreateBankTransaction(ctx, CreateTransactionInput{BankAccountID: 1, Amount: 0, ContraAccountID: 2}); err == nil {
		t.Fatal("CreateBankTransaction() accepted a zero amount")
	}
	if err := svc.TransferFunds(ctx, TransferInput{FromAccountID: 1, ToAccountID: 1, Amount: 10}); err == nil {
		t.Fatal("TransferFunds() accepted identical accounts")
	}
	if err := svc.TransferFunds(ctx, TransferInput{FromAccountID: 1, ToAccountID: 2, Amount: 0}); err == nil {
		t.Fatal("TransferFunds() accepted a non-positive amount")
	}
}

func TestBankAccountSummariesAddOpeningAndTransactionAmounts(t *testing.T) {
	repo := newMockRepo()
	initial := numericOf(1000)
	account := sqlc.BankAccount{ID: 7, CompanyID: 3, Name: "Operating", InitialBalance: initial}
	repo.accounts[account.ID] = account
	id := "txn-1"
	repo.transactions[id] = sqlc.BankTransaction{ID: pgtype.UUID{Valid: true}, BankAccountID: account.ID, Amount: numericOf(-125)}
	service := NewService(repo, nil, &mockPoster{})

	summaries, err := service.ListBankAccountSummaries(context.Background(), 3)
	require.NoError(t, err)
	require.Len(t, summaries, 1)
	require.Equal(t, 875.0, summaries[0].Balance)
}

func TestTransactionSummariesReturnFinalAndRunningBalances(t *testing.T) {
	repo := newMockRepo()
	account := sqlc.BankAccount{ID: 7, InitialBalance: numericOf(100)}
	repo.transactions["txn-1"] = sqlc.BankTransaction{ID: pgtype.UUID{Valid: true}, BankAccountID: 7, Amount: numericOf(10)}
	repo.transactions["txn-2"] = sqlc.BankTransaction{ID: pgtype.UUID{Valid: true}, BankAccountID: 7, Amount: numericOf(-20)}
	orderedRepo := &orderedBankRepo{mockRepo: repo, ordered: []sqlc.BankTransaction{repo.transactions["txn-1"], repo.transactions["txn-2"]}}
	service := NewService(orderedRepo, nil, &mockPoster{})

	summaries, balance, err := service.ListBankTransactionSummaries(context.Background(), account)
	require.NoError(t, err)
	require.Len(t, summaries, 2)
	require.Equal(t, 90.0, balance)
	require.Equal(t, 90.0, summaries[0].RunningBalance)
	require.Equal(t, 80.0, summaries[1].RunningBalance)
}
