package orders

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/odyssey-erp/odyssey-erp/internal/connectors"
	"github.com/odyssey-erp/odyssey-erp/internal/connectors/providers/shopify"
	"github.com/odyssey-erp/odyssey-erp/internal/outbox"
	"github.com/odyssey-erp/odyssey-erp/internal/sqlc"
)

// MarketplaceProcessor handles CanonicalEvents produced by commerce connectors (like Shopify).
type MarketplaceProcessor struct {
	logger  *slog.Logger
	service *Service
	queries *sqlc.Queries
}

func NewMarketplaceProcessor(logger *slog.Logger, service *Service, queries *sqlc.Queries) *MarketplaceProcessor {
	return &MarketplaceProcessor{
		logger:  logger,
		service: service,
		queries: queries,
	}
}

// RegisterOutboxHandlers connects Marketplace logic to shared outbox events.
func RegisterOutboxHandlers(dispatcher *outbox.Dispatcher, processor *MarketplaceProcessor) {
	handler := func(ctx context.Context, event outbox.Event) error {
		canonical := &connectors.CanonicalEvent{
			CompanyID:     event.CompanyID,
			ConnectionID:  event.AggregateID,
			EventType:     event.EventType,
			CorrelationID: event.CorrelationID.String(),
			Payload:       []byte(event.Payload),
		}
		return processor.ProcessEvent(ctx, canonical)
	}

	dispatcher.Register("ecommerce.order.created", handler)
	dispatcher.Register("ecommerce.order.updated", handler)
}

// ProcessEvent consumes a canonical event from the integration layer.
func (p *MarketplaceProcessor) ProcessEvent(ctx context.Context, evt *connectors.CanonicalEvent) error {
	p.logger.Info("processing marketplace event",
		slog.String("event_type", evt.EventType),
		slog.String("correlation_id", evt.CorrelationID),
	)

	switch evt.EventType {
	case "ecommerce.order.created":
		return p.handleOrderCreated(ctx, evt)
	case "ecommerce.order.updated":
		return p.handleOrderUpdated(ctx, evt)
	default:
		return fmt.Errorf("marketplace_processor: unsupported event type: %s", evt.EventType)
	}
}

func (p *MarketplaceProcessor) handleOrderCreated(ctx context.Context, evt *connectors.CanonicalEvent) error {
	var shopifyOrder shopify.ShopifyOrder
	if err := json.Unmarshal(evt.Payload, &shopifyOrder); err != nil {
		return fmt.Errorf("failed to unmarshal shopify order: %w", err)
	}

	// 1. Map Customer (fallback to a dummy customer if not found)
	// For production, we'd sync customers or create them on the fly.
	// For this test, let's look up mapping, or default to customer ID 1
	customerID := int64(1)
	customerStrID := strconv.FormatInt(shopifyOrder.Customer.ID, 10)
	custMap, err := p.queries.GetObjectMappingByRemote(ctx, sqlc.GetObjectMappingByRemoteParams{
		CompanyID:        evt.CompanyID,
		ConnectionID:     evt.ConnectionID,
		RemoteEntityType: "customer",
		RemoteEntityID:   customerStrID,
	})
	if err == nil && custMap.ID != 0 {
		customerID = custMap.LocalEntityID
	}

	// 2. Map Line Items (fallback to product ID 1 if not found)
	var lines []CreateSalesOrderLineReq
	for i, item := range shopifyOrder.LineItems {
		productID := int64(1)
		variantStrID := strconv.FormatInt(item.VariantID, 10)
		prodMap, err := p.queries.GetObjectMappingByRemote(ctx, sqlc.GetObjectMappingByRemoteParams{
			CompanyID:        evt.CompanyID,
			ConnectionID:     evt.ConnectionID,
			RemoteEntityType: "product",
			RemoteEntityID:   variantStrID,
		})
		if err == nil && prodMap.ID != 0 {
			productID = prodMap.LocalEntityID
		}

		price, _ := strconv.ParseFloat(item.Price, 64)

		desc := item.Title
		lines = append(lines, CreateSalesOrderLineReq{
			ProductID:              productID,
			FulfillmentWarehouseID: 1, // Default warehouse
			Description:            &desc,
			Quantity:               float64(item.Quantity),
			UOM:                    "EA", // Assuming each
			UnitPrice:              price,
			LineOrder:              i,
		})
	}

	// 3. Create Sales Order with origin="marketplace"
	req := CreateSalesOrderRequest{
		CompanyID:  evt.CompanyID,
		CustomerID: customerID,
		OrderDate:  time.Now(),
		Currency:   shopifyOrder.Currency,
		Lines:      lines,
		Notes:      &shopifyOrder.Name,
	}

	// System user ID = 1
	order, err := p.service.Create(ctx, req, 1)
	if err != nil {
		return fmt.Errorf("failed to create sales order: %w", err)
	}

	p.logger.Info("marketplace order created in odyssey", slog.Int64("sales_order_id", order.ID))
	return nil
}

func (p *MarketplaceProcessor) handleOrderUpdated(ctx context.Context, evt *connectors.CanonicalEvent) error {
	// If the order was cancelled before fulfillment, cancel the Odyssey Sales Order.
	// If it was cancelled after fulfillment, flag for return/refund exception workflow.
	p.logger.Info("simulating marketplace order update mapping")
	return nil
}
