package midtrans_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/odyssey-erp/odyssey-erp/internal/connectors"
	"github.com/odyssey-erp/odyssey-erp/internal/connectors/providers/midtrans"
)

// TestMidtransSandboxCertification is a deterministic provider-compatible
// sandbox contract. It exercises the same adapter commands and callback
// translation used against Midtrans, while making the test independent of a
// merchant account, customer card, or network availability.
func TestMidtransSandboxCertification(t *testing.T) {
	sandbox := newSandboxSimulator()
	adapter := midtrans.NewAdapter(silentLogger(), nil, connectors.ProviderOptions{
		HTTPClient:                &http.Client{Transport: sandbox},
		AllowPlaintextCredentials: true,
		DevelopmentMode:           true,
		RetryPolicy: connectors.RetryPolicy{
			MaxAttempts: 2,
			BaseDelay:   time.Nanosecond,
			MaxDelay:    time.Nanosecond,
		},
	})
	conn := plaintextConnection(t, midtrans.Credentials{ServerKey: sandboxServerKey, BaseURL: "http://midtrans-sandbox.invalid"})
	ctx := context.Background()

	// Checkout creates a pending transaction and returns a Snap token.
	checkout := &connectors.OutboxCommand{
		CommandType: "payment.create_checkout",
		Payload:     []byte(`{"order_id":"cert-main","gross_amount":100000,"currency":"IDR","customer_name":"Sandbox Buyer","customer_email":"buyer@example.com"}`),
	}
	if err := adapter.ExecuteCommand(ctx, conn, checkout); err != nil {
		t.Fatalf("checkout certification failed: %v", err)
	}
	var checkoutResult midtrans.CheckoutResult
	if err := json.Unmarshal(checkout.Payload, &checkoutResult); err != nil {
		t.Fatal(err)
	}
	if checkoutResult.Token == "" || checkoutResult.RedirectURL == "" {
		t.Fatalf("incomplete checkout result: %#v", checkoutResult)
	}

	mainPayment := &certifiedPayment{status: connectors.PaymentStatusPending}
	settledAt := time.Date(2026, 8, 10, 8, 0, 0, 0, time.UTC)
	applyCallback(t, adapter, conn, mainPayment, "cert-main", "settlement", "txn-main", "100000.00", settledAt)
	if mainPayment.status != connectors.PaymentStatusSettled || mainPayment.confirmations != 1 {
		t.Fatalf("settlement state = %#v", mainPayment)
	}

	// Midtrans can retry the same notification and can deliver an older status
	// after a newer status. Neither may create a second confirmation or reopen
	// the intent.
	duplicate := applyCallback(t, adapter, conn, mainPayment, "cert-main", "settlement", "txn-main", "100000.00", settledAt)
	if duplicate.Applied || !duplicate.Duplicate {
		t.Fatalf("duplicate callback result = %#v", duplicate)
	}
	stale := applyCallback(t, adapter, conn, mainPayment, "cert-main", "authorize", "txn-main", "100000.00", settledAt.Add(-time.Minute))
	if stale.Applied || !stale.OutOfOrder {
		t.Fatalf("out-of-order callback result = %#v", stale)
	}
	if mainPayment.confirmations != 1 {
		t.Fatalf("duplicate/out-of-order callbacks changed confirmation count: %d", mainPayment.confirmations)
	}

	// Partial and full refunds use distinct stable refund keys and converge via
	// the same callback reducer as a live integration.
	partialRefund := &connectors.OutboxCommand{
		CommandType: "payment.refund",
		Payload:     []byte(`{"order_id":"cert-main","refund_key":"refund-cert-partial","amount":25000,"reason":"partial sandbox refund"}`),
	}
	if err := adapter.ExecuteCommand(ctx, conn, partialRefund); err != nil {
		t.Fatalf("partial refund certification failed: %v", err)
	}
	applyCallback(t, adapter, conn, mainPayment, "cert-main", "partial_refund", "txn-main", "100000.00", settledAt.Add(2*time.Hour))
	if mainPayment.status != connectors.PaymentStatusPartiallyRefunded {
		t.Fatalf("partial refund state = %#v", mainPayment)
	}

	fullRefund := &connectors.OutboxCommand{
		CommandType: "payment.refund",
		Payload:     []byte(`{"order_id":"cert-main","refund_key":"refund-cert-full","reason":"full sandbox refund"}`),
	}
	if err := adapter.ExecuteCommand(ctx, conn, fullRefund); err != nil {
		t.Fatalf("full refund certification failed: %v", err)
	}
	applyCallback(t, adapter, conn, mainPayment, "cert-main", "refund", "txn-main", "100000.00", settledAt.Add(3*time.Hour))
	if mainPayment.status != connectors.PaymentStatusRefunded {
		t.Fatalf("full refund state = %#v", mainPayment)
	}

	// Expiry is terminal and a later stale settlement cannot produce an AR
	// payment.
	if err := createCheckout(adapter, conn, "cert-expire", 50000); err != nil {
		t.Fatal(err)
	}
	expiredPayment := &certifiedPayment{status: connectors.PaymentStatusPending}
	expiredAt := settledAt.Add(4 * time.Hour)
	applyCallback(t, adapter, conn, expiredPayment, "cert-expire", "expire", "txn-expire", "50000.00", expiredAt)
	lateSettlement := applyCallback(t, adapter, conn, expiredPayment, "cert-expire", "settlement", "txn-expire", "50000.00", expiredAt.Add(-time.Minute))
	if expiredPayment.status != connectors.PaymentStatusExpired || expiredPayment.confirmations != 0 || !lateSettlement.OutOfOrder {
		t.Fatalf("expiry recovery state = %#v, late=%#v", expiredPayment, lateSettlement)
	}

	reconciliation, err := connectors.ReconcileSettlement(connectors.PaymentSettlement{
		ProviderReference: "cert-main",
		PayoutReference:   "payout-cert-1",
		Currency:          "IDR",
		GrossMinor:        100000,
		FeeMinor:          2500,
		TaxMinor:          500,
		NetMinor:          97000,
		PayoutMinor:       97000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if reconciliation.Status != connectors.ReconciliationMatched || reconciliation.DifferenceMinor != 0 {
		t.Fatalf("payout reconciliation = %#v", reconciliation)
	}

	// The provider accepts the checkout but the response times out. Recovery
	// performs exactly one status lookup and never sends a second checkout.
	sandbox.timeoutNextCheckout = true
	timeoutAdapter := midtrans.NewAdapter(silentLogger(), nil, connectors.ProviderOptions{
		HTTPClient:                &http.Client{Transport: sandbox},
		AllowPlaintextCredentials: true,
		DevelopmentMode:           true,
		RetryPolicy:               connectors.RetryPolicy{MaxAttempts: 1},
	})
	timeoutCommand := &connectors.OutboxCommand{
		CommandType: "payment.create_checkout",
		Payload:     []byte(`{"order_id":"cert-timeout","gross_amount":75000,"currency":"IDR"}`),
	}
	if err := timeoutAdapter.ExecuteCommand(ctx, conn, timeoutCommand); err == nil {
		t.Fatal("timeout checkout unexpectedly succeeded")
	}
	timeoutSnapshot, err := timeoutAdapter.LookupPaymentStatus(ctx, conn, "cert-timeout")
	if err != nil {
		t.Fatalf("timeout lookup failed: %v", err)
	}
	timeoutPayment := &certifiedPayment{status: connectors.PaymentStatusCreated}
	timeoutTransition, err := connectors.ApplyPaymentTransition(timeoutPayment.status, time.Time{}, connectors.PaymentEvent{
		EventType:  timeoutSnapshot.EventType,
		OccurredAt: timeoutSnapshot.OccurredAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !timeoutTransition.Applied || timeoutTransition.ToStatus != connectors.PaymentStatusSettled {
		t.Fatalf("timeout recovery transition = %#v", timeoutTransition)
	}
	if sandbox.checkoutCalls["cert-timeout"] != 1 {
		t.Fatalf("timeout recovery created %d checkout requests", sandbox.checkoutCalls["cert-timeout"])
	}
}

const sandboxServerKey = "SB-Mid-server-certification-key"

type certifiedPayment struct {
	status        connectors.PaymentStatus
	lastEventAt   time.Time
	confirmations int
}

func applyCallback(t *testing.T, adapter *midtrans.Adapter, conn *connectors.Connection, payment *certifiedPayment, orderID, status, transactionID, grossAmount string, occurredAt time.Time) connectors.PaymentTransitionResult {
	t.Helper()
	payload := signedCallback(orderID, status, transactionID, grossAmount, occurredAt, sandboxServerKey)
	if err := adapter.VerifyCallbackSignature(context.Background(), conn, nil, payload); err != nil {
		t.Fatalf("callback signature rejected: %v", err)
	}
	events, err := adapter.TranslateWebhook(context.Background(), conn, nil, payload)
	if err != nil {
		t.Fatalf("callback translation failed: %v", err)
	}
	event := events[0]
	result, err := connectors.ApplyPaymentTransition(payment.status, payment.lastEventAt, connectors.PaymentEvent{
		EventType:       event.EventType,
		ProviderEventID: event.CausationID,
		OccurredAt:      event.EventTime,
	})
	if err != nil {
		t.Fatalf("callback lifecycle failed: %v", err)
	}
	if result.Applied {
		payment.status = result.ToStatus
		payment.lastEventAt = event.EventTime
		if event.EventType == "payment.captured" || event.EventType == "payment.settled" {
			payment.confirmations++
		}
	}
	return result
}

func signedCallback(orderID, status, transactionID, grossAmount string, occurredAt time.Time, serverKey string) []byte {
	return mustJSON(map[string]string{
		"order_id":           orderID,
		"transaction_id":     transactionID,
		"gross_amount":       grossAmount,
		"status_code":        "200",
		"transaction_status": status,
		"transaction_time":   occurredAt.In(time.FixedZone("WIB", 7*60*60)).Format("2006-01-02 15:04:05"),
		"signature_key":      midtransSignature(orderID, "200", grossAmount, serverKey),
	})
}

func createCheckout(adapter *midtrans.Adapter, conn *connectors.Connection, orderID string, amount int64) error {
	return adapter.ExecuteCommand(context.Background(), conn, &connectors.OutboxCommand{
		CommandType: "payment.create_checkout",
		Payload:     []byte(fmt.Sprintf(`{"order_id":%q,"gross_amount":%d,"currency":"IDR"}`, orderID, amount)),
	})
}

func mustJSON(value any) []byte {
	payload, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return payload
}

type sandboxOrder struct {
	amount          int64
	transactionID   string
	status          string
	transactionTime time.Time
	refundAmount    int64
}

type sandboxSimulator struct {
	mu                  sync.Mutex
	orders              map[string]*sandboxOrder
	checkoutCalls       map[string]int
	refundCalls         map[string]int
	timeoutNextCheckout bool
}

func newSandboxSimulator() *sandboxSimulator {
	return &sandboxSimulator{
		orders:        make(map[string]*sandboxOrder),
		checkoutCalls: make(map[string]int),
		refundCalls:   make(map[string]int),
	}
}

func (s *sandboxSimulator) RoundTrip(req *http.Request) (*http.Response, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	body, _ := io.ReadAll(req.Body)
	path := strings.TrimPrefix(req.URL.Path, "/")
	switch {
	case req.Method == http.MethodPost && path == "snap/v1/transactions":
		var payload midtrans.SnapTokenRequest
		if err := json.Unmarshal(body, &payload); err != nil {
			return response(http.StatusBadRequest, `{}`, req), nil
		}
		orderID := payload.TransactionDetails.OrderID
		s.checkoutCalls[orderID]++
		order := s.orders[orderID]
		if order == nil {
			order = &sandboxOrder{
				amount:          payload.TransactionDetails.GrossAmt,
				transactionID:   "txn-" + orderID,
				status:          "pending",
				transactionTime: time.Date(2026, 8, 10, 8, 0, 0, 0, time.UTC),
			}
			s.orders[orderID] = order
		}
		if s.timeoutNextCheckout {
			s.timeoutNextCheckout = false
			order.status = "settlement"
			return nil, context.DeadlineExceeded
		}
		return response(http.StatusCreated, fmt.Sprintf(`{"token":"snap-%s","redirect_url":"https://sandbox.invalid/snap/%s"}`, orderID, orderID), req), nil

	case req.Method == http.MethodGet && strings.HasPrefix(path, "v2/") && strings.HasSuffix(path, "/status"):
		orderID := strings.TrimSuffix(strings.TrimPrefix(path, "v2/"), "/status")
		order := s.orders[orderID]
		if order == nil {
			return response(http.StatusNotFound, `{}`, req), nil
		}
		return response(http.StatusOK, s.statusPayload(orderID, order), req), nil

	case req.Method == http.MethodPost && strings.HasPrefix(path, "v2/") && strings.HasSuffix(path, "/refund"):
		orderID := strings.TrimSuffix(strings.TrimPrefix(path, "v2/"), "/refund")
		order := s.orders[orderID]
		if order == nil {
			return response(http.StatusNotFound, `{}`, req), nil
		}
		var refund struct {
			Amount int64 `json:"amount"`
		}
		if err := json.Unmarshal(body, &refund); err != nil {
			return response(http.StatusBadRequest, `{}`, req), nil
		}
		s.refundCalls[orderID]++
		amount := refund.Amount
		if amount == 0 {
			amount = order.amount - order.refundAmount
		}
		order.refundAmount += amount
		if order.refundAmount >= order.amount {
			order.status = "refund"
		} else {
			order.status = "partial_refund"
		}
		return response(http.StatusOK, fmt.Sprintf(`{"order_id":%q,"transaction_id":%q,"transaction_status":%q,"refund_amount":"%d.00"}`, orderID, order.transactionID, order.status, amount), req), nil
	default:
		return response(http.StatusNotFound, `{}`, req), nil
	}
}

func (s *sandboxSimulator) statusPayload(orderID string, order *sandboxOrder) string {
	return fmt.Sprintf(`{"order_id":%q,"transaction_id":%q,"gross_amount":"%d.00","transaction_status":%q,"transaction_time":%q}`, orderID, order.transactionID, order.amount, order.status, order.transactionTime.In(time.FixedZone("WIB", 7*60*60)).Format("2006-01-02 15:04:05"))
}
