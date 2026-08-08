package bankfeeds

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/odyssey-erp/odyssey-erp/internal/finance/automation"
	"github.com/odyssey-erp/odyssey-erp/internal/finance/banking"
	"github.com/odyssey-erp/odyssey-erp/internal/sqlc"
)

type Repository interface {
	CreateBankConnection(ctx context.Context, arg sqlc.CreateBankConnectionParams) (sqlc.BankConnection, error)
	GetBankConnection(ctx context.Context, id int64) (sqlc.BankConnection, error)
	ListBankConnections(ctx context.Context, companyID int64) ([]sqlc.BankConnection, error)
	UpdateBankConnectionStatus(ctx context.Context, arg sqlc.UpdateBankConnectionStatusParams) error

	CreateBankConnectionAccount(ctx context.Context, arg sqlc.CreateBankConnectionAccountParams) (sqlc.BankConnectionAccount, error)
	GetBankConnectionAccount(ctx context.Context, arg sqlc.GetBankConnectionAccountParams) (sqlc.BankConnectionAccount, error)
	ListBankConnectionAccounts(ctx context.Context, connectionID int64) ([]sqlc.BankConnectionAccount, error)
	UpdateBankConnectionAccountCursor(ctx context.Context, arg sqlc.UpdateBankConnectionAccountCursorParams) error

	CreateBankFeedSyncRun(ctx context.Context, arg sqlc.CreateBankFeedSyncRunParams) (sqlc.BankFeedSyncRun, error)
	UpdateBankFeedSyncRun(ctx context.Context, arg sqlc.UpdateBankFeedSyncRunParams) error

	CreateBankFeedEvent(ctx context.Context, arg sqlc.CreateBankFeedEventParams) (sqlc.BankFeedEvent, error)
	UpdateBankFeedEventStatus(ctx context.Context, arg sqlc.UpdateBankFeedEventStatusParams) error

	GetBankAccount(ctx context.Context, id int64) (sqlc.BankAccount, error)
}

type PGRepository struct {
	db      *pgxpool.Pool
	queries *sqlc.Queries
}

func NewPGRepository(db *pgxpool.Pool) *PGRepository {
	return &PGRepository{
		db:      db,
		queries: sqlc.New(db),
	}
}

// Delegate all Repository methods to sqlc.Queries
func (r *PGRepository) CreateBankConnection(ctx context.Context, arg sqlc.CreateBankConnectionParams) (sqlc.BankConnection, error) {
	return r.queries.CreateBankConnection(ctx, arg)
}
func (r *PGRepository) GetBankConnection(ctx context.Context, id int64) (sqlc.BankConnection, error) {
	return r.queries.GetBankConnection(ctx, id)
}
func (r *PGRepository) ListBankConnections(ctx context.Context, companyID int64) ([]sqlc.BankConnection, error) {
	return r.queries.ListBankConnections(ctx, companyID)
}
func (r *PGRepository) UpdateBankConnectionStatus(ctx context.Context, arg sqlc.UpdateBankConnectionStatusParams) error {
	return r.queries.UpdateBankConnectionStatus(ctx, arg)
}
func (r *PGRepository) CreateBankConnectionAccount(ctx context.Context, arg sqlc.CreateBankConnectionAccountParams) (sqlc.BankConnectionAccount, error) {
	return r.queries.CreateBankConnectionAccount(ctx, arg)
}
func (r *PGRepository) GetBankConnectionAccount(ctx context.Context, arg sqlc.GetBankConnectionAccountParams) (sqlc.BankConnectionAccount, error) {
	return r.queries.GetBankConnectionAccount(ctx, arg)
}
func (r *PGRepository) ListBankConnectionAccounts(ctx context.Context, connectionID int64) ([]sqlc.BankConnectionAccount, error) {
	return r.queries.ListBankConnectionAccounts(ctx, connectionID)
}
func (r *PGRepository) UpdateBankConnectionAccountCursor(ctx context.Context, arg sqlc.UpdateBankConnectionAccountCursorParams) error {
	return r.queries.UpdateBankConnectionAccountCursor(ctx, arg)
}
func (r *PGRepository) CreateBankFeedSyncRun(ctx context.Context, arg sqlc.CreateBankFeedSyncRunParams) (sqlc.BankFeedSyncRun, error) {
	return r.queries.CreateBankFeedSyncRun(ctx, arg)
}
func (r *PGRepository) UpdateBankFeedSyncRun(ctx context.Context, arg sqlc.UpdateBankFeedSyncRunParams) error {
	return r.queries.UpdateBankFeedSyncRun(ctx, arg)
}
func (r *PGRepository) CreateBankFeedEvent(ctx context.Context, arg sqlc.CreateBankFeedEventParams) (sqlc.BankFeedEvent, error) {
	return r.queries.CreateBankFeedEvent(ctx, arg)
}
func (r *PGRepository) UpdateBankFeedEventStatus(ctx context.Context, arg sqlc.UpdateBankFeedEventStatusParams) error {
	return r.queries.UpdateBankFeedEventStatus(ctx, arg)
}
func (r *PGRepository) GetBankAccount(ctx context.Context, id int64) (sqlc.BankAccount, error) {
	return r.queries.GetBankAccount(ctx, id)
}

type BankingService interface {
	ImportStatement(ctx context.Context, account sqlc.BankAccount, entries []banking.NormalizedStatementEntry, filename string, contentHash string) (banking.ImportResult, error)
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

	run, err := s.repo.CreateBankFeedSyncRun(ctx, sqlc.CreateBankFeedSyncRunParams{
		ConnectionID: conn.ID,
		Status:       "PENDING",
	})
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

	return s.repo.UpdateBankFeedSyncRun(ctx, sqlc.UpdateBankFeedSyncRunParams{
		ID:          run.ID,
		Status:      "COMPLETED",
		CompletedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
	})
}

func (s *Service) syncAccount(ctx context.Context, port FeedPort, runID int64, conn sqlc.BankConnection, acc sqlc.BankConnectionAccount) error {
	cursor := ""
	if acc.Cursor.Valid {
		cursor = acc.Cursor.String
	}

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
		err = s.repo.UpdateBankConnectionAccountCursor(ctx, sqlc.UpdateBankConnectionAccountCursorParams{
			ID:     acc.ID,
			Cursor: pgtype.Text{String: cursor, Valid: cursor != ""},
		})
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
	_ = s.repo.UpdateBankFeedSyncRun(ctx, sqlc.UpdateBankFeedSyncRunParams{
		ID:           runID,
		Status:       "FAILED",
		CompletedAt:  pgtype.Timestamptz{Time: time.Now(), Valid: true},
		ErrorDetails: pgtype.Text{String: err.Error(), Valid: true},
	})
}

// SaveWebhookEvent saves a webhook payload to the database for later asynchronous processing.
func (s *Service) SaveWebhookEvent(ctx context.Context, provider, eventType string, payload []byte) error {
	_, err := s.repo.CreateBankFeedEvent(ctx, sqlc.CreateBankFeedEventParams{
		ProviderID: provider,
		EventType:  eventType,
		Payload:    payload,
		OccurredAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
	})
	return err
}
