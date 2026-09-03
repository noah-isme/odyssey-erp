package banking

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type orderedBankRepo struct {
	*mockRepo
	ordered []BankTransaction
}

func (r *orderedBankRepo) ListBankTransactions(context.Context, int64) ([]BankTransaction, error) {
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
	account := BankAccount{ID: 7, CompanyID: 3, Name: "Operating", InitialBalance: 1000}
	repo.accounts[account.ID] = account
	id := newMockTransactionID()
	repo.transactions[id.String()] = BankTransaction{ID: id, BankAccountID: account.ID, Amount: -125}
	service := NewService(repo, nil, &mockPoster{})

	summaries, err := service.ListBankAccountSummaries(context.Background(), 3)
	require.NoError(t, err)
	require.Len(t, summaries, 1)
	require.Equal(t, 875.0, summaries[0].Balance)
}

func TestTransactionSummariesReturnFinalAndRunningBalances(t *testing.T) {
	repo := newMockRepo()
	account := BankAccount{ID: 7, InitialBalance: 100}
	first, second := newMockTransactionID(), newMockTransactionID()
	repo.transactions[first.String()] = BankTransaction{ID: first, BankAccountID: 7, Amount: 10}
	repo.transactions[second.String()] = BankTransaction{ID: second, BankAccountID: 7, Amount: -20}
	orderedRepo := &orderedBankRepo{mockRepo: repo, ordered: []BankTransaction{repo.transactions[first.String()], repo.transactions[second.String()]}}
	service := NewService(orderedRepo, nil, &mockPoster{})

	summaries, balance, err := service.ListBankTransactionSummaries(context.Background(), account)
	require.NoError(t, err)
	require.Len(t, summaries, 2)
	require.Equal(t, 90.0, balance)
	require.Equal(t, 90.0, summaries[0].RunningBalance)
	require.Equal(t, 80.0, summaries[1].RunningBalance)
}

func newMockTransactionID() uuid.UUID { return uuid.New() }
