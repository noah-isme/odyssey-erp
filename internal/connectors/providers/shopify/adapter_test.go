package shopify_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"testing"

	"github.com/odyssey-erp/odyssey-erp/internal/connectors"
	"github.com/odyssey-erp/odyssey-erp/internal/connectors/providers/shopify"
)

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
