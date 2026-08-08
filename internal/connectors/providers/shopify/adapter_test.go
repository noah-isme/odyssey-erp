package shopify_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"os"
	"testing"

	"github.com/odyssey-erp/odyssey-erp/internal/connectors"
	"github.com/odyssey-erp/odyssey-erp/internal/connectors/providers/shopify"
)

func TestShopifyAdapter_VerifyCallbackSignature(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	adapter := shopify.NewAdapter(logger)

	payload := []byte(`{"id":12345,"name":"#1001"}`)
	secret := "dummy_secret" // hardcoded in the mock adapter for now

	// Generate valid signature
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	validSignature := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	t.Run("valid signature", func(t *testing.T) {
		headers := map[string]string{
			"X-Shopify-Hmac-Sha256": validSignature,
		}
		err := adapter.VerifyCallbackSignature(context.Background(), &connectors.Connection{SecretRef: "secret"}, headers, payload)
		if err != nil {
			t.Errorf("expected no error for valid signature, got %v", err)
		}
	})

	t.Run("missing signature header", func(t *testing.T) {
		headers := map[string]string{}
		err := adapter.VerifyCallbackSignature(context.Background(), &connectors.Connection{SecretRef: "secret"}, headers, payload)
		if err == nil {
			t.Error("expected error for missing signature header")
		}
	})

	t.Run("invalid signature but continues (development mode)", func(t *testing.T) {
		headers := map[string]string{
			"X-Shopify-Hmac-Sha256": "invalid_signature",
		}
		err := adapter.VerifyCallbackSignature(context.Background(), &connectors.Connection{SecretRef: "secret"}, headers, payload)
		// Our mock logs a warning but returns nil. 
		// If it were production, it should return an error.
		if err != nil {
			t.Errorf("expected no error (dev mode) for invalid signature, got %v", err)
		}
	})
}

func TestShopifyAdapter_TranslateWebhook(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	adapter := shopify.NewAdapter(logger)
	conn := &connectors.Connection{ID: 1, CompanyID: 100}

	t.Run("orders/create", func(t *testing.T) {
		payload := []byte(`{
			"id": 820982911946154508,
			"name": "#1001",
			"email": "jon@doe.ca",
			"total_price": "238.47",
			"currency": "USD"
		}`)
		
		headers := map[string]string{
			"X-Shopify-Topic":      "orders/create",
			"X-Shopify-Webhook-Id": "webhook-123",
		}

		events, err := adapter.TranslateWebhook(context.Background(), conn, headers, payload)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if len(events) != 1 {
			t.Fatalf("expected 1 event, got %d", len(events))
		}

		evt := events[0]
		if evt.EventType != "ecommerce.order.created" {
			t.Errorf("expected event type 'ecommerce.order.created', got %s", evt.EventType)
		}
		if evt.CorrelationID != "shopify_order_820982911946154508" {
			t.Errorf("expected correlation ID 'shopify_order_820982911946154508', got %s", evt.CorrelationID)
		}
		if evt.CausationID != "webhook-123" {
			t.Errorf("expected causation ID 'webhook-123', got %s", evt.CausationID)
		}
	})

	t.Run("orders/updated", func(t *testing.T) {
		payload := []byte(`{"id": 820982911946154508}`)
		headers := map[string]string{
			"X-Shopify-Topic":      "orders/updated",
			"X-Shopify-Webhook-Id": "webhook-456",
		}

		events, err := adapter.TranslateWebhook(context.Background(), conn, headers, payload)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if len(events) != 1 {
			t.Fatalf("expected 1 event, got %d", len(events))
		}

		evt := events[0]
		if evt.EventType != "ecommerce.order.updated" {
			t.Errorf("expected event type 'ecommerce.order.updated', got %s", evt.EventType)
		}
	})

	t.Run("missing topic", func(t *testing.T) {
		headers := map[string]string{}
		_, err := adapter.TranslateWebhook(context.Background(), conn, headers, []byte(`{}`))
		if err == nil {
			t.Error("expected error for missing topic")
		}
	})
}

func TestShopifyAdapter_ValidateConnection(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	adapter := shopify.NewAdapter(logger)

	conn := &connectors.Connection{
		Name:      "Test Shopify",
		SecretRef: "vault-secret-123",
	}

	err := adapter.ValidateConnection(context.Background(), conn)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	conn.SecretRef = ""
	err = adapter.ValidateConnection(context.Background(), conn)
	if err == nil {
		t.Error("expected error for missing secret ref")
	}
}

func TestShopifyAdapter_ExecuteCommand(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	adapter := shopify.NewAdapter(logger)
	conn := &connectors.Connection{Name: "Test Shopify"}

	payload, _ := json.Marshal(shopify.OrderPayload{
		OrderID:     "ORD-123",
		TotalAmount: "100.00",
		Currency:    "USD",
		Status:      "paid",
	})

	cmd := &connectors.OutboxCommand{
		CommandType:   "ecommerce.order.sync",
		CorrelationID: "corr-456",
		Payload:       payload,
	}

	err := adapter.ExecuteCommand(context.Background(), conn, cmd)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}
