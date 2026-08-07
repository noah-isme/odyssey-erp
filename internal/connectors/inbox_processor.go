package connectors

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/google/uuid"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/odyssey-erp/odyssey-erp/internal/outbox"
	"github.com/odyssey-erp/odyssey-erp/internal/sqlc"
)

// InboxProcessor handles incoming provider webhooks and events safely.
type InboxProcessor struct {
	queries    *sqlc.Queries
	registry   *DefaultRegistry
	outboxRepo *outbox.Repository
	logger     *slog.Logger
}

// NewInboxProcessor creates a new processor for incoming webhooks.
func NewInboxProcessor(queries *sqlc.Queries, registry *DefaultRegistry, outboxRepo *outbox.Repository, logger *slog.Logger) *InboxProcessor {
	return &InboxProcessor{
		queries:    queries,
		registry:   registry,
		outboxRepo: outboxRepo,
		logger:     logger,
	}
}

// ProcessWebhook receives a raw webhook, validates its signature, deduplicates it, and translates it to canonical events.
func (p *InboxProcessor) ProcessWebhook(ctx context.Context, connectionID int64, companyID int64, providerEventID string, payload []byte, signature string) error {
	// 1. Get the connection to identify the provider
	connRec, err := p.queries.GetConnection(ctx, sqlc.GetConnectionParams{
		ID:        connectionID,
		CompanyID: companyID,
	})
	if err != nil {
		return fmt.Errorf("connectors: connection not found: %w", err)
	}

	// 2. Get the provider adapter
	adapter, err := p.registry.GetAdapter(connRec.Provider)
	if err != nil {
		return fmt.Errorf("connectors: provider adapter not found: %w", err)
	}

	// 3. Validate signature
	if err := adapter.VerifyCallbackSignature(ctx, payload, signature); err != nil {
		return fmt.Errorf("connectors: invalid webhook signature: %w", err)
	}

	// 4. Durably store the raw event for deduplication (Inbox)
	inboxEvt, err := p.queries.InsertInboxEvent(ctx, sqlc.InsertInboxEventParams{
		CompanyID:       companyID,
		ConnectionID:    connectionID,
		ProviderEventID: providerEventID,
		RawPayload:      payload,
	})
	if err != nil {
		return fmt.Errorf("connectors: failed to insert inbox event: %w", err)
	}

	// If ID is 0, ON CONFLICT DO NOTHING triggered, meaning we've already processed this exact event ID.
	if inboxEvt.ID == 0 {
		p.logger.Info("connectors: webhook event deduplicated, ignoring", slog.String("provider_event_id", providerEventID))
		return nil
	}

	// Build domain connection object
	conn := &Connection{
		ID:        connRec.ID,
		CompanyID: connRec.CompanyID,
		Provider:  connRec.Provider,
		Type:      connRec.Type,
		Name:      connRec.Name,
		SecretRef: connRec.SecretRef,
		Status:    ConnectionStatus(connRec.Status),
	}

	// 5. Translate raw payload into CanonicalEvents
	events, err := adapter.TranslateWebhook(ctx, conn, providerEventID, payload)
	if err != nil {
		return fmt.Errorf("connectors: failed to translate webhook: %w", err)
	}

	// 6. Save the canonical events
	for _, evt := range events {
		_, err = p.queries.InsertCanonicalEvent(ctx, sqlc.InsertCanonicalEventParams{
			CompanyID:     companyID,
			ConnectionID:  connectionID,
			EventType:     evt.EventType,
			EventTime:     pgtype.Timestamptz{Time: evt.EventTime, Valid: true},
			CorrelationID: evt.CorrelationID,
			CausationID:   evt.CausationID,
			Payload:       evt.Payload,
		})
		if err != nil {
			return fmt.Errorf("connectors: failed to insert canonical event: %w", err)
		}

		if p.outboxRepo != nil {
			corrID, err := uuid.Parse(evt.CorrelationID)
			if err != nil {
				corrID = uuid.New()
			}
			var causIDPtr *uuid.UUID
			if causID, err := uuid.Parse(evt.CausationID); err == nil {
				causIDPtr = &causID
			}

			var payloadMap map[string]any
			if err := json.Unmarshal(evt.Payload, &payloadMap); err != nil {
				payloadMap = map[string]any{"raw": string(evt.Payload)}
			}

			_, err = p.outboxRepo.InsertEvent(ctx, p.queries, outbox.PublishRequest{
				CompanyID:     companyID,
				CorrelationID: corrID,
				CausationID:   causIDPtr,
				EventType:     evt.EventType,
				AggregateType: "ConnectorConnection",
				AggregateID:   connectionID,
				Payload:       payloadMap,
			})
			if err != nil {
				return fmt.Errorf("connectors: failed to route event to outbox: %w", err)
			}
		}
	}

	// 7. Mark inbox event as fully processed
	err = p.queries.MarkInboxEventProcessed(ctx, inboxEvt.ID)
	if err != nil {
		return fmt.Errorf("connectors: failed to mark inbox event as processed: %w", err)
	}

	p.logger.Info("connectors: webhook successfully translated to canonical events",
		slog.Int("event_count", len(events)),
		slog.String("provider_event_id", providerEventID),
	)

	return nil
}
