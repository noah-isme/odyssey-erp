package midtrans_test

import (
	"bytes"
	"context"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/odyssey-erp/odyssey-erp/internal/connectors"
	"github.com/odyssey-erp/odyssey-erp/internal/connectors/providers/midtrans"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
)

func newTestVault(t *testing.T) *shared.Vault {
	t.Helper()
	t.Setenv("APP_MASTER_KEY", "test-master-key-for-unit-tests-only")
	v, err := shared.NewVault()
	if err != nil {
		t.Fatalf("newTestVault: %v", err)
	}
	return v
}

func encryptedConn(t *testing.T, vault *shared.Vault, serverKey string) *connectors.Connection {
	t.Helper()
	conn := &connectors.Connection{
		ID:        1,
		CompanyID: 42,
		Provider:  "midtrans",
		Type:      "payment",
		Name:      "Test Midtrans",
	}
	if err := conn.SetCredentials(vault, serverKey); err != nil {
		t.Fatalf("encryptedConn: %v", err)
	}
	return conn
}

func plaintextConnection(t *testing.T, credentials midtrans.Credentials) *connectors.Connection {
	t.Helper()
	payload, err := json.Marshal(credentials)
	if err != nil {
		t.Fatalf("marshal credentials: %v", err)
	}
	return &connectors.Connection{ID: 1, CompanyID: 42, SecretRef: string(payload)}
}

func midtransSignature(orderID, statusCode, grossAmount, serverKey string) string {
	raw := orderID + statusCode + grossAmount + serverKey
	sum := sha512.Sum512([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func silentLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), nil))
}

func response(status int, body string, req *http.Request) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    req,
	}
}

// VerifyCallbackSignature ---------------------------------------------------

func TestMidtransAdapter_VerifyCallbackSignature(t *testing.T) {
	vault := newTestVault(t)
	const serverKey = "SB-Mid-server-test-key"
	conn := encryptedConn(t, vault, serverKey)
	adapter := midtrans.NewAdapter(silentLogger(), vault)

	payload, _ := json.Marshal(map[string]string{
		"order_id":           "inv-100-1700000000",
		"status_code":        "200",
		"gross_amount":       "150000.00",
		"transaction_status": "settlement",
		"signature_key":      midtransSignature("inv-100-1700000000", "200", "150000.00", serverKey),
	})
	if err := adapter.VerifyCallbackSignature(context.Background(), conn, nil, payload); err != nil {
		t.Fatalf("valid signature rejected: %v", err)
	}

	for name, raw := range map[string][]byte{
		"wrong signature": []byte(`{"order_id":"inv-100-1700000000","status_code":"200","gross_amount":"150000.00","signature_key":"deadbeef"}`),
		"malformed":       []byte("not-json"),
		"missing fields":  []byte(`{"order_id":"inv-100-1700000000","signature_key":"deadbeef"}`),
	} {
		t.Run(name, func(t *testing.T) {
			if err := adapter.VerifyCallbackSignature(context.Background(), conn, nil, raw); err == nil {
				t.Fatal("invalid callback accepted")
			}
		})
	}
}

func TestMidtransAdapter_VerifyCallbackSignature_WrongKey(t *testing.T) {
	vault := newTestVault(t)
	conn := encryptedConn(t, vault, "wrong-server-key")
	adapter := midtrans.NewAdapter(silentLogger(), vault)
	payload, _ := json.Marshal(map[string]string{
		"order_id":      "inv-100-1700000000",
		"status_code":   "200",
		"gross_amount":  "150000.00",
		"signature_key": midtransSignature("inv-100-1700000000", "200", "150000.00", "correct-key"),
	})
	if err := adapter.VerifyCallbackSignature(context.Background(), conn, nil, payload); err == nil {
		t.Fatal("signature generated with another key was accepted")
	}
}

// TranslateWebhook ----------------------------------------------------------

