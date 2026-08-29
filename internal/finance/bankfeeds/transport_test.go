package bankfeeds

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"

	"github.com/odyssey-erp/odyssey-erp/internal/finance/automation"
)

type statementTransportFake struct {
	artifact StatementArtifact
	called   int
}

func (f *statementTransportFake) FetchStatement(_ context.Context, _ automation.ConnectionRef, _ automation.ExternalReference, cursor string) (StatementArtifact, string, bool, error) {
	f.called++
	if cursor != "" {
		return StatementArtifact{Account: f.artifact.Account, Filename: f.artifact.Filename, Content: f.artifact.Content, ContentHash: f.artifact.ContentHash, SignatureVerified: true}, cursor, false, nil
	}
	return f.artifact, "done", false, nil
}

type cursorStatementTransportFake struct {
	artifact StatementArtifact
	called   int
}

func (f *cursorStatementTransportFake) FetchStatement(_ context.Context, _ automation.ConnectionRef, _ automation.ExternalReference, cursor string) (StatementArtifact, string, bool, error) {
	f.called++
	if f.called == 1 {
		return f.artifact, "page-1", true, nil
	}
	if f.called == 2 {
		return f.artifact, "page-2", true, nil
	}
	return f.artifact, "page-1", true, nil
}

func TestSyncStatementTransportUsesManualImportParserAndChecksum(t *testing.T) {
	content := []byte("date,amount,description,reference\n2026-08-12,10.25,Transport,stmt-1\n")
	hash := sha256.Sum256(content)
	connection := automation.ConnectionRef{CompanyID: 7, ConnectionID: 9, Provider: "statement"}
	account := automation.ExternalReference{Connection: connection, ObjectType: "account", ObjectID: "external-1"}
	transport := &statementTransportFake{artifact: StatementArtifact{Account: account, Filename: "statement.csv", Content: content, ContentHash: hex.EncodeToString(hash[:]), SignatureVerified: true}}
	imports := &bankingImportFake{}
	repo := &bankFeedRepoFake{connection: BankConnection{ID: 9, CompanyID: 7, ProviderID: "statement", Status: "ACTIVE"}, accounts: []BankConnectionAccount{{ID: 12, ConnectionID: 9, BankAccountID: 77, ExternalAccountID: "external-1"}}}
	provider := &transactionFeedFake{}
	service := NewService(repo, imports, map[string]FeedPort{"statement": provider})

	if err := service.SyncStatementTransport(context.Background(), 9, transport); err != nil {
		t.Fatal(err)
	}
	if transport.called != 1 || imports.calls != 1 {
		t.Fatalf("transport_calls=%d import_calls=%d", transport.called, imports.calls)
	}
	if provider.validationCalls != 1 {
		t.Fatalf("provider validation_calls=%d, want 1", provider.validationCalls)
	}
}

func TestSyncStatementTransportDerivesMissingChecksum(t *testing.T) {
	content := []byte("date,amount,description,reference\n2026-08-12,10.25,Transport,stmt-2\n")
	hash := sha256.Sum256(content)
	connection := automation.ConnectionRef{CompanyID: 7, ConnectionID: 9, Provider: "statement"}
	account := automation.ExternalReference{Connection: connection, ObjectType: "account", ObjectID: "external-1"}
	transport := &statementTransportFake{artifact: StatementArtifact{Account: account, Filename: "statement.csv", Content: content, SignatureVerified: true}}
	imports := &bankingImportFake{}
	repo := &bankFeedRepoFake{connection: BankConnection{ID: 9, CompanyID: 7, ProviderID: "statement", Status: "ACTIVE"}, accounts: []BankConnectionAccount{{ID: 12, ConnectionID: 9, BankAccountID: 77, ExternalAccountID: "external-1"}}}
	provider := &transactionFeedFake{}
	service := NewService(repo, imports, map[string]FeedPort{"statement": provider})

	if err := service.SyncStatementTransport(context.Background(), 9, transport); err != nil {
		t.Fatal(err)
	}
	if imports.hash != hex.EncodeToString(hash[:]) {
		t.Fatalf("derived hash = %q, want %q", imports.hash, hex.EncodeToString(hash[:]))
	}
}

func TestSyncStatementTransportFailsClosedBeforeCreatingRunWithoutProvider(t *testing.T) {
	content := []byte("date,amount,description,reference\n2026-08-12,10.25,Transport,stmt-3\n")
	connection := automation.ConnectionRef{CompanyID: 7, ConnectionID: 9, Provider: "statement"}
	account := automation.ExternalReference{Connection: connection, ObjectType: "account", ObjectID: "external-1"}
	transport := &statementTransportFake{artifact: StatementArtifact{
		Account: account, Filename: "statement.csv", Content: content, SignatureVerified: true,
	}}
	imports := &bankingImportFake{}
	repo := &bankFeedRepoFake{connection: BankConnection{ID: 9, CompanyID: 7, ProviderID: "statement", Status: "ACTIVE"}, accounts: []BankConnectionAccount{{ID: 12, ConnectionID: 9, BankAccountID: 77, ExternalAccountID: "external-1"}}}
	service := NewService(repo, imports, nil)

	err := service.SyncStatementTransport(context.Background(), 9, transport)
	if !errors.Is(err, ErrProviderUnavailable) {
		t.Fatalf("SyncStatementTransport() error = %v, want ErrProviderUnavailable", err)
	}
	if repo.syncRun.ID != 0 || imports.calls != 0 || transport.called != 0 {
		t.Fatalf("sync_run=%+v imports=%d transport_calls=%d, want no durable run or import", repo.syncRun, imports.calls, transport.called)
	}
}

