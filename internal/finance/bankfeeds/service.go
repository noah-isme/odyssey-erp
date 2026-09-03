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

// BankConnection is the database-neutral representation of an external bank feed.
type BankConnection struct {
	ID               int64
	CompanyID        int64
	ProviderID       string
	ConnectionRef    string
	Status           string
	ConsentExpiresAt *time.Time
	HealthStatus     string
	ErrorDetails     string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type BankConnectionAccount struct {
	ID                int64
	ConnectionID      int64
	BankAccountID     int64
	ExternalAccountID string
	Cursor            string
	LastSyncedAt      *time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type BankFeedSyncRun struct {
	ID           int64
	ConnectionID int64
	Status       string
	StartedAt    time.Time
	CompletedAt  *time.Time
	ErrorDetails string
}

type BankFeedEvent struct {
	ID           int64
	ConnectionID int64
	ProviderID   string
	EventType    string
	Payload      []byte
	PayloadHash  string
	Status       string
	ErrorDetails string
	OccurredAt   time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type CreateBankConnectionInput struct {
	CompanyID        int64
	ProviderID       string
	ConnectionRef    string
	Status           string
	ConsentExpiresAt *time.Time
	HealthStatus     string
}

type UpdateBankConnectionStatusInput struct {
	ID           int64
	Status       string
	HealthStatus string
	ErrorDetails *string
}

type CreateBankConnectionAccountInput struct {
	ConnectionID      int64
	BankAccountID     int64
	ExternalAccountID string
}

type UpdateBankFeedSyncRunInput struct {
	ID           int64
	Status       string
	CompletedAt  *time.Time
	ErrorDetails *string
}

type CreateBankFeedEventInput struct {
	ConnectionID int64
	ProviderID   string
	EventType    string
	Payload      []byte
	PayloadHash  string
	OccurredAt   time.Time
}

type UpdateBankFeedEventStatusInput struct {
	ID           int64
	Status       string
	ErrorDetails *string
}

type Repository interface {
	CreateBankConnection(ctx context.Context, input CreateBankConnectionInput) (BankConnection, error)
	GetBankConnection(ctx context.Context, id int64) (BankConnection, error)
	ListBankConnections(ctx context.Context, companyID int64) ([]BankConnection, error)
	UpdateBankConnectionStatus(ctx context.Context, input UpdateBankConnectionStatusInput) error

	CreateBankConnectionAccount(ctx context.Context, input CreateBankConnectionAccountInput) (BankConnectionAccount, error)
	GetBankConnectionAccount(ctx context.Context, connectionID int64, externalAccountID string) (BankConnectionAccount, error)
	ListBankConnectionAccounts(ctx context.Context, connectionID int64) ([]BankConnectionAccount, error)
	UpdateBankConnectionAccountCursor(ctx context.Context, accountID int64, cursor string) error

	CreateBankFeedSyncRun(ctx context.Context, connectionID int64, status string) (BankFeedSyncRun, error)
	UpdateBankFeedSyncRun(ctx context.Context, input UpdateBankFeedSyncRunInput) error

	CreateBankFeedEvent(ctx context.Context, input CreateBankFeedEventInput) (BankFeedEvent, error)
	GetBankFeedEvent(ctx context.Context, id int64) (BankFeedEvent, error)
	ClaimBankFeedEvent(ctx context.Context, id int64) (bool, error)
	UpdateBankFeedEventStatus(ctx context.Context, input UpdateBankFeedEventStatusInput) error

	GetBankAccount(ctx context.Context, id int64) (banking.BankAccount, error)
}

type BankingService interface {
	ImportStatement(ctx context.Context, account banking.BankAccount, entries []banking.NormalizedStatementEntry, filename string, contentHash string) (banking.ImportResult, error)
}

type Service struct {
	repo    Repository
	banking BankingService
	ports   map[string]FeedPort
}

func NewService(repo Repository, bankingSvc BankingService, ports map[string]FeedPort) *Service {
	return &Service{
		repo:    repo,
		banking: bankingSvc,
		ports:   ports,
	}
}

// SyncConnection orchestrates incremental syncing for all mapped accounts of a connection.
func (s *Service) SyncConnection(ctx context.Context, connectionID int64) error {
	if connectionID <= 0 {
		return errors.New("connection id is required")
	}
	conn, err := s.repo.GetBankConnection(ctx, connectionID)
	if err != nil {
		return fmt.Errorf("failed to get connection: %w", err)
	}

	if conn.Status != "ACTIVE" {
		return fmt.Errorf("connection is not active")
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

	port, ok := s.ports[conn.ProviderID]
	if !ok {
		return fmt.Errorf("unsupported provider: %s", conn.ProviderID)
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

	for _, acc := range accounts {
		if acc.ConnectionID != conn.ID || acc.ID <= 0 || acc.BankAccountID <= 0 || strings.TrimSpace(acc.ExternalAccountID) == "" {
			err := fmt.Errorf("invalid account mapping for connection %d", conn.ID)
			s.failRun(ctx, run.ID, err)
			return err
		}
		err := s.syncAccount(ctx, port, run.ID, conn, acc)
		if err != nil {
			s.failRun(ctx, run.ID, err)
			return err
		}
	}

	completedAt := time.Now().UTC()
	return s.repo.UpdateBankFeedSyncRun(ctx, UpdateBankFeedSyncRunInput{ID: run.ID, Status: "COMPLETED", CompletedAt: &completedAt})
}

func (s *Service) syncAccount(ctx context.Context, port FeedPort, runID int64, conn BankConnection, acc BankConnectionAccount) error {
	cursor := acc.Cursor

	for {
		req := SyncRequest{
			Connection: automation.ConnectionRef{
				CompanyID:    conn.CompanyID,
				ConnectionID: conn.ID,
				Provider:     conn.ProviderID,
			},
			Account: automation.ExternalReference{
				Connection: automation.ConnectionRef{
					CompanyID:    conn.CompanyID,
					ConnectionID: conn.ID,
					Provider:     conn.ProviderID,
				},
				ObjectType: "account",
				ObjectID:   acc.ExternalAccountID,
			},
			Cursor: cursor,
		}

		result, err := port.Transactions(ctx, req)
		if err != nil {
			return fmt.Errorf("failed to sync external account %s: %w", acc.ExternalAccountID, err)
		}

		if len(result.Transactions) > 0 {
			bankAcc, err := s.repo.GetBankAccount(ctx, acc.BankAccountID)
			if err != nil {
				return fmt.Errorf("failed to get internal bank account: %w", err)
			}
			if bankAcc.CompanyID != conn.CompanyID {
				return fmt.Errorf("mapped bank account %d does not belong to connection company", bankAcc.ID)
			}

			// Map bankfeeds.Transaction to banking.NormalizedStatementEntry
			var entries []banking.NormalizedStatementEntry
			for _, t := range result.Transactions {
				fingerprint := transactionFingerprint(t, acc.ExternalAccountID)
				entries = append(entries, banking.NormalizedStatementEntry{
					Date:        t.BookedAt,
					Amount:      t.Amount,
					Description: t.Description,
					Reference:   t.Reference.ObjectID,
					Fingerprint: fingerprint,
				})
			}

			filename := fmt.Sprintf("feed_sync_run_%d", runID)
			_, err = s.banking.ImportStatement(ctx, bankAcc, entries, filename, "")
			if err != nil {
				return fmt.Errorf("banking service failed to import statement: %w", err)
			}
		}

		if result.HasMore && result.NextCursor == cursor {
			return fmt.Errorf("provider returned an unchanged cursor while more transactions are available")
		}
		cursor = result.NextCursor
		err = s.repo.UpdateBankConnectionAccountCursor(ctx, acc.ID, cursor)
		if err != nil {
			return fmt.Errorf("failed to update cursor: %w", err)
		}

		if !result.HasMore {
			break
		}
	}
	return nil
}

func (s *Service) failRun(ctx context.Context, runID int64, err error) {
	completedAt := time.Now().UTC()
	errorDetails := err.Error()
	_ = s.repo.UpdateBankFeedSyncRun(ctx, UpdateBankFeedSyncRunInput{ID: runID, Status: "FAILED", CompletedAt: &completedAt, ErrorDetails: &errorDetails})
}

// SaveWebhookEvent verifies and saves a webhook payload for later asynchronous
// processing. The connection ID is the tenant boundary; provider-only events
// are rejected because a provider name is not enough to identify a company.
func (s *Service) SaveWebhookEvent(ctx context.Context, connectionID int64, provider, eventType string, headers map[string]string, payload []byte) (BankFeedEvent, error) {
	if connectionID <= 0 || strings.TrimSpace(provider) == "" || len(payload) == 0 {
		return BankFeedEvent{}, errors.New("connection, provider, and payload are required")
	}
	conn, err := s.repo.GetBankConnection(ctx, connectionID)
	if err != nil {
		return BankFeedEvent{}, fmt.Errorf("failed to get webhook connection: %w", err)
	}
	if conn.ProviderID != provider {
		return BankFeedEvent{}, errors.New("webhook provider does not match connection")
	}
	port, ok := s.ports[provider]
	if !ok {
		return BankFeedEvent{}, fmt.Errorf("unsupported provider: %s", provider)
	}
	verifier, ok := port.(WebhookVerifier)
	if !ok {
		return BankFeedEvent{}, errors.New("provider does not support verified webhooks")
	}

	inbound, err := verifier.VerifyWebhook(ctx, automation.ConnectionRef{
		CompanyID:    conn.CompanyID,
		ConnectionID: conn.ID,
		Provider:     conn.ProviderID,
	}, headers, payload)
	if err != nil {
		return BankFeedEvent{}, fmt.Errorf("webhook verification failed: %w", err)
	}
	if inbound.EventType != "" {
		eventType = inbound.EventType
	}
	if strings.TrimSpace(eventType) == "" {
		eventType = "unknown"
	}
	occurredAt := inbound.OccurredAt
	if occurredAt.IsZero() {
		occurredAt = time.Now().UTC()
	}
	payloadHash := inbound.PayloadHash
	if payloadHash == "" {
		hash := sha256.Sum256(payload)
		payloadHash = hex.EncodeToString(hash[:])
	}

	return s.repo.CreateBankFeedEvent(ctx, CreateBankFeedEventInput{
		ConnectionID: connectionID,
		ProviderID:   provider,
		EventType:    eventType,
		Payload:      payload,
		PayloadHash:  payloadHash,
		OccurredAt:   occurredAt,
	})
}

// ProcessWebhookEvent converges a verified callback with the same incremental
// polling path used by scheduled syncs. Claiming and terminal status checks
// make duplicate deliveries safe; banking.ImportStatement performs the final
// transaction-level deduplication by external reference/fingerprint.
func (s *Service) ProcessWebhookEvent(ctx context.Context, eventID int64) error {
	if eventID <= 0 {
		return errors.New("event id is required")
	}
	event, err := s.repo.GetBankFeedEvent(ctx, eventID)
	if err != nil {
		return fmt.Errorf("failed to get bank feed event: %w", err)
	}
	if event.Status == "PROCESSED" {
		return nil
	}
	claimed, err := s.repo.ClaimBankFeedEvent(ctx, eventID)
	if err != nil {
		return fmt.Errorf("failed to claim bank feed event: %w", err)
	}
	if !claimed {
		latest, getErr := s.repo.GetBankFeedEvent(ctx, eventID)
		if getErr == nil && latest.Status == "PROCESSED" {
			return nil
		}
		return errors.New("bank feed event is already being processed")
	}
	event, err = s.repo.GetBankFeedEvent(ctx, eventID)
	if err != nil {
		return fmt.Errorf("failed to reload claimed bank feed event: %w", err)
	}
	conn, err := s.repo.GetBankConnection(ctx, event.ConnectionID)
	if err != nil {
		return s.failEvent(ctx, eventID, fmt.Errorf("failed to get event connection: %w", err))
	}
	if conn.ProviderID != event.ProviderID {
		return s.failEvent(ctx, eventID, errors.New("event provider does not match connection"))
	}
	if err := s.SyncConnection(ctx, conn.ID); err != nil {
		return s.failEvent(ctx, eventID, fmt.Errorf("failed to converge webhook with bank sync: %w", err))
	}
	if err := s.repo.UpdateBankFeedEventStatus(ctx, UpdateBankFeedEventStatusInput{ID: eventID, Status: "PROCESSED"}); err != nil {
		return fmt.Errorf("failed to mark bank feed event processed: %w", err)
	}
	return nil
}

func (s *Service) failEvent(ctx context.Context, eventID int64, err error) error {
	details := err.Error()
	_ = s.repo.UpdateBankFeedEventStatus(ctx, UpdateBankFeedEventStatusInput{ID: eventID, Status: "FAILED", ErrorDetails: &details})
	return err
}

func transactionFingerprint(transaction Transaction, externalAccountID string) string {
	if transaction.Reference.ObjectID != "" {
		return transaction.Reference.ObjectID
	}
	hash := sha256.New()
	_, _ = fmt.Fprintf(hash, "%s|%s|%s|%s|%s|%s", externalAccountID, transaction.BookedAt.UTC().Format(time.RFC3339Nano), transaction.ValueDate.UTC().Format(time.DateOnly), transaction.Amount.Amount.String(), transaction.Description, transaction.CounterpartyReference)
	return hex.EncodeToString(hash.Sum(nil))
}