func TestMidtransAdapter_TranslateWebhook(t *testing.T) {
	adapter := midtrans.NewAdapter(silentLogger(), newTestVault(t))
	conn := &connectors.Connection{ID: 1, CompanyID: 42}
	for status, expected := range map[string]string{
		"settlement":     "payment.settled",
		"capture":        "payment.captured",
		"authorize":      "payment.authorized",
		"pending":        "payment.pending",
		"expire":         "payment.expired",
		"cancel":         "payment.cancelled",
		"deny":           "payment.failed",
		"refund":         "payment.refunded",
		"partial_refund": "payment.partially_refunded",
	} {
		t.Run(status, func(t *testing.T) {
			payload, _ := json.Marshal(map[string]string{
				"transaction_id":     "txn-abc",
				"order_id":           "inv-55-1700000001",
				"gross_amount":       "200000.00",
				"transaction_status": status,
				"status_code":        "200",
				"transaction_time":   "2026-08-10 12:00:00",
			})
			events, err := adapter.TranslateWebhook(context.Background(), conn, nil, payload)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(events) != 1 || events[0].EventType != expected {
				t.Fatalf("events = %#v, expected %q", events, expected)
			}
			if events[0].CorrelationID != "inv-55-1700000001" || events[0].CausationID != "txn-abc" {
				t.Fatalf("unexpected event identifiers: %#v", events[0])
			}
			if !events[0].EventTime.Equal(time.Date(2026, 8, 10, 5, 0, 0, 0, time.UTC)) {
				t.Fatalf("unexpected event time: %s", events[0].EventTime)
			}
		})
	}
}

func TestMidtransAdapter_TranslateWebhook_RejectsIncompletePayload(t *testing.T) {
	adapter := midtrans.NewAdapter(silentLogger(), newTestVault(t))
	conn := &connectors.Connection{ID: 1, CompanyID: 42}
	for _, payload := range [][]byte{[]byte("not-json{{{"), []byte(`{"transaction_status":"settlement"}`)} {
		if _, err := adapter.TranslateWebhook(context.Background(), conn, nil, payload); err == nil {
			t.Fatalf("incomplete payload %s was accepted", payload)
		}
	}
}

// ExecuteCommand ------------------------------------------------------------

func TestMidtransAdapter_ExecuteCommand_CreateCheckout(t *testing.T) {
	var gotMethod, gotPath, gotBody, gotUser string
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		gotMethod, gotPath = req.Method, req.URL.Path
		gotUser, _, _ = req.BasicAuth()
		body, _ := io.ReadAll(req.Body)
		gotBody = string(body)
		return response(http.StatusCreated, `{"token":"snap-tok-abc","redirect_url":"https://sandbox.invalid/snap/snap-tok-abc"}`, req), nil
	})
	adapter := midtrans.NewAdapter(silentLogger(), nil, connectors.ProviderOptions{
		HTTPClient:                &http.Client{Transport: transport},
		AllowPlaintextCredentials: true,
	})
	cmd := &connectors.OutboxCommand{
		ID:            11,
		CommandType:   "payment.create_checkout",
		CorrelationID: "corr-11",
		Payload:       []byte(`{"order_id":"inv-100-1700000000","gross_amount":150000,"customer_name":"Ada","customer_email":"ada@example.com"}`),
	}
	if err := adapter.ExecuteCommand(context.Background(), plaintextConnection(t, midtrans.Credentials{ServerKey: "SB-Mid-server-abc", BaseURL: "https://sandbox.invalid"}), cmd); err != nil {
		t.Fatalf("checkout command failed: %v", err)
	}
	if gotMethod != http.MethodPost || gotPath != "/snap/v1/transactions" || gotUser != "SB-Mid-server-abc" {
		t.Fatalf("request = %s %s user=%q", gotMethod, gotPath, gotUser)
	}
	if !strings.Contains(gotBody, `"order_id":"inv-100-1700000000"`) || !strings.Contains(gotBody, `"gross_amount":150000`) {
		t.Fatalf("unexpected request body: %s", gotBody)
	}
	var result midtrans.CheckoutResult
	if err := json.Unmarshal(cmd.Payload, &result); err != nil {
		t.Fatalf("decode checkout result: %v", err)
	}
	if result.Token != "snap-tok-abc" || result.RedirectURL == "" {
		t.Fatalf("unexpected checkout result: %#v", result)
	}
}

func TestMidtransAdapter_ExecuteCommand_RejectsUnsupportedAndMalformed(t *testing.T) {
	adapter := midtrans.NewAdapter(silentLogger(), newTestVault(t))
	conn := encryptedConn(t, newTestVault(t), "SB-Mid-server-abc")
	if err := adapter.ExecuteCommand(context.Background(), conn, &connectors.OutboxCommand{CommandType: "payment.void"}); err == nil {
		t.Fatal("unsupported command accepted")
	}
	if err := adapter.ExecuteCommand(context.Background(), conn, &connectors.OutboxCommand{CommandType: "payment.create_checkout", Payload: []byte("not-json")}); err == nil {
		t.Fatal("malformed checkout payload accepted")
	}
}

