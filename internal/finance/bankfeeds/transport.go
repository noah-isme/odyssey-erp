package bankfeeds

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/odyssey-erp/odyssey-erp/internal/finance/automation"
	"github.com/odyssey-erp/odyssey-erp/internal/finance/banking"
)

// StatementArtifact is a verified statement file returned by a bank transport.
// The payload is consumed immediately by the normalized import path; callers
// must keep credentials and provider secrets out of this value.
type StatementArtifact struct {
	Account           automation.ExternalReference
	Filename          string
	Content           []byte
	ContentHash       string
	SourceID          string
	RetrievedAt       time.Time
	SignatureVerified bool
}

// StatementTransport retrieves signed/encrypted CSV or OFX artifacts. Cursor
// advancement is committed only after the artifact imports successfully.
type StatementTransport interface {
	FetchStatement(context.Context, automation.ConnectionRef, automation.ExternalReference, string) (StatementArtifact, string, bool, error)
}

// SyncStatementTransport consumes statement artifacts through the same
// deduplicating banking import service used by manual CSV/OFX uploads.
func (s *Service) SyncStatementTransport(ctx context.Context, connectionID int64, transport StatementTransport) error {
	if connectionID <= 0 || transport == nil {
		return errors.New("connection and statement transport are required")
	}
	conn, err := s.repo.GetBankConnection(ctx, connectionID)
	if err != nil {
		return fmt.Errorf("failed to get connection: %w", err)
	}
	if conn.Status != "ACTIVE" {
		return errors.New("connection is not active")
	}
	if conn.CompanyID <= 0 {
		return errors.New("connection company is required")
	}
	if conn.ConsentExpiresAt != nil && !time.Now().Before(*conn.ConsentExpiresAt) {
		return errors.New("connection consent has expired")
	}
	if s.banking == nil {
		return errors.New("banking import service is not configured")
	}
	run, err := s.repo.CreateBankFeedSyncRun(ctx, conn.ID, "PENDING")
	if err != nil {
		return fmt.Errorf("failed to create sync run: %w", err)
	}
	accounts, err := s.repo.ListBankConnectionAccounts(ctx, conn.ID)
	if err != nil {
		s.failRun(ctx, run.ID, err)
		return err
	}
	connectionRef := automation.ConnectionRef{CompanyID: conn.CompanyID, ConnectionID: conn.ID, Provider: conn.ProviderID}
	for _, mapping := range accounts {
		if mapping.ConnectionID != conn.ID || mapping.ID <= 0 || mapping.BankAccountID <= 0 || strings.TrimSpace(mapping.ExternalAccountID) == "" {
			err := fmt.Errorf("invalid account mapping for connection %d", conn.ID)
			s.failRun(ctx, run.ID, err)
			return err
		}
		bankAccount, err := s.repo.GetBankAccount(ctx, mapping.BankAccountID)
		if err != nil {
			err = fmt.Errorf("failed to get internal bank account: %w", err)
			s.failRun(ctx, run.ID, err)
			return err
		}
		if bankAccount.CompanyID != conn.CompanyID {
			err = fmt.Errorf("mapped bank account %d does not belong to connection company", bankAccount.ID)
			s.failRun(ctx, run.ID, err)
			return err
		}
		accountRef := automation.ExternalReference{Connection: connectionRef, ObjectType: "account", ObjectID: mapping.ExternalAccountID}
		cursor := mapping.Cursor
		for {
			artifact, nextCursor, hasMore, fetchErr := transport.FetchStatement(ctx, connectionRef, accountRef, cursor)
			if fetchErr != nil {
				err = fmt.Errorf("failed to fetch statement for account %s: %w", mapping.ExternalAccountID, fetchErr)
				s.failRun(ctx, run.ID, err)
				return err
			}
			if err := validateStatementArtifact(&artifact, accountRef); err != nil {
				s.failRun(ctx, run.ID, err)
				return err
			}
			entries, err := banking.ParseStatement(artifact.Filename, artifact.Content, bankAccount.Currency, bankAccount.ID)
			if err != nil {
				err = fmt.Errorf("failed to parse statement artifact %s: %w", artifact.Filename, err)
				s.failRun(ctx, run.ID, err)
				return err
			}
			if _, err := s.banking.ImportStatement(ctx, bankAccount, entries, artifact.Filename, artifact.ContentHash); err != nil {
				err = fmt.Errorf("banking service failed to import statement artifact: %w", err)
				s.failRun(ctx, run.ID, err)
				return err
			}
			if hasMore && nextCursor == cursor {
				err := errors.New("statement transport returned an unchanged cursor while more artifacts are available")
				s.failRun(ctx, run.ID, err)
				return err
			}
			if err := s.repo.UpdateBankConnectionAccountCursor(ctx, mapping.ID, nextCursor); err != nil {
				err = fmt.Errorf("failed to update statement cursor: %w", err)
				s.failRun(ctx, run.ID, err)
				return err
			}
			cursor = nextCursor
			if !hasMore {
				break
			}
		}
	}
	completedAt := time.Now().UTC()
	return s.repo.UpdateBankFeedSyncRun(ctx, UpdateBankFeedSyncRunInput{ID: run.ID, Status: "COMPLETED", CompletedAt: &completedAt})
}

func validateStatementArtifact(artifact *StatementArtifact, expected automation.ExternalReference) error {
	if artifact == nil {
		return errors.New("statement artifact is required")
	}
	if artifact.Account != expected {
		return errors.New("statement artifact account does not match connection mapping")
	}
	if strings.TrimSpace(artifact.Filename) == "" || len(artifact.Content) == 0 || !artifact.SignatureVerified {
		return errors.New("statement artifact must contain a verified non-empty file")
	}
	hash := sha256.Sum256(artifact.Content)
	computed := hex.EncodeToString(hash[:])
	if strings.TrimSpace(artifact.ContentHash) == "" {
		artifact.ContentHash = computed
	}
	if !strings.EqualFold(artifact.ContentHash, computed) {
		return errors.New("statement artifact checksum mismatch")
	}
	return nil
}
