package bankfeeds

import (
	"context"
	"fmt"
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
	ProviderID   string
	EventType    string
	Payload      []byte
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
	ProviderID string
	EventType  string
	Payload    []byte
	OccurredAt time.Time
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
	conn, err := s.repo.GetBankConnection(ctx, connectionID)
	if err != nil {
		return fmt.Errorf("failed to get connection: %w", err)
	}

	if conn.Status != "ACTIVE" {
		return fmt.Errorf("connection is not active")
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
		err := s.syncAccount(ctx, port, run.ID, conn, acc)
		if err != nil {
			s.failRun(ctx, run.ID, err)
			return err
		}
	}

	completedAt := time.Now()
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

			// Map bankfeeds.Transaction to banking.NormalizedStatementEntry
			var entries []banking.NormalizedStatementEntry
			for _, t := range result.Transactions {
				entries = append(entries, banking.NormalizedStatementEntry{
					Date:        t.BookedAt,
					Amount:      t.Amount,
					Description: t.Description,
					Reference:   t.Reference.ObjectID,
					Fingerprint: "", // Let the import handle generation if empty
				})
			}

			filename := fmt.Sprintf("feed_sync_run_%d", runID)
			_, err = s.banking.ImportStatement(ctx, bankAcc, entries, filename, "")
			if err != nil {
				return fmt.Errorf("banking service failed to import statement: %w", err)
			}
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
	completedAt := time.Now()
	errorDetails := err.Error()
	_ = s.repo.UpdateBankFeedSyncRun(ctx, UpdateBankFeedSyncRunInput{ID: runID, Status: "FAILED", CompletedAt: &completedAt, ErrorDetails: &errorDetails})
}

// SaveWebhookEvent saves a webhook payload to the database for later asynchronous processing.
func (s *Service) SaveWebhookEvent(ctx context.Context, provider, eventType string, payload []byte) error {
	_, err := s.repo.CreateBankFeedEvent(ctx, CreateBankFeedEventInput{
		ProviderID: provider,
		EventType:  eventType,
		Payload:    payload,
		OccurredAt: time.Now(),
	})
	return err
}