func TestMidtransAdapter_ExecuteCommand_RefundAndLookup(t *testing.T) {
	var refundBody string
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(req.Body)
		switch {
		case req.Method == http.MethodPost && req.URL.Path == "/v2/inv-55/refund":
			refundBody = string(body)
			return response(http.StatusOK, `{"order_id":"inv-55","transaction_id":"txn-55","transaction_status":"partial_refund","refund_key":"refund-55","refund_amount":"25000.00"}`, req), nil
		case req.Method == http.MethodGet && req.URL.Path == "/v2/inv-55/status":
			return response(http.StatusOK, `{"order_id":"inv-55","transaction_id":"txn-55","transaction_status":"settlement","transaction_time":"2026-08-10 12:00:00"}`, req), nil
		default:
			return response(http.StatusNotFound, `{}`, req), nil
		}
	})
	adapter := midtrans.NewAdapter(silentLogger(), nil, connectors.ProviderOptions{
		HTTPClient:                &http.Client{Transport: transport},
		AllowPlaintextCredentials: true,
	})
	conn := plaintextConnection(t, midtrans.Credentials{ServerKey: "SB-Mid-server-key", BaseURL: "https://sandbox.invalid"})

	refund := &connectors.OutboxCommand{
		CommandType: "payment.refund",
		Payload:     []byte(`{"order_id":"inv-55","refund_key":"refund-55","amount":25000,"reason":"sandbox test"}`),
	}
	if err := adapter.ExecuteCommand(context.Background(), conn, refund); err != nil {
		t.Fatalf("refund failed: %v", err)
	}
	if !strings.Contains(refundBody, `"refund_key":"refund-55"`) || !strings.Contains(refundBody, `"amount":25000`) {
		t.Fatalf("refund body = %s", refundBody)
	}
	var refundResult midtrans.RefundResult
	if err := json.Unmarshal(refund.Payload, &refundResult); err != nil {
		t.Fatal(err)
	}
	if refundResult.TransactionStatus != "partial_refund" || refundResult.RefundAmount != "25000.00" {
		t.Fatalf("refund result = %#v", refundResult)
	}

	lookup, err := adapter.LookupPaymentStatus(context.Background(), conn, "inv-55")
	if err != nil {
		t.Fatalf("lookup failed: %v", err)
	}
	if lookup.EventType != "payment.settled" || lookup.ProviderReference != "inv-55" || lookup.TransactionID != "txn-55" {
		t.Fatalf("lookup result = %#v", lookup)
	}
}

func TestMidtransAdapter_ExecuteCommand_APIError(t *testing.T) {
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return response(http.StatusUnauthorized, `{"error_messages":["Access denied"]}`, req), nil
	})
	adapter := midtrans.NewAdapter(silentLogger(), nil, connectors.ProviderOptions{
		HTTPClient:                &http.Client{Transport: transport},
		AllowPlaintextCredentials: true,
	})
	cmd := &connectors.OutboxCommand{
		CommandType: "payment.create_checkout",
		Payload:     []byte(`{"order_id":"inv-12-1700000005","gross_amount":100}`),
	}
	err := adapter.ExecuteCommand(context.Background(), plaintextConnection(t, midtrans.Credentials{ServerKey: "bad-key", BaseURL: "https://sandbox.invalid"}), cmd)
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("expected provider 401, got %v", err)
	}
}

func TestMidtransAdapter_ExecuteCommand_RetriesProviderFailure(t *testing.T) {
	attempts := 0
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		attempts++
		if attempts == 1 {
			return response(http.StatusBadGateway, `{"status_message":"temporary outage"}`, req), nil
		}
		return response(http.StatusCreated, `{"token":"snap-retried","redirect_url":"https://sandbox.invalid/snap/retried"}`, req), nil
	})
	adapter := midtrans.NewAdapter(silentLogger(), nil, connectors.ProviderOptions{
		HTTPClient:                &http.Client{Transport: transport},
		AllowPlaintextCredentials: true,
		RetryPolicy: connectors.RetryPolicy{
			MaxAttempts: 2,
			BaseDelay:   time.Nanosecond,
			MaxDelay:    time.Nanosecond,
		},
	})
	cmd := &connectors.OutboxCommand{
		CommandType: "payment.create_checkout",
		Payload:     []byte(`{"order_id":"inv-retry-1","gross_amount":100}`),
	}
	if err := adapter.ExecuteCommand(context.Background(), plaintextConnection(t, midtrans.Credentials{ServerKey: "SB-Mid-server-key", BaseURL: "https://sandbox.invalid"}), cmd); err != nil {
		t.Fatalf("retryable checkout failed: %v", err)
	}
	if attempts != 2 {
		t.Fatalf("expected 2 provider attempts, got %d", attempts)
	}
}

