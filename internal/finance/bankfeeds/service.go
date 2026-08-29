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

// ErrSyncInProgress is returned when another worker or operator already owns
// the connection's sync lease. Callers may retry the task, but must not start
// a second provider poll for the same connection.
var ErrSyncInProgress = errors.New("bankfeeds: sync already in progress")

type bankFeedSyncLease interface {
	AcquireBankFeedSyncLease(context.Context, int64) (func(), error)
}

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
	ports   *ProviderRouter
}

func NewService(repo Repository, bankingSvc BankingService, ports map[string]FeedPort) *Service {
	return NewServiceWithProviderRouter(repo, bankingSvc, NewProviderRouter(ports))
}

// NewServiceWithProviderRouter makes provider registration an explicit
// application boundary. Passing an empty router is safe: sync and webhook
// operations return ErrProviderUnavailable until a real, credential-aware
// adapter is registered.
func NewServiceWithProviderRouter(repo Repository, bankingSvc BankingService, router *ProviderRouter) *Service {
	return &Service{
		repo:    repo,
		banking: bankingSvc,
		ports:   router,
	}
}

// SyncConnection orchestrates incremental syncing for all mapped accounts of a connection.
func (s *Service) SyncConnection(ctx context.Context, connectionID int64) error {
	if connectionID <= 0 {
		return errors.New("connection id is required")
	}
	if s == nil || s.repo == nil {
		return errors.New("bank feed repository is not configured")
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
	port, err := s.providerPort(automation.ConnectionRef{
		CompanyID:    conn.CompanyID,
		ConnectionID: conn.ID,
		Provider:     conn.ProviderID,
	})
	if err != nil {
		return fmt.Errorf("bank-feed provider %q is unavailable: %w", conn.ProviderID, err)
	}
	releaseSyncLease, err := s.acquireSyncLease(ctx, conn.ID)
	if err != nil {
		return err
	}
	defer releaseSyncLease()

	run, err := s.repo.CreateBankFeedSyncRun(ctx, conn.ID, "PENDING")
	if err != nil {
		return fmt.Errorf("failed to create sync run: %w", err)
	}
	if err := port.ValidateConnection(ctx, automation.ConnectionRef{
		CompanyID:    conn.CompanyID,
		ConnectionID: conn.ID,
		Provider:     conn.ProviderID,
	}); err != nil {
		err = fmt.Errorf("bank-feed provider %q connection validation failed: %w", conn.ProviderID, err)
		s.failRun(ctx, run.ID, err)
		return err
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
	seenCursors := map[string]struct{}{cursor: {}}

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
		if err := validateCursorProgress(seenCursors, cursor, result.NextCursor, result.HasMore); err != nil {
			return err
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
				if err := validateTransaction(t, req.Account, bankAcc.Currency); err != nil {
					return fmt.Errorf("invalid transaction for external account %s: %w", acc.ExternalAccountID, err)
				}
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

// validateCursorProgress prevents a malformed provider page from making a
// worker loop forever. Cursors are opaque, so the only safe progression check
// is that a page with more results supplies a non-empty cursor that has not
// already been observed in this sync operation.
func validateCursorProgress(seen map[string]struct{}, current, next string, hasMore bool) error {
	if !hasMore {
		return nil
	}
	if strings.TrimSpace(next) == "" {
		return errors.New("provider returned an empty cursor while more transactions are available")
	}
	if next == current {
		return errors.New("provider returned an unchanged cursor while more transactions are available")
	}
	if _, exists := seen[next]; exists {
		return errors.New("provider returned a previously seen cursor while more transactions are available")
	}
	seen[next] = struct{}{}
	return nil
}

// validateTransaction keeps a provider page inside the account and company
// scope selected by the connection mapping before it reaches the normalized
// banking importer. A malformed adapter response must fail closed rather than
// importing another account's transaction or a value in the wrong currency.
func validateTransaction(transaction Transaction, expectedAccount automation.ExternalReference, accountCurrency string) error {
	if transaction.Account != expectedAccount {
		return errors.New("transaction account does not match connection mapping")
	}
	if !isZeroTransactionReference(transaction.Reference) {
		if err := transaction.Reference.Validate(); err != nil {
			return fmt.Errorf("transaction reference: %w", err)
		}
		if transaction.Reference.Connection != expectedAccount.Connection {
			return errors.New("transaction reference is outside connection scope")
		}
	}
	if err := transaction.Amount.Validate(); err != nil {
		return fmt.Errorf("transaction amount: %w", err)
	}
	if !strings.EqualFold(strings.TrimSpace(transaction.Amount.Currency), strings.TrimSpace(accountCurrency)) {
		return errors.New("transaction currency does not match bank account")
	}
	if transaction.BookedAt.IsZero() {
		return errors.New("transaction booked date is required")
	}
	return nil
}

func isZeroTransactionReference(reference automation.ExternalReference) bool {
	return reference.Connection == (automation.ConnectionRef{}) &&
		reference.ObjectType == "" && reference.ObjectID == ""
}

func (s *Service) failRun(ctx context.Context, runID int64, err error) {
	if s == nil || s.repo == nil || err == nil {
		return
	}
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
	if s == nil || s.repo == nil {
		return BankFeedEvent{}, errors.New("bank feed repository is not configured")
	}
	conn, err := s.repo.GetBankConnection(ctx, connectionID)
	if err != nil {
		return BankFeedEvent{}, fmt.Errorf("failed to get webhook connection: %w", err)
	}
	if conn.ProviderID != provider {
		return BankFeedEvent{}, errors.New("webhook provider does not match connection")
	}
	if conn.Status != "ACTIVE" {
		return BankFeedEvent{}, errors.New("connection is not active")
	}
	if conn.ConsentExpiresAt != nil && !time.Now().Before(*conn.ConsentExpiresAt) {
		return BankFeedEvent{}, errors.New("connection consent has expired")
	}
	port, err := s.providerPort(automation.ConnectionRef{
		CompanyID:    conn.CompanyID,
		ConnectionID: conn.ID,
		Provider:     conn.ProviderID,
	})
	if err != nil {
		return BankFeedEvent{}, fmt.Errorf("bank-feed provider %q is unavailable: %w", provider, err)
	}
	if s.banking == nil {
		return BankFeedEvent{}, errors.New("banking import service is not configured")
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

func (s *Service) providerPort(ref automation.ConnectionRef) (FeedPort, error) {
	if s == nil {
		return nil, ErrProviderUnavailable
	}
	return s.ports.port(ref)
}

func (s *Service) acquireSyncLease(ctx context.Context, connectionID int64) (func(), error) {
	if s == nil || s.repo == nil {
		return nil, errors.New("bank feed repository is not configured")
	}
	if leaseRepo, ok := s.repo.(bankFeedSyncLease); ok {
		return leaseRepo.AcquireBankFeedSyncLease(ctx, connectionID)
	}
	// Small in-memory fakes and explicitly scoped integrations may not need a
	// database lease. Production repositories implement the lease so separate
	// worker processes cannot poll one connection concurrently.
	return func() {}, nil
}

// ProcessWebhookEvent converges a verified callback with the same incremental
// polling path used by scheduled syncs. Claiming and terminal status checks
// make duplicate deliveries safe; banking.ImportStatement performs the final
// transaction-level deduplication by external reference/fingerprint.
func (s *Service) ProcessWebhookEvent(ctx context.Context, eventID int64) error {
	if eventID <= 0 {
		return errors.New("event id is required")
	}
	if s == nil || s.repo == nil {
		return errors.New("bank feed repository is not configured")
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
	if s == nil || s.repo == nil {
		return err
	}
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
