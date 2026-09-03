package connectors

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"

	"github.com/odyssey-erp/odyssey-erp/internal/outbox"
)

// InboxProcessor handles incoming provider webhooks and events safely.
type InboxProcessor struct {
	repo       InboxRepository
	registry   *DefaultRegistry
	outboxRepo *outbox.Repository
	logger     *slog.Logger
}

// NewInboxProcessor creates a new processor for incoming webhooks.
func NewInboxProcessor(repo InboxRepository, registry *DefaultRegistry, outboxRepo *outbox.Repository, logger *slog.Logger) *InboxProcessor {
	return &InboxProcessor{
		repo:       repo,
		registry:   registry,
		outboxRepo: outboxRepo,
		logger:     logger,
	}
}

// ProcessWebhook receives a raw webhook, validates its signature, deduplicates it, and translates it to canonical events.
func (p *InboxProcessor) ProcessWebhook(ctx context.Context, connectionID int64, companyID int64, headers map[string]string, payload []byte) error {
	// 1. Get the connection to identify the provider
	connRec, err := p.repo.GetConnection(ctx, companyID, connectionID)
	if err != nil {
		return fmt.Errorf("connectors: connection not found: %w", err)
	}

	// 2. Get the provider adapter
	adapter, err := p.registry.GetAdapter(connRec.Provider)
	if err != nil {
		return fmt.Errorf("connectors: provider adapter not found: %w", err)
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

	// 3. Validate signature
	if err := adapter.VerifyCallbackSignature(ctx, conn, headers, payload); err != nil {
		return fmt.Errorf("connectors: invalid webhook signature: %w", err)
	}

	providerEventID := webhookEventID(connRec.Provider, headers, payload)

	// 4. Durably store the raw event for deduplication (Inbox)
	inboxEvt, err := p.repo.InsertInboxEvent(ctx, InboxEventInput{
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

	// 5. Translate raw payload into CanonicalEvents
	events, err := adapter.TranslateWebhook(ctx, conn, headers, payload)
	if err != nil {
		return fmt.Errorf("connectors: failed to translate webhook: %w", err)
	}

	// 6. Save the canonical events
	for _, evt := range events {
		_, err = p.repo.InsertCanonicalEvent(ctx, CanonicalEventInput{
			CompanyID:     companyID,
			ConnectionID:  connectionID,
			EventType:     evt.EventType,
			EventTime:     evt.EventTime,
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

			_, err = p.outboxRepo.InsertEvent(ctx, outbox.PublishRequest{
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
	err = p.repo.MarkInboxEventProcessed(ctx, inboxEvt.ID)
	if err != nil {
		return fmt.Errorf("connectors: failed to mark inbox event as processed: %w", err)
	}

	p.logger.Info("connectors: webhook successfully translated to canonical events",
		slog.Int("event_count", len(events)),
		slog.String("provider_event_id", providerEventID),
	)

	return nil
}

// webhookEventID returns a provider event identifier without generating a
// random value. Random fallbacks made identical webhook replays look like new
// events and defeated the inbox uniqueness constraint.
func webhookEventID(provider string, headers map[string]string, payload []byte) string {
	if value := Header(headers, "X-Provider-Event-Id", "X-Shopify-Webhook-Id", "X-DHL-Event-Id", "X-Message-Reference", "X-WhatsApp-Event-Id", "Stripe-Event-Id"); value != "" {
		return value
	}
	if provider == "stripe" {
		var event struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(payload, &event) == nil && strings.TrimSpace(event.ID) != "" {
			return event.ID
		}
	}
	return ProviderPayloadID(payload)
}
