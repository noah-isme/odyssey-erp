package shopify_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/odyssey-erp/odyssey-erp/internal/connectors"
	"github.com/odyssey-erp/odyssey-erp/internal/connectors/providers/shopify"
)

func TestShopifyAdapter_VerifyCallbackSignature(t *testing.T) {
	secret := "shopify-app-secret"
	payload := []byte(`{"id":12345,"name":"#1001"}`)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(payload)
	signature := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	credentials := `{"shop_url":"http://shop.invalid","access_token":"token","app_secret":"shopify-app-secret"}`
	adapter := shopify.NewAdapter(slog.Default(), connectors.ProviderOptions{DevelopmentMode: true})
	conn := &connectors.Connection{SecretRef: credentials}
	if err := adapter.VerifyCallbackSignature(context.Background(), conn, map[string]string{"X-Shopify-Hmac-Sha256": signature}, payload); err != nil {
		t.Fatalf("valid signature rejected: %v", err)
	}
	if err := adapter.VerifyCallbackSignature(context.Background(), conn, map[string]string{"X-Shopify-Hmac-Sha256": "invalid_signature"}, payload); err == nil {
		t.Fatal("invalid signature accepted")
	}
	if err := adapter.VerifyCallbackSignature(context.Background(), conn, nil, payload); err == nil {
		t.Fatal("missing signature accepted")
	}
}

func TestShopifyAdapter_ValidateAndTranslate(t *testing.T) {
	adapter := shopify.NewAdapter(slog.Default(), connectors.ProviderOptions{DevelopmentMode: true})
	conn := &connectors.Connection{ID: 1, CompanyID: 100, SecretRef: `{"shop_url":"http://shop.invalid","access_token":"token","app_secret":"secret"}`}
	if err := adapter.ValidateConnection(context.Background(), conn); err != nil {
		t.Fatalf("expected valid connection, got %v", err)
	}
	if err := adapter.ValidateConnection(context.Background(), &connectors.Connection{}); err == nil {
		t.Fatal("missing secret accepted")
	}
	events, err := adapter.TranslateWebhook(context.Background(), conn, map[string]string{
		"X-Shopify-Topic":      "orders/create",
		"X-Shopify-Webhook-Id": "webhook-123",
	}, []byte(`{"id":820982911946154508,"created_at":"2026-08-09T10:00:00Z"}`))
	if err != nil || len(events) != 1 || events[0].CorrelationID != "shopify_order_820982911946154508" {
		t.Fatalf("TranslateWebhook() = %#v, %v", events, err)
	}
}

func TestShopifyAdapter_OrderContract(t *testing.T) {
	var gotMethod, gotPath, gotIdempotency string
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		gotMethod, gotPath = req.Method, req.URL.Path
		gotIdempotency = req.Header.Get("X-Odyssey-Idempotency-Key")
		return &http.Response{StatusCode: http.StatusCreated, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"order":{"id":99}}`)), Request: req}, nil
	})
	adapter := shopify.NewAdapter(slog.Default(), connectors.ProviderOptions{
		HTTPClient:                &http.Client{Transport: transport},
		AllowPlaintextCredentials: true,
	})
	credentials, _ := json.Marshal(struct {
		ShopURL     string `json:"shop_url"`
		AccessToken string `json:"access_token"`
		AppSecret   string `json:"app_secret"`
	}{ShopURL: "https://shop.sandbox.invalid", AccessToken: "token", AppSecret: "secret"})
	payload, _ := json.Marshal(shopify.OrderPayload{
		OrderID:       "ORD-123",
		TotalAmount:   "100.00",
		Currency:      "USD",
		Status:        "paid",
		CustomerEmail: "customer@example.com",
		LineItems:     []shopify.ShopifyOrderLine{{VariantID: 12, Quantity: 1, Price: "100.00"}},
	})
	err := adapter.ExecuteCommand(context.Background(), &connectors.Connection{SecretRef: string(credentials)}, &connectors.OutboxCommand{
		ID:            12,
		CommandType:   "ecommerce.order.sync",
		CorrelationID: "corr-12",
		Payload:       payload,
	})
	if err != nil {
		t.Fatalf("Shopify contract failed: %v", err)
	}
	if gotMethod != http.MethodPost || gotPath != "/admin/api/2024-04/orders.json" || gotIdempotency != "odyssey-command-12" {
		t.Fatalf("request = %s %s idempotency=%q", gotMethod, gotPath, gotIdempotency)
	}
}

func TestShopifyAdapter_ExplicitDevelopmentCommand(t *testing.T) {
	adapter := shopify.NewAdapter(slog.Default(), connectors.ProviderOptions{DevelopmentMode: true})
	err := adapter.ExecuteCommand(context.Background(), &connectors.Connection{}, &connectors.OutboxCommand{
		CommandType: "ecommerce.order.sync",
		Payload:     []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("explicit development fake failed: %v", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }
