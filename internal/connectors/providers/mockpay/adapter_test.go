package mockpay_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"

	"github.com/odyssey-erp/odyssey-erp/internal/connectors"
	"github.com/odyssey-erp/odyssey-erp/internal/connectors/providers/mockpay"
)

func TestAdapterValidatesAndTranslatesPaymentCommands(t *testing.T) {
	adapter := mockpay.NewAdapter(slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx := context.Background()
	conn := &connectors.Connection{ID: 5, CompanyID: 8, Name: "Mock payments"}
	if err := adapter.ValidateConnection(ctx, conn); err == nil {
		t.Fatal("ValidateConnection() accepted missing secret")
	}
	conn.SecretRef = "secret-ref"
	if err := adapter.ValidateConnection(ctx, conn); err != nil {
		t.Fatal(err)
	}
	if status, err := adapter.CheckHealth(ctx, conn); err != nil || status != connectors.StatusHealthy {
		t.Fatalf("CheckHealth() = %v, %v", status, err)
	}
	if err := adapter.RefreshToken(ctx, conn); err != nil {
		t.Fatal(err)
	}
	if err := adapter.VerifyCallbackSignature(ctx, conn, nil, nil); err == nil {
		t.Fatal("VerifyCallbackSignature() accepted missing signature")
	}
	if err := adapter.VerifyCallbackSignature(ctx, conn, map[string]string{"X-Provider-Signature": "sig"}, nil); err != nil {
		t.Fatal(err)
	}

	payload, _ := json.Marshal(mockpay.PaymentChargePayload{Amount: 25, Currency: "USD", Source: "card-1"})
	if err := adapter.ExecuteCommand(ctx, conn, &connectors.OutboxCommand{CommandType: "payment.charge", Payload: payload}); err != nil {
		t.Fatal(err)
	}
	if err := adapter.ExecuteCommand(ctx, conn, &connectors.OutboxCommand{CommandType: "payment.charge", Payload: []byte("bad")}); err == nil {
		t.Fatal("ExecuteCommand() accepted malformed payload")
	}
	if err := adapter.ExecuteCommand(ctx, conn, &connectors.OutboxCommand{CommandType: "payment.refund"}); err == nil {
		t.Fatal("ExecuteCommand() accepted unsupported command")
	}

	events, err := adapter.TranslateWebhook(ctx, conn, map[string]string{"X-Provider-Event-Id": "evt-9"}, []byte(`{"amount":25}`))
	if err != nil || len(events) != 1 || events[0].EventType != "payment.captured" || events[0].CorrelationID != "evt-9" {
		t.Fatalf("TranslateWebhook() = %#v, %v", events, err)
	}
}
