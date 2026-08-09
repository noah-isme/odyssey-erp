package bankfeeds

import (
	"context"
	"testing"
	"time"

	"github.com/odyssey-erp/odyssey-erp/internal/finance/automation"
	"github.com/odyssey-erp/odyssey-erp/internal/finance/banking"
)

type bankFeedRepoFake struct {
	connection BankConnection
	event      BankFeedEvent
	accounts   []BankConnectionAccount
	claimed    bool
	imports    int
}

func (r *bankFeedRepoFake) CreateBankConnection(context.Context, CreateBankConnectionInput) (BankConnection, error) {
	return r.connection, nil
}
func (r *bankFeedRepoFake) GetBankConnection(context.Context, int64) (BankConnection, error) {
	return r.connection, nil
}
func (r *bankFeedRepoFake) ListBankConnections(context.Context, int64) ([]BankConnection, error) {
	return []BankConnection{r.connection}, nil
}
func (r *bankFeedRepoFake) UpdateBankConnectionStatus(context.Context, UpdateBankConnectionStatusInput) error {
	return nil
}
func (r *bankFeedRepoFake) CreateBankConnectionAccount(context.Context, CreateBankConnectionAccountInput) (BankConnectionAccount, error) {
	return r.accounts[0], nil
}
func (r *bankFeedRepoFake) GetBankConnectionAccount(context.Context, int64, string) (BankConnectionAccount, error) {
	return r.accounts[0], nil
}
func (r *bankFeedRepoFake) ListBankConnectionAccounts(context.Context, int64) ([]BankConnectionAccount, error) {
	return r.accounts, nil
}
func (r *bankFeedRepoFake) UpdateBankConnectionAccountCursor(context.Context, int64, string) error {
	return nil
}
func (r *bankFeedRepoFake) CreateBankFeedSyncRun(context.Context, int64, string) (BankFeedSyncRun, error) {
	return BankFeedSyncRun{ID: 31}, nil
}
func (r *bankFeedRepoFake) UpdateBankFeedSyncRun(context.Context, UpdateBankFeedSyncRunInput) error {
	return nil
}
func (r *bankFeedRepoFake) CreateBankFeedEvent(_ context.Context, input CreateBankFeedEventInput) (BankFeedEvent, error) {
	r.event = BankFeedEvent{ID: 51, ConnectionID: input.ConnectionID, ProviderID: input.ProviderID, Status: "PENDING"}
	return r.event, nil
}
func (r *bankFeedRepoFake) GetBankFeedEvent(context.Context, int64) (BankFeedEvent, error) {
	return r.event, nil
}
func (r *bankFeedRepoFake) ClaimBankFeedEvent(context.Context, int64) (bool, error) {
	if r.claimed || r.event.Status == "PROCESSED" {
		return false, nil
	}
	r.claimed = true
	r.event.Status = "PROCESSING"
	return true, nil
}
func (r *bankFeedRepoFake) UpdateBankFeedEventStatus(_ context.Context, input UpdateBankFeedEventStatusInput) error {
	r.event.Status = input.Status
	if input.ErrorDetails != nil {
		r.event.ErrorDetails = *input.ErrorDetails
	}
	return nil
}
func (r *bankFeedRepoFake) GetBankAccount(context.Context, int64) (banking.BankAccount, error) {
	return banking.BankAccount{ID: 77, CompanyID: 7, Currency: "USD"}, nil
}

type transactionFeedFake struct{ calls int }

func (f *transactionFeedFake) ValidateConnection(context.Context, automation.ConnectionRef) error {
	return nil
}
func (f *transactionFeedFake) ListAccounts(context.Context, automation.ConnectionRef) ([]Account, error) {
	return nil, nil
}
func (f *transactionFeedFake) Balances(context.Context, automation.ConnectionRef, []automation.ExternalReference) ([]Balance, error) {
	return nil, nil
}
func (f *transactionFeedFake) Transactions(context.Context, SyncRequest) (TransactionPage, error) {
	f.calls++
	return TransactionPage{Transactions: []Transaction{{
		Reference:   automation.ExternalReference{ObjectID: "txn-1"},
		Amount:      automation.MustParseExact("10"),
		BookedAt:    time.Date(2026, 8, 9, 0, 0, 0, 0, time.UTC),
		Description: "bank payment",
	}}}, nil
}

type bankingImportFake struct{ calls int }

func (f *bankingImportFake) ImportStatement(context.Context, banking.BankAccount, []banking.NormalizedStatementEntry, string, string) (banking.ImportResult, error) {
	f.calls++
	return banking.ImportResult{Imported: 1}, nil
}

func TestProcessWebhookEventConvergesThroughIdempotentSync(t *testing.T) {
	feed := &transactionFeedFake{}
	bankingService := &bankingImportFake{}
	repo := &bankFeedRepoFake{
		connection: BankConnection{ID: 9, CompanyID: 7, ProviderID: "fake", Status: "ACTIVE"},
		event:      BankFeedEvent{ID: 51, ConnectionID: 9, ProviderID: "fake", Status: "PENDING"},
		accounts:   []BankConnectionAccount{{ID: 12, ConnectionID: 9, BankAccountID: 77, ExternalAccountID: "external-1"}},
	}
	service := NewService(repo, bankingService, map[string]FeedPort{"fake": feed})

	if err := service.ProcessWebhookEvent(context.Background(), 51); err != nil {
		t.Fatal(err)
	}
	if repo.event.Status != "PROCESSED" || feed.calls != 1 || bankingService.calls != 1 {
		t.Fatalf("event=%+v feed_calls=%d import_calls=%d", repo.event, feed.calls, bankingService.calls)
	}
	if err := service.ProcessWebhookEvent(context.Background(), 51); err != nil {
		t.Fatal(err)
	}
	if feed.calls != 1 || bankingService.calls != 1 {
		t.Fatalf("processed event was re-synced: feed_calls=%d import_calls=%d", feed.calls, bankingService.calls)
	}
}
