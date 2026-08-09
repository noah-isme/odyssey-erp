package bankfeeds

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/odyssey-erp/odyssey-erp/internal/finance/banking"
	"github.com/odyssey-erp/odyssey-erp/internal/sqlc"
)

// PGRepository maps SQLC rows and parameters to bank-feed-owned types.
type PGRepository struct {
	queries *sqlc.Queries
}

func NewPGRepository(db *pgxpool.Pool) *PGRepository {
	return &PGRepository{queries: sqlc.New(db)}
}

func (r *PGRepository) CreateBankConnection(ctx context.Context, input CreateBankConnectionInput) (BankConnection, error) {
	row, err := r.queries.CreateBankConnection(ctx, sqlc.CreateBankConnectionParams{
		CompanyID:        input.CompanyID,
		ProviderID:       input.ProviderID,
		ConnectionRef:    input.ConnectionRef,
		Status:           input.Status,
		ConsentExpiresAt: optionalTimestamp(input.ConsentExpiresAt),
		HealthStatus:     input.HealthStatus,
	})
	if err != nil {
		return BankConnection{}, err
	}
	return mapBankConnection(row), nil
}

func (r *PGRepository) GetBankConnection(ctx context.Context, id int64) (BankConnection, error) {
	row, err := r.queries.GetBankConnection(ctx, id)
	if err != nil {
		return BankConnection{}, err
	}
	return mapBankConnection(row), nil
}

func (r *PGRepository) ListBankConnections(ctx context.Context, companyID int64) ([]BankConnection, error) {
	rows, err := r.queries.ListBankConnections(ctx, companyID)
	if err != nil {
		return nil, err
	}
	items := make([]BankConnection, len(rows))
	for i, row := range rows {
		items[i] = mapBankConnection(row)
	}
	return items, nil
}

func (r *PGRepository) UpdateBankConnectionStatus(ctx context.Context, input UpdateBankConnectionStatusInput) error {
	return r.queries.UpdateBankConnectionStatus(ctx, sqlc.UpdateBankConnectionStatusParams{
		ID:           input.ID,
		Status:       input.Status,
		HealthStatus: input.HealthStatus,
		ErrorDetails: optionalText(input.ErrorDetails),
	})
}

func (r *PGRepository) CreateBankConnectionAccount(ctx context.Context, input CreateBankConnectionAccountInput) (BankConnectionAccount, error) {
	row, err := r.queries.CreateBankConnectionAccount(ctx, sqlc.CreateBankConnectionAccountParams{
		ConnectionID:      input.ConnectionID,
		BankAccountID:     input.BankAccountID,
		ExternalAccountID: input.ExternalAccountID,
	})
	if err != nil {
		return BankConnectionAccount{}, err
	}
	return mapBankConnectionAccount(row), nil
}

func (r *PGRepository) GetBankConnectionAccount(ctx context.Context, connectionID int64, externalAccountID string) (BankConnectionAccount, error) {
	row, err := r.queries.GetBankConnectionAccount(ctx, sqlc.GetBankConnectionAccountParams{
		ConnectionID:      connectionID,
		ExternalAccountID: externalAccountID,
	})
	if err != nil {
		return BankConnectionAccount{}, err
	}
	return mapBankConnectionAccount(row), nil
}

func (r *PGRepository) ListBankConnectionAccounts(ctx context.Context, connectionID int64) ([]BankConnectionAccount, error) {
	rows, err := r.queries.ListBankConnectionAccounts(ctx, connectionID)
	if err != nil {
		return nil, err
	}
	items := make([]BankConnectionAccount, len(rows))
	for i, row := range rows {
		items[i] = mapBankConnectionAccount(row)
	}
	return items, nil
}

func (r *PGRepository) UpdateBankConnectionAccountCursor(ctx context.Context, accountID int64, cursor string) error {
	return r.queries.UpdateBankConnectionAccountCursor(ctx, sqlc.UpdateBankConnectionAccountCursorParams{
		ID:     accountID,
		Cursor: optionalTextValue(cursor),
	})
}

func (r *PGRepository) CreateBankFeedSyncRun(ctx context.Context, connectionID int64, status string) (BankFeedSyncRun, error) {
	row, err := r.queries.CreateBankFeedSyncRun(ctx, sqlc.CreateBankFeedSyncRunParams{ConnectionID: connectionID, Status: status})
	if err != nil {
		return BankFeedSyncRun{}, err
	}
	return mapBankFeedSyncRun(row), nil
}

