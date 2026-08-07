package stripe_test

import (
	"context"
	"testing"
	"bytes"
	"log/slog"

	"github.com/odyssey-erp/odyssey-erp/internal/connectors"
	"github.com/odyssey-erp/odyssey-erp/internal/connectors/providers/stripe"
)

func TestStripeAdapter_ValidateConnection(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), nil))
	adapter := stripe.NewAdapter(logger)

	validConn := &connectors.Connection{
		SecretRef: `{"api_key": "sk_test_123"}`,
	}
	if err := adapter.ValidateConnection(context.Background(), validConn); err != nil {
		t.Errorf("expected no error for valid connection, got %v", err)
	}

	invalidConn := &connectors.Connection{
		SecretRef: "", // empty
	}
	if err := adapter.ValidateConnection(context.Background(), invalidConn); err == nil {
		t.Errorf("expected error for missing secret reference")
	}
}

func TestStripeAdapter_CheckHealth(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), nil))
	adapter := stripe.NewAdapter(logger)

	conn := &connectors.Connection{
		SecretRef: `{"api_key": "sk_test_123"}`,
	}
	status, err := adapter.CheckHealth(context.Background(), conn)
	if err != nil {
		t.Errorf("expected no error for check health, got %v", err)
	}
	if status != connectors.StatusHealthy {
		t.Errorf("expected status healthy, got %v", status)
	}
}
