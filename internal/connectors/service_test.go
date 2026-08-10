package connectors_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/odyssey-erp/odyssey-erp/internal/connectors"
)

func TestCreateCheckoutIntentPersistsBeforeProviderCallAndUsesIDRUnits(t *testing.T) {
	repo := &checkoutRepo{connection: connectors.Connection{ID: 7, CompanyID: 42, Provider: "cert", Type: "payment", Status: connectors.StatusHealthy}}
	adapter := &checkoutAdapter{}
	registry := connectors.NewRegistry()
	registry.Register("cert", adapter)
	service := connectors.NewService(repo, nil, registry)

	result, err := service.CreateCheckoutIntent(context.Background(), connectors.CreateCheckoutIntentRequest{
		CompanyID:     42,
		ConnectionID:  7,
		SourceType:    "ar_invoice",
		SourceID:      99,
		Amount:        1_500_000,
		Currency:      "IDR",
		CustomerName:  "Sandbox Buyer",
		CustomerEmail: "buyer@example.com",
		OrderID:       "inv-99-cert",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.PaymentIntentID != 101 || result.Token == "" || repo.input.Status != string(connectors.PaymentStatusCreated) {
		t.Fatalf("result=%#v input=%#v", result, repo.input)
	}
	if repo.updatedStatus != string(connectors.PaymentStatusPending) || repo.updatedURL == "" {
		t.Fatalf("checkout update status=%q url=%q", repo.updatedStatus, repo.updatedURL)
	}
	if adapter.grossAmount != 1_500_000 {
		t.Fatalf("provider gross amount=%d, want 1500000 IDR units", adapter.grossAmount)
	}
}

func TestCreateCheckoutIntentLeavesCreatedIntentForTimeoutRecovery(t *testing.T) {
	repo := &checkoutRepo{connection: connectors.Connection{ID: 7, CompanyID: 42, Provider: "cert", Type: "payment", Status: connectors.StatusHealthy}}
	adapter := &checkoutAdapter{err: errors.New("provider response timed out")}
	registry := connectors.NewRegistry()
	registry.Register("cert", adapter)
	service := connectors.NewService(repo, nil, registry)

	result, err := service.CreateCheckoutIntent(context.Background(), connectors.CreateCheckoutIntentRequest{
		CompanyID: 42, ConnectionID: 7, SourceType: "ar_invoice", SourceID: 99,
		Amount: 75000, Currency: "IDR", OrderID: "inv-99-timeout",
	})
	if err == nil || result.PaymentIntentID != 101 {
		t.Fatalf("timeout result=%#v err=%v", result, err)
	}
	if repo.input.Status != string(connectors.PaymentStatusCreated) || repo.updatedStatus != "" {
		t.Fatalf("timeout intent was not left recoverable: input=%#v updated=%q", repo.input, repo.updatedStatus)
	}
}

type checkoutRepo struct {
	connection    connectors.Connection
	input         connectors.PaymentIntentInput
	updatedStatus string
	updatedURL    string
}

func (r *checkoutRepo) ListConnections(context.Context, int64) ([]connectors.Connection, error) {
	return []connectors.Connection{r.connection}, nil
}
func (r *checkoutRepo) CreateConnection(context.Context, connectors.ConnectionCreateInput) (connectors.Connection, error) {
	return r.connection, nil
}
func (r *checkoutRepo) GetConnection(context.Context, int64, int64) (connectors.Connection, error) {
	return r.connection, nil
}
func (r *checkoutRepo) UpdateConnectionStatus(context.Context, int64, int64, string) (connectors.Connection, error) {
	return r.connection, nil
}
func (r *checkoutRepo) CreatePaymentIntent(_ context.Context, input connectors.PaymentIntentInput) (int64, error) {
	r.input = input
	return 101, nil
}
func (r *checkoutRepo) EnqueueOutboxCommand(context.Context, connectors.OutboxEnqueueInput) (int64, error) {
	return 1, nil
}
func (r *checkoutRepo) UpdatePaymentIntentCheckout(_ context.Context, _ int64, _ int64, _ int64, status, _ string, url string) error {
	r.updatedStatus = status
	r.updatedURL = url
	return nil
}
func (r *checkoutRepo) GetPaymentIntentByProviderReference(context.Context, int64, int64, string) (connectors.PaymentIntent, error) {
	return connectors.PaymentIntent{}, nil
}
func (r *checkoutRepo) ApplyPaymentIntentEvent(context.Context, connectors.PaymentIntentEventInput) (connectors.PaymentTransitionResult, error) {
	return connectors.PaymentTransitionResult{}, nil
}

type checkoutAdapter struct {
	err         error
	grossAmount int64
}

func (a *checkoutAdapter) ValidateConnection(context.Context, *connectors.Connection) error {
	return nil
}
func (a *checkoutAdapter) CheckHealth(context.Context, *connectors.Connection) (connectors.ConnectionStatus, error) {
	return connectors.StatusHealthy, nil
}
func (a *checkoutAdapter) RefreshToken(context.Context, *connectors.Connection) error { return nil }
func (a *checkoutAdapter) VerifyCallbackSignature(context.Context, *connectors.Connection, map[string]string, []byte) error {
	return nil
}
func (a *checkoutAdapter) ExecuteCommand(_ context.Context, _ *connectors.Connection, cmd *connectors.OutboxCommand) error {
	if a.err != nil {
		return a.err
	}
	var payload struct {
		GrossAmount int64 `json:"gross_amount"`
	}
	if err := json.Unmarshal(cmd.Payload, &payload); err != nil {
		return err
	}
	a.grossAmount = payload.GrossAmount
	cmd.Payload = []byte(`{"token":"token-cert","redirect_url":"https://sandbox.invalid/cert"}`)
	return nil
}
func (a *checkoutAdapter) TranslateWebhook(context.Context, *connectors.Connection, map[string]string, []byte) ([]*connectors.CanonicalEvent, error) {
	return nil, nil
}

var _ connectors.Repository = (*checkoutRepo)(nil)
var _ connectors.ProviderAdapter = (*checkoutAdapter)(nil)
