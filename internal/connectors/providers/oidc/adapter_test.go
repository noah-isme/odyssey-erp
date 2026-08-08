package oidc_test

import (
	"context"
	"testing"
	"bytes"
	"log/slog"

	"github.com/odyssey-erp/odyssey-erp/internal/connectors"
	"github.com/odyssey-erp/odyssey-erp/internal/connectors/providers/oidc"
)

func TestOIDCAdapter_ValidateConnection(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), nil))
	adapter := oidc.NewAdapter(logger)

	validConn := &connectors.Connection{
		SecretRef: `{"issuer": "https://accounts.google.com"}`,
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

func TestOIDCAdapter_ExecuteCommand(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), nil))
	adapter := oidc.NewAdapter(logger)

	conn := &connectors.Connection{}
	
	validCmd := &connectors.OutboxCommand{
		CommandType: "auth.user.provision",
	}
	if err := adapter.ExecuteCommand(context.Background(), conn, validCmd); err != nil {
		t.Errorf("expected no error for auth.user.provision, got %v", err)
	}

	invalidCmd := &connectors.OutboxCommand{
		CommandType: "unknown.command",
	}
	if err := adapter.ExecuteCommand(context.Background(), conn, invalidCmd); err == nil {
		t.Errorf("expected error for unknown command")
	}
}

func TestOIDCAdapter_TranslateWebhook(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), nil))
	adapter := oidc.NewAdapter(logger)

	conn := &connectors.Connection{CompanyID: 1, ID: 10}
	
	validPayload := []byte("logout_token=eyJhbGciOiJSUzI1...")
	events, err := adapter.TranslateWebhook(context.Background(), conn, map[string]string{"X-Provider-Event-Id": "evt_123"}, validPayload)
	if err != nil {
		t.Errorf("expected no error for valid payload, got %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].EventType != "auth.user.logged_out" {
		t.Errorf("expected event type auth.user.logged_out, got %v", events[0].EventType)
	}

	invalidPayload := []byte("some_other_data=123")
	_, err = adapter.TranslateWebhook(context.Background(), conn, map[string]string{"X-Provider-Event-Id": "evt_123"}, invalidPayload)
	if err == nil {
		t.Errorf("expected error for payload missing logout_token")
	}
}
