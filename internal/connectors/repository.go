package connectors

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/odyssey-erp/odyssey-erp/internal/sqlc"
)

// PGRepository maps connector persistence rows to connector domain values.
type PGRepository struct {
	queries *sqlc.Queries
}

func NewRepository(pool *pgxpool.Pool) *PGRepository {
	return &PGRepository{queries: sqlc.New(pool)}
}

func (r *PGRepository) ListConnections(ctx context.Context, companyID int64) ([]Connection, error) {
	rows, err := r.queries.ListConnections(ctx, companyID)
	if err != nil {
		return nil, err
	}
	connections := make([]Connection, len(rows))
	for i, row := range rows {
		connections[i] = mapConnection(row)
	}
	return connections, nil
}

func (r *PGRepository) CreateConnection(ctx context.Context, input ConnectionCreateInput) (Connection, error) {
	row, err := r.queries.CreateConnection(ctx, sqlc.CreateConnectionParams{
		CompanyID:   input.CompanyID,
		Provider:    input.Provider,
		Type:        input.Type,
		Name:        input.Name,
		SecretRef:   input.SecretRef,
		Status:      string(StatusDisabled),
		TokenExpiry: optionalTimestamp(input.TokenExpiry),
	})
	if err != nil {
		return Connection{}, err
	}
	return mapConnection(row), nil
}

func (r *PGRepository) GetConnection(ctx context.Context, companyID, connectionID int64) (Connection, error) {
	row, err := r.queries.GetConnection(ctx, sqlc.GetConnectionParams{ID: connectionID, CompanyID: companyID})
	if err != nil {
		return Connection{}, err
	}
	return mapConnection(row), nil
}

func (r *PGRepository) UpdateConnectionStatus(ctx context.Context, companyID, connectionID int64, status string) (Connection, error) {
	row, err := r.queries.UpdateConnectionStatus(ctx, sqlc.UpdateConnectionStatusParams{
		ID:        connectionID,
		CompanyID: companyID,
		Status:    status,
		LastError: pgtype.Text{},
	})
	if err != nil {
		return Connection{}, err
	}
	return mapConnection(row), nil
}

func (r *PGRepository) CreatePaymentIntent(ctx context.Context, input PaymentIntentInput) (int64, error) {
	row, err := r.queries.CreatePaymentIntent(ctx, sqlc.CreatePaymentIntentParams{
		CompanyID:         input.CompanyID,
		ConnectionID:      input.ConnectionID,
		SourceType:        input.SourceType,
		SourceID:          input.SourceID,
		Amount:            numericOf(input.Amount),
		Currency:          input.Currency,
		Status:            input.Status,
		ProviderReference: optionalText(input.ProviderReference),
		CheckoutUrl:       optionalText(input.CheckoutURL),
	})
	return row.ID, err
}

func (r *PGRepository) EnqueueOutboxCommand(ctx context.Context, input OutboxEnqueueInput) (int64, error) {
	row, err := r.queries.EnqueueOutboxCommand(ctx, sqlc.EnqueueOutboxCommandParams{
		CompanyID:     input.CompanyID,
		ConnectionID:  input.ConnectionID,
		CommandType:   input.CommandType,
		CorrelationID: input.CorrelationID,
		Payload:       input.Payload,
	})
	return row.ID, err
}

func (r *PGRepository) InsertInboxEvent(ctx context.Context, input InboxEventInput) (InboxEvent, error) {
	row, err := r.queries.InsertInboxEvent(ctx, sqlc.InsertInboxEventParams{
		CompanyID:       input.CompanyID,
		ConnectionID:    input.ConnectionID,
		ProviderEventID: input.ProviderEventID,
		RawPayload:      input.RawPayload,
	})
	if err != nil {
		return InboxEvent{}, err
	}
	return mapInboxEvent(row), nil
}

func (r *PGRepository) InsertCanonicalEvent(ctx context.Context, input CanonicalEventInput) (int64, error) {
	row, err := r.queries.InsertCanonicalEvent(ctx, sqlc.InsertCanonicalEventParams{
		CompanyID:     input.CompanyID,
		ConnectionID:  input.ConnectionID,
		EventType:     input.EventType,
		EventTime:     pgtype.Timestamptz{Time: input.EventTime, Valid: true},
		CorrelationID: input.CorrelationID,
		CausationID:   input.CausationID,
		Payload:       input.Payload,
	})
	return row.ID, err
}

func (r *PGRepository) MarkInboxEventProcessed(ctx context.Context, id int64) error {
	return r.queries.MarkInboxEventProcessed(ctx, id)
}

func (r *PGRepository) GetPendingOutboxCommands(ctx context.Context, limit int32) ([]OutboxCommand, error) {
	rows, err := r.queries.GetPendingOutboxCommands(ctx, limit)
	if err != nil {
		return nil, err
	}
	commands := make([]OutboxCommand, len(rows))
	for i, row := range rows {
		commands[i] = mapOutboxCommand(row)
	}
	return commands, nil
}

func (r *PGRepository) UpdateOutboxCommandState(ctx context.Context, update OutboxCommandStateUpdate) error {
	_, err := r.queries.UpdateOutboxCommandState(ctx, sqlc.UpdateOutboxCommandStateParams{
		ID:          update.ID,
		State:       update.State,
		NextAttempt: pgtype.Timestamptz{Time: update.NextAttempt, Valid: true},
	})
	return err
}

func mapConnection(row sqlc.ConnectorConnection) Connection {
	return Connection{
		ID:          row.ID,
		CompanyID:   row.CompanyID,
		Provider:    row.Provider,
		Type:        row.Type,
		Name:        row.Name,
		SecretRef:   row.SecretRef,
		Status:      ConnectionStatus(row.Status),
		LastSync:    timestampPtr(row.LastSync),
		LastError:   textPtr(row.LastError),
		TokenExpiry: timestampPtr(row.TokenExpiry),
		CreatedAt:   row.CreatedAt.Time,
		UpdatedAt:   row.UpdatedAt.Time,
	}
}

func mapInboxEvent(row sqlc.ConnectorInboxEvent) InboxEvent {
	return InboxEvent{
		ID:              row.ID,
		CompanyID:       row.CompanyID,
		ConnectionID:    row.ConnectionID,
		ProviderEventID: row.ProviderEventID,
		RawPayload:      row.RawPayload,
		Processed:       row.Processed,
		CreatedAt:       row.CreatedAt.Time,
		ProcessedAt:     timestampPtr(row.ProcessedAt),
	}
}

func mapOutboxCommand(row sqlc.ConnectorOutboxCommand) OutboxCommand {
	return OutboxCommand{
		ID:            row.ID,
		CompanyID:     row.CompanyID,
		ConnectionID:  row.ConnectionID,
		CommandType:   row.CommandType,
		CorrelationID: row.CorrelationID,
		Payload:       row.Payload,
		State:         row.State,
		Attempts:      int(row.Attempts),
		NextAttempt:   row.NextAttempt.Time,
		CreatedAt:     row.CreatedAt.Time,
		UpdatedAt:     row.UpdatedAt.Time,
	}
}

func numericOf(value float64) pgtype.Numeric {
	var numeric pgtype.Numeric
	_ = numeric.Scan(fmt.Sprintf("%.2f", value))
	return numeric
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

func optionalText(value string) pgtype.Text {
	return pgtype.Text{String: value, Valid: value != ""}
}

func textPtr(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	result := value.String
	return &result
}
