package connectors_test

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/odyssey-erp/odyssey-erp/internal/connectors"
	"github.com/odyssey-erp/odyssey-erp/internal/connectors/providers/mockpay"
)

type MockRegistry struct {
	adapter connectors.ProviderAdapter
}

func (m *MockRegistry) GetAdapter(provider string) (connectors.ProviderAdapter, error) {
	return m.adapter, nil
}

func TestOutboxWorkerMockpay(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	adapter := mockpay.NewAdapter(logger)

	// Create the registry
	registry := connectors.NewRegistry()
	registry.Register("mockpay", adapter)

	// Verify it registers
	_, err := registry.GetAdapter("mockpay")
	if err != nil {
		t.Fatalf("expected to find mockpay adapter, got error: %v", err)
	}

	// Just verifying the adapter interface conforms
	var _ connectors.ProviderAdapter = adapter

	// Let's test the adapter directly with a fake payload
	ctx := context.Background()
	conn := &connectors.Connection{
		ID:        1,
		CompanyID: 1,
		Provider:  "mockpay",
		Type:      "payment",
		Name:      "Test Pay",
		SecretRef: "vault:secret-123",
		Status:    connectors.StatusHealthy,
	}

	err = adapter.ValidateConnection(ctx, conn)
	if err != nil {
		t.Errorf("expected validation to pass, got %v", err)
	}

	cmd := &connectors.OutboxCommand{
		ID:            10,
		CompanyID:     1,
		ConnectionID:  1,
		CommandType:   "payment.charge",
		CorrelationID: "txn-999",
		Payload:       []byte(`{"amount": 150.50, "currency": "IDR", "source": "tok_123"}`),
		State:         "pending",
		Attempts:      0,
		NextAttempt:   time.Now(),
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	err = adapter.ExecuteCommand(ctx, conn, cmd)
	if err != nil {
		t.Errorf("expected ExecuteCommand to pass for payment.charge, got %v", err)
	}

	cmdInvalid := &connectors.OutboxCommand{
		ID:          11,
		CommandType: "unknown.action",
	}

	err = adapter.ExecuteCommand(ctx, conn, cmdInvalid)
	if err == nil {
		t.Errorf("expected ExecuteCommand to fail for unknown action, but it passed")
	}
}
