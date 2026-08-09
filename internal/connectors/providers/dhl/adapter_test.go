package dhl_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/odyssey-erp/odyssey-erp/internal/connectors"
	"github.com/odyssey-erp/odyssey-erp/internal/connectors/providers/dhl"
)

func TestAdapterLifecycleAndCommands(t *testing.T) {
	adapter := dhl.NewAdapter(nilLogger())
	ctx := context.Background()
	conn := &connectors.Connection{ID: 4, CompanyID: 9, Name: "DHL"}

	if err := adapter.ValidateConnection(ctx, conn); err == nil {
		t.Fatal("ValidateConnection() accepted missing credentials")
	}
	conn.SecretRef = "dhl-secret"
	if err := adapter.ValidateConnection(ctx, conn); err != nil {
		t.Fatal(err)
	}
	if status, err := adapter.CheckHealth(ctx, conn); err != nil || status != connectors.StatusHealthy {
		t.Fatalf("CheckHealth() = %v, %v", status, err)
	}
	if err := adapter.RefreshToken(ctx, conn); err != nil {
		t.Fatal(err)
	}
	if err := adapter.VerifyCallbackSignature(ctx, conn, nil, []byte("payload")); err != nil {
		t.Fatal(err)
	}

	payload, _ := json.Marshal(dhl.ShipmentPayload{Origin: "Jakarta", Destination: "Bandung", Weight: 2.5})
	if err := adapter.ExecuteCommand(ctx, conn, &connectors.OutboxCommand{CommandType: "shipment.book", CorrelationID: "corr-1", Payload: payload}); err != nil {
		t.Fatal(err)
	}
	if err := adapter.ExecuteCommand(ctx, conn, &connectors.OutboxCommand{CommandType: "shipment.book", Payload: []byte("not-json")}); err == nil {
		t.Fatal("ExecuteCommand() accepted malformed payload")
	}
	if err := adapter.ExecuteCommand(ctx, conn, &connectors.OutboxCommand{CommandType: "shipment.cancel"}); err == nil {
		t.Fatal("ExecuteCommand() accepted unsupported command")
	}

	events, err := adapter.TranslateWebhook(ctx, conn, map[string]string{"X-Provider-Event-Id": "evt-1"}, []byte(`{"status":"delivered"}`))
	if err != nil || len(events) != 1 || events[0].EventType != "shipment.status_updated" || events[0].CorrelationID != "evt-1" {
		t.Fatalf("TranslateWebhook() = %#v, %v", events, err)
	}
}