// Health and credentials ----------------------------------------------------

func TestMidtransAdapter_CheckHealth_404IsHealthy(t *testing.T) {
	var gotPath, gotUser string
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		gotPath = req.URL.Path
		gotUser, _, _ = req.BasicAuth()
		return response(http.StatusNotFound, `{"status_code":"404"}`, req), nil
	})
	adapter := midtrans.NewAdapter(silentLogger(), nil, connectors.ProviderOptions{
		HTTPClient:                &http.Client{Transport: transport},
		AllowPlaintextCredentials: true,
	})
	status, err := adapter.CheckHealth(context.Background(), &connectors.Connection{
		ID:        7,
		SecretRef: plaintextSecret(t, midtrans.Credentials{ServerKey: "SB-Mid-server-key", BaseURL: "https://sandbox.invalid"}),
	})
	if err != nil || status != connectors.StatusHealthy {
		t.Fatalf("CheckHealth() = %v, %v", status, err)
	}
	if gotPath != "/v2/odyssey-health-check-7/status" || gotUser != "SB-Mid-server-key" {
		t.Fatalf("health request = %s user=%q", gotPath, gotUser)
	}
}

func TestMidtransAdapter_CheckHealth_UsesProductionAPIEndpoint(t *testing.T) {
	var gotHost string
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		gotHost = req.URL.Host
		return response(http.StatusOK, `{"status_code":"200","transaction_status":"settlement"}`, req), nil
	})
	adapter := midtrans.NewAdapter(silentLogger(), nil, connectors.ProviderOptions{
		HTTPClient:                &http.Client{Transport: transport},
		AllowPlaintextCredentials: true,
	})
	status, err := adapter.CheckHealth(context.Background(), &connectors.Connection{
		ID:        8,
		SecretRef: plaintextSecret(t, midtrans.Credentials{ServerKey: "Mid-server-live-key", IsProd: true}),
	})
	if err != nil || status != connectors.StatusHealthy {
		t.Fatalf("CheckHealth() = %v, %v", status, err)
	}
	if gotHost != "api.midtrans.com" {
		t.Fatalf("production health host = %q", gotHost)
	}
}

func TestMidtransAdapter_ValidateConnection(t *testing.T) {
	vault := newTestVault(t)
	adapter := midtrans.NewAdapter(silentLogger(), vault)
	if err := adapter.ValidateConnection(context.Background(), encryptedConn(t, vault, "SB-Mid-server-key")); err != nil {
		t.Fatalf("valid credentials rejected: %v", err)
	}
	if err := adapter.ValidateConnection(context.Background(), &connectors.Connection{}); err == nil {
		t.Fatal("missing credentials accepted")
	}
}

func TestMidtransAdapter_RefreshToken(t *testing.T) {
	adapter := midtrans.NewAdapter(silentLogger(), newTestVault(t))
	if err := adapter.RefreshToken(context.Background(), &connectors.Connection{}); err != nil {
		t.Fatalf("RefreshToken should be a no-op for server-key auth: %v", err)
	}
}

func TestWebhookNotificationVerifySignatureThroughAdapter(t *testing.T) {
	const serverKey = "my-server-key"
	orderID, statusCode, grossAmount := "ORDER-999", "200", "50000.00"
	payload, _ := json.Marshal(map[string]string{
		"order_id":      orderID,
		"status_code":   statusCode,
		"gross_amount":  grossAmount,
		"signature_key": midtransSignature(orderID, statusCode, grossAmount, serverKey),
	})
	vault := newTestVault(t)
	adapter := midtrans.NewAdapter(silentLogger(), vault)
	conn := encryptedConn(t, vault, serverKey)
	if err := adapter.VerifyCallbackSignature(context.Background(), conn, nil, payload); err != nil {
		t.Fatalf("valid signature rejected: %v", err)
	}
}

func plaintextSecret(t *testing.T, credentials midtrans.Credentials) string {
	t.Helper()
	payload, err := json.Marshal(credentials)
	if err != nil {
		t.Fatalf("marshal credentials: %v", err)
	}
	return string(payload)
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

var _ connectors.ProviderAdapter = (*midtrans.Adapter)(nil)

func TestMain(m *testing.M) {
	if err := os.Setenv("APP_MASTER_KEY", "test-master-key-for-unit-tests-only"); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}