func (r *PGRepository) UpdateBankFeedSyncRun(ctx context.Context, input UpdateBankFeedSyncRunInput) error {
	return r.queries.UpdateBankFeedSyncRun(ctx, sqlc.UpdateBankFeedSyncRunParams{
		ID:           input.ID,
		Status:       input.Status,
		CompletedAt:  optionalTimestamp(input.CompletedAt),
		ErrorDetails: optionalText(input.ErrorDetails),
	})
}

func (r *PGRepository) CreateBankFeedEvent(ctx context.Context, input CreateBankFeedEventInput) (BankFeedEvent, error) {
	row, err := r.queries.CreateBankFeedEvent(ctx, sqlc.CreateBankFeedEventParams{
		ProviderID: input.ProviderID,
		EventType:  input.EventType,
		Payload:    input.Payload,
		OccurredAt: pgtype.Timestamptz{Time: input.OccurredAt, Valid: true},
	})
	if err != nil {
		return BankFeedEvent{}, err
	}
	return mapBankFeedEvent(row), nil
}

func (r *PGRepository) UpdateBankFeedEventStatus(ctx context.Context, input UpdateBankFeedEventStatusInput) error {
	return r.queries.UpdateBankFeedEventStatus(ctx, sqlc.UpdateBankFeedEventStatusParams{
		ID:           input.ID,
		Status:       input.Status,
		ErrorDetails: optionalText(input.ErrorDetails),
	})
}

func (r *PGRepository) GetBankAccount(ctx context.Context, id int64) (banking.BankAccount, error) {
	row, err := r.queries.GetBankAccount(ctx, id)
	if err != nil {
		return banking.BankAccount{}, err
	}
	amount, err := row.InitialBalance.Float64Value()
	if err != nil || !amount.Valid {
		return banking.BankAccount{}, fmt.Errorf("invalid bank account opening balance")
	}
	return banking.BankAccount{
		ID:             row.ID,
		CompanyID:      row.CompanyID,
		Name:           row.Name,
		AccountNumber:  row.AccountNumber,
		Currency:       row.Currency,
		GLAccountID:    row.GlAccountID,
		InitialBalance: amount.Float64,
		IsActive:       row.IsActive,
		CreatedAt:      row.CreatedAt.Time,
		UpdatedAt:      row.UpdatedAt.Time,
	}, nil
}

func mapBankConnection(row sqlc.BankConnection) BankConnection {
	return BankConnection{
		ID:               row.ID,
		CompanyID:        row.CompanyID,
		ProviderID:       row.ProviderID,
		ConnectionRef:    row.ConnectionRef,
		Status:           row.Status,
		ConsentExpiresAt: timestampPtr(row.ConsentExpiresAt),
		HealthStatus:     row.HealthStatus,
		ErrorDetails:     row.ErrorDetails.String,
		CreatedAt:        row.CreatedAt.Time,
		UpdatedAt:        row.UpdatedAt.Time,
	}
}

func mapBankConnectionAccount(row sqlc.BankConnectionAccount) BankConnectionAccount {
	return BankConnectionAccount{
		ID:                row.ID,
		ConnectionID:      row.ConnectionID,
		BankAccountID:     row.BankAccountID,
		ExternalAccountID: row.ExternalAccountID,
		Cursor:            row.Cursor.String,
		LastSyncedAt:      timestampPtr(row.LastSyncedAt),
		CreatedAt:         row.CreatedAt.Time,
		UpdatedAt:         row.UpdatedAt.Time,
	}
}

func mapBankFeedSyncRun(row sqlc.BankFeedSyncRun) BankFeedSyncRun {
	return BankFeedSyncRun{
		ID:           row.ID,
		ConnectionID: row.ConnectionID,
		Status:       row.Status,
		StartedAt:    row.StartedAt.Time,
		CompletedAt:  timestampPtr(row.CompletedAt),
		ErrorDetails: row.ErrorDetails.String,
	}
}

func mapBankFeedEvent(row sqlc.BankFeedEvent) BankFeedEvent {
	return BankFeedEvent{
		ID:           row.ID,
		ProviderID:   row.ProviderID,
		EventType:    row.EventType,
		Payload:      row.Payload,
		Status:       row.Status,
		ErrorDetails: row.ErrorDetails.String,
		OccurredAt:   row.OccurredAt.Time,
		CreatedAt:    row.CreatedAt.Time,
		UpdatedAt:    row.UpdatedAt.Time,
	}
}

func optionalTimestamp(value *time.Time) pgtype.Timestamptz {
	if value == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *value, Valid: true}
}

func timestampPtr(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	result := value.Time
	return &result
}

func optionalText(value *string) pgtype.Text {
	if value == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *value, Valid: true}
}

func optionalTextValue(value string) pgtype.Text {
	return pgtype.Text{String: value, Valid: value != ""}
}
