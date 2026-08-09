package connectors

import (
	"context"
	"testing"
)

type registryAdapter struct{}

func (registryAdapter) ValidateConnection(context.Context, *Connection) error { return nil }
func (registryAdapter) CheckHealth(context.Context, *Connection) (ConnectionStatus, error) {
	return StatusHealthy, nil
}
func (registryAdapter) RefreshToken(context.Context, *Connection) error { return nil }
func (registryAdapter) VerifyCallbackSignature(context.Context, *Connection, map[string]string, []byte) error {
	return nil
}
func (registryAdapter) ExecuteCommand(context.Context, *Connection, *OutboxCommand) error { return nil }
func (registryAdapter) TranslateWebhook(context.Context, *Connection, map[string]string, []byte) ([]*CanonicalEvent, error) {
	return nil, nil
}

func TestRegistryRegistersAndFindsAdapters(t *testing.T) {
	registry := NewRegistry()
	adapter := registryAdapter{}
	registry.Register("test", adapter)
	got, err := registry.GetAdapter("test")
	if err != nil || got == nil {
		t.Fatalf("GetAdapter() = %v, %v", got, err)
	}
	if _, err := registry.GetAdapter("missing"); err == nil {
		t.Fatal("GetAdapter() returned an adapter for an unknown provider")
	}
}
