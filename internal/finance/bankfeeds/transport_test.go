package bankfeeds

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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

func TestSyncStatementTransportUsesManualImportParserAndChecksum(t *testing.T) {
	content := []byte("date,amount,description,reference\n2026-08-12,10.25,Transport,stmt-1\n")
	hash := sha256.Sum256(content)
	connection := automation.ConnectionRef{CompanyID: 7, ConnectionID: 9, Provider: "statement"}
	account := automation.ExternalReference{Connection: connection, ObjectType: "account", ObjectID: "external-1"}
	transport := &statementTransportFake{artifact: StatementArtifact{Account: account, Filename: "statement.csv", Content: content, ContentHash: hex.EncodeToString(hash[:]), SignatureVerified: true}}
	imports := &bankingImportFake{}
	repo := &bankFeedRepoFake{connection: BankConnection{ID: 9, CompanyID: 7, ProviderID: "statement", Status: "ACTIVE"}, accounts: []BankConnectionAccount{{ID: 12, ConnectionID: 9, BankAccountID: 77, ExternalAccountID: "external-1"}}}
	service := NewService(repo, imports, nil)

	if err := service.SyncStatementTransport(context.Background(), 9, transport); err != nil {
		t.Fatal(err)
	}
	if transport.called != 1 || imports.calls != 1 {
		t.Fatalf("transport_calls=%d import_calls=%d", transport.called, imports.calls)
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
	service := NewService(repo, imports, nil)

	if err := service.SyncStatementTransport(context.Background(), 9, transport); err != nil {
		t.Fatal(err)
	}
	if imports.hash != hex.EncodeToString(hash[:]) {
		t.Fatalf("derived hash = %q, want %q", imports.hash, hex.EncodeToString(hash[:]))
	}
}

func TestValidateStatementArtifactRejectsTampering(t *testing.T) {
	ref := automation.ExternalReference{Connection: automation.ConnectionRef{CompanyID: 1, ConnectionID: 2, Provider: "p"}, ObjectType: "account", ObjectID: "a"}
	err := validateStatementArtifact(&StatementArtifact{Account: ref, Filename: "statement.csv", Content: []byte("x"), ContentHash: "bad", SignatureVerified: true}, ref)
	if err == nil {
		t.Fatal("expected checksum mismatch")
	}
}
