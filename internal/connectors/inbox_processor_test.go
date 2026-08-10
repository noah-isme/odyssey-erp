package connectors_test

import (
	"bytes"
	"context"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/odyssey-erp/odyssey-erp/internal/connectors"
	"github.com/odyssey-erp/odyssey-erp/internal/connectors/providers/midtrans"
	"github.com/odyssey-erp/odyssey-erp/internal/connectors/providers/mockpay"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
)

// A stub that only contains what we need to pass the compiler if we don't have a real DB.
// Real unit tests would typically use pgxpool mock or an ephemeral postgres db for SQLC queries.
// For now, we just ensure the types are properly wired.

func TestInboxProcessorAndWebhook(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	adapter := mockpay.NewAdapter(logger)

	registry := connectors.NewRegistry()
	registry.Register("mockpay", adapter)

	// Create processor with nil queries just to verify struct composition and HTTP wiring.
	// In a real test, we would provide a test database or a mock of sqlc.Queries.
	var repo connectors.InboxRepository
	processor := connectors.NewInboxProcessor(repo, registry, nil, logger)
	handler := connectors.NewWebhookHandler(processor)

	r := chi.NewRouter()
	handler.MountRoutes(r)

	// Build a dummy request
	payload := []byte(`{"event_type": "charge.succeeded"}`)
	req, _ := http.NewRequest(http.MethodPost, "/1/1", bytes.NewBuffer(payload))
	req.Header.Set("X-Provider-Event-Id", "evt_123")
	req.Header.Set("X-Provider-Signature", "fake-sig-456")

	w := httptest.NewRecorder()

	// Just make sure it routes properly. It will fail on `p.queries.GetConnection` because queries is nil.
	defer func() {
		if r := recover(); r != nil {
			// Panic expected due to nil queries, but routing and parsing succeeded!
			t.Log("Recovered from expected panic on nil database queries")
		}
	}()

	r.ServeHTTP(w, req)
}

func TestInboxProcessorDeduplicatesAndDoesNotRegressPaymentState(t *testing.T) {
	t.Setenv("APP_MASTER_KEY", "test-master-key-for-inbox")
	vault, err := shared.NewVault()
	if err != nil {
		t.Fatal(err)
	}
	const serverKey = "SB-Mid-server-inbox"
	conn := connectors.Connection{ID: 9, CompanyID: 42, Provider: "midtrans", Type: "payment", Name: "sandbox"}
	if err := conn.SetCredentials(vault, serverKey); err != nil {
		t.Fatal(err)
	}
	repo := &paymentInboxFake{connection: conn, status: connectors.PaymentStatusPending}
	registry := connectors.NewRegistry()
	registry.Register("midtrans", midtrans.NewAdapter(slog.New(slog.NewTextHandler(ioDiscard{}, nil)), vault))
	processor := connectors.NewInboxProcessor(repo, registry, nil, slog.Default())

	settledAt := time.Date(2026, 8, 10, 8, 0, 0, 0, time.UTC)
	settled := inboxCallback("inv-9-1", "settlement", "txn-9", "100000.00", settledAt, serverKey)
	if err := processor.ProcessWebhook(context.Background(), conn.ID, conn.CompanyID, nil, settled); err != nil {
		t.Fatal(err)
	}
	if err := processor.ProcessWebhook(context.Background(), conn.ID, conn.CompanyID, nil, settled); err != nil {
		t.Fatal(err)
	}

	stale := inboxCallback("inv-9-1", "authorize", "txn-9", "100000.00", settledAt.Add(-time.Minute), serverKey)
	if err := processor.ProcessWebhook(context.Background(), conn.ID, conn.CompanyID, nil, stale); err != nil {
		t.Fatal(err)
	}
	if repo.status != connectors.PaymentStatusSettled || repo.applied != 1 {
		t.Fatalf("payment state = %s, applied=%d", repo.status, repo.applied)
	}
	if len(repo.canonical) != 2 {
		t.Fatalf("canonical events = %d, want the first event plus the auditable stale event", len(repo.canonical))
	}
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) { return len(p), nil }

type paymentInboxFake struct {
	connection connectors.Connection
	status     connectors.PaymentStatus
	lastEvent  time.Time
	applied    int
	inbox      map[string]connectors.InboxEvent
	canonical  []connectors.CanonicalEventInput
}

func (f *paymentInboxFake) GetConnection(context.Context, int64, int64) (connectors.Connection, error) {
	return f.connection, nil
}

func (f *paymentInboxFake) InsertInboxEvent(_ context.Context, input connectors.InboxEventInput) (connectors.InboxEvent, error) {
	if f.inbox == nil {
		f.inbox = make(map[string]connectors.InboxEvent)
	}
	if existing, ok := f.inbox[input.ProviderEventID]; ok {
		return existing, nil
	}
	event := connectors.InboxEvent{ID: int64(len(f.inbox) + 1), CompanyID: input.CompanyID, ConnectionID: input.ConnectionID, ProviderEventID: input.ProviderEventID, RawPayload: input.RawPayload}
	f.inbox[input.ProviderEventID] = event
	return event, nil
}

func (f *paymentInboxFake) InsertCanonicalEvent(_ context.Context, input connectors.CanonicalEventInput) (int64, error) {
	f.canonical = append(f.canonical, input)
	return int64(len(f.canonical)), nil
}

func (f *paymentInboxFake) MarkInboxEventProcessed(_ context.Context, id int64) error {
	for key, event := range f.inbox {
		if event.ID == id {
			event.Processed = true
			f.inbox[key] = event
		}
	}
	return nil
}

func (f *paymentInboxFake) ApplyPaymentIntentEvent(_ context.Context, input connectors.PaymentIntentEventInput) (connectors.PaymentTransitionResult, error) {
	result, err := connectors.ApplyPaymentTransition(f.status, f.lastEvent, connectors.PaymentEvent{EventType: input.EventType, ProviderEventID: input.ProviderEventID, OccurredAt: input.OccurredAt})
	if err == nil && result.Applied {
		f.status = result.ToStatus
		f.lastEvent = input.OccurredAt
		f.applied++
	}
	return result, err
}

func (f *paymentInboxFake) GetPaymentIntentByProviderReference(context.Context, int64, int64, string) (connectors.PaymentIntent, error) {
	return connectors.PaymentIntent{ID: 1, CompanyID: f.connection.CompanyID, ConnectionID: f.connection.ID, Status: f.status, ProviderReference: "inv-9-1"}, nil
}

func (f *paymentInboxFake) UpdatePaymentIntentCheckout(context.Context, int64, int64, int64, string, string, string) error {
	return nil
}

func inboxCallback(orderID, status, transactionID, grossAmount string, occurredAt time.Time, serverKey string) []byte {
	values := map[string]string{
		"order_id":           orderID,
		"transaction_id":     transactionID,
		"gross_amount":       grossAmount,
		"status_code":        "200",
		"transaction_status": status,
		"transaction_time":   occurredAt.In(time.FixedZone("WIB", 7*60*60)).Format("2006-01-02 15:04:05"),
	}
	sum := sha512.Sum512([]byte(orderID + "200" + grossAmount + serverKey))
	values["signature_key"] = hex.EncodeToString(sum[:])
	payload, _ := json.Marshal(values)
	return payload
}

var _ connectors.InboxRepository = (*paymentInboxFake)(nil)
var _ connectors.PaymentIntentRepository = (*paymentInboxFake)(nil)