func TestSyncStatementTransportValidatesProviderBeforeCreatingRun(t *testing.T) {
	content := []byte("date,amount,description,reference\n2026-08-12,10.25,Transport,stmt-4\n")
	connection := automation.ConnectionRef{CompanyID: 7, ConnectionID: 9, Provider: "statement"}
	account := automation.ExternalReference{Connection: connection, ObjectType: "account", ObjectID: "external-1"}
	transport := &statementTransportFake{artifact: StatementArtifact{
		Account: account, Filename: "statement.csv", Content: content, SignatureVerified: true,
	}}
	imports := &bankingImportFake{}
	repo := &bankFeedRepoFake{connection: BankConnection{ID: 9, CompanyID: 7, ProviderID: "statement", Status: "ACTIVE"}, accounts: []BankConnectionAccount{{ID: 12, ConnectionID: 9, BankAccountID: 77, ExternalAccountID: "external-1"}}}
	validationErr := errors.New("statement provider credentials unavailable")
	provider := &transactionFeedFake{validationErr: validationErr}
	service := NewService(repo, imports, map[string]FeedPort{"statement": provider})

	err := service.SyncStatementTransport(context.Background(), 9, transport)
	if !errors.Is(err, validationErr) {
		t.Fatalf("SyncStatementTransport() error = %v, want provider validation error", err)
	}
	if provider.validationCalls != 1 || repo.syncRun.ID != 0 || imports.calls != 0 || transport.called != 0 {
		t.Fatalf("validation_calls=%d sync_run=%+v imports=%d transport_calls=%d, want validation only", provider.validationCalls, repo.syncRun, imports.calls, transport.called)
	}
}

func TestSyncStatementTransportDoesNotFetchWhenAnotherSyncOwnsLease(t *testing.T) {
	content := []byte("date,amount,description,reference\n2026-08-12,10.25,Transport,stmt-5\n")
	connection := automation.ConnectionRef{CompanyID: 7, ConnectionID: 9, Provider: "statement"}
	account := automation.ExternalReference{Connection: connection, ObjectType: "account", ObjectID: "external-1"}
	transport := &statementTransportFake{artifact: StatementArtifact{
		Account: account, Filename: "statement.csv", Content: content, SignatureVerified: true,
	}}
	imports := &bankingImportFake{}
	repo := &bankFeedRepoFake{
		connection: BankConnection{ID: 9, CompanyID: 7, ProviderID: "statement", Status: "ACTIVE"},
		accounts:   []BankConnectionAccount{{ID: 12, ConnectionID: 9, BankAccountID: 77, ExternalAccountID: "external-1"}},
		leaseErr:   ErrSyncInProgress,
	}
	provider := &transactionFeedFake{}
	service := NewService(repo, imports, map[string]FeedPort{"statement": provider})

	err := service.SyncStatementTransport(context.Background(), 9, transport)
	if !errors.Is(err, ErrSyncInProgress) {
		t.Fatalf("SyncStatementTransport() error = %v, want ErrSyncInProgress", err)
	}
	if repo.leaseCalls != 1 || provider.validationCalls != 0 || transport.called != 0 || repo.syncRun.ID != 0 {
		t.Fatalf("lease_calls=%d validation_calls=%d transport_calls=%d sync_run=%+v, want lease-only", repo.leaseCalls, provider.validationCalls, transport.called, repo.syncRun)
	}
}

func TestSyncStatementTransportRejectsPreviouslySeenCursorBeforeImportingCyclePage(t *testing.T) {
	content := []byte("date,amount,description,reference\n2026-08-12,10.25,Transport,stmt-cycle\n")
	connection := automation.ConnectionRef{CompanyID: 7, ConnectionID: 9, Provider: "statement"}
	account := automation.ExternalReference{Connection: connection, ObjectType: "account", ObjectID: "external-1"}
	transport := &cursorStatementTransportFake{artifact: StatementArtifact{
		Account: account, Filename: "statement.csv", Content: content, SignatureVerified: true,
	}}
	imports := &bankingImportFake{}
	repo := &bankFeedRepoFake{
		connection: BankConnection{ID: 9, CompanyID: 7, ProviderID: "statement", Status: "ACTIVE"},
		accounts:   []BankConnectionAccount{{ID: 12, ConnectionID: 9, BankAccountID: 77, ExternalAccountID: "external-1"}},
	}
	provider := &transactionFeedFake{}
	service := NewService(repo, imports, map[string]FeedPort{"statement": provider})

	err := service.SyncStatementTransport(context.Background(), 9, transport)
	if err == nil || !strings.Contains(err.Error(), "previously seen cursor") {
		t.Fatalf("SyncStatementTransport() error = %v, want previously-seen cursor rejection", err)
	}
	if transport.called != 3 || imports.calls != 2 {
		t.Fatalf("transport_calls=%d imports=%d, want 3 and 2", transport.called, imports.calls)
	}
	if repo.syncRun.Status != "FAILED" {
		t.Fatalf("sync run = %+v, want FAILED", repo.syncRun)
	}
}

func TestValidateStatementArtifactRejectsTampering(t *testing.T) {
	ref := automation.ExternalReference{Connection: automation.ConnectionRef{CompanyID: 1, ConnectionID: 2, Provider: "p"}, ObjectType: "account", ObjectID: "a"}
	err := validateStatementArtifact(&StatementArtifact{Account: ref, Filename: "statement.csv", Content: []byte("x"), ContentHash: "bad", SignatureVerified: true}, ref)
	if err == nil {
		t.Fatal("expected checksum mismatch")
	}
}
