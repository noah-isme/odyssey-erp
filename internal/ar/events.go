package ar

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/odyssey-erp/odyssey-erp/internal/outbox"
)

// RegisterOutboxHandlers connects AR logic to shared outbox events.
func RegisterOutboxHandlers(dispatcher *outbox.Dispatcher, service *Service, logger *slog.Logger) {
	handler := func(ctx context.Context, event outbox.Event) error {
		return processPaymentCaptured(ctx, service, logger, event)
	}

	dispatcher.Register("payment.captured", handler)
}

func processPaymentCaptured(ctx context.Context, service *Service, logger *slog.Logger, event outbox.Event) error {
	logger.Info("processing payment.captured event",
		slog.String("event_type", event.EventType),
		slog.String("correlation_id", event.CorrelationID.String()),
	)

	// Since the original CorrelationID (OrderID) might not be a valid UUID, the inbox processor
	// generated a random UUID for event.CorrelationID and stored the raw payload in event.Payload.
	// In the real system we need the PaymentIntent ProviderReference which matches the webhook OrderID.

	var payloadMap map[string]any
	if err := json.Unmarshal([]byte(event.Payload), &payloadMap); err != nil {
		return fmt.Errorf("failed to unmarshal payment payload: %w", err)
	}

	// Midtrans webhook contains "order_id"
	var orderID string
	if oid, ok := payloadMap["order_id"].(string); ok {
		orderID = oid
	} else {
		// Stripe webhook contains "id" or inside data.object
		return fmt.Errorf("could not extract order_id from payload")
	}

	// The orderID in Midtrans is formatted as "inv-{invoiceID}-{timestamp}"
	parts := strings.Split(orderID, "-")
	if len(parts) < 2 {
		return fmt.Errorf("invalid order_id format: %s", orderID)
	}

	invoiceID, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return fmt.Errorf("failed to parse invoice ID from order_id: %w", err)
	}

	invoice, err := service.GetARInvoiceWithDetails(ctx, invoiceID)
	if err != nil || invoice == nil {
		return fmt.Errorf("invoice not found: %d", invoiceID)
	}

	if invoice.Balance <= 0 {
		logger.Info("invoice already paid", slog.Int64("invoice_id", invoiceID))
		return nil
	}

	input := CreateARPaymentInput{
		Number:   fmt.Sprintf("PAY-ONL-%d", time.Now().Unix()),
		Currency: invoice.Currency,
		Amount:   invoice.Balance,
		PaidAt:   time.Now(),
		Method:   "online",
		Note:     fmt.Sprintf("Payment via Provider Ref: %s", orderID),
		CreatedBy: 1, // System user
		Allocations: []PaymentAllocationInput{
			{
				ARInvoiceID: invoiceID,
				Amount:      invoice.Balance,
			},
		},
	}

	payment, err := service.RegisterARPayment(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to register AR payment: %w", err)
	}

	logger.Info("successfully registered AR payment", slog.Int64("payment_id", payment.ID))

	return nil
}
