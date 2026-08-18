package midtransiris

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	accountingmoney "github.com/odyssey-erp/odyssey-erp/internal/accounting/money"
	"github.com/odyssey-erp/odyssey-erp/internal/connectors"
	"github.com/odyssey-erp/odyssey-erp/internal/finance/automation"
	"github.com/odyssey-erp/odyssey-erp/internal/finance/payments"
)

func TestLegacySubmitLookupCancelUsesStableReferencesAndExactAmount(t *testing.T) {
	var calls atomic.Int32
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		if got := r.Header.Get("Authorization"); !strings.HasPrefix(got, "Basic ") {
			t.Fatalf("authorization = %q", got)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/iris/api/v1/payouts":
			var payload legacyPayoutRequest
			if err := json.Unmarshal(body, &payload); err != nil {
				t.Fatal(err)
			}
			if len(payload.Payouts) != 1 || payload.Payouts[0].Amount != "10000.25" || payload.Payouts[0].BeneficiaryBank != "bca" || payload.Payouts[0].BeneficiaryAccount != "1234" {
				t.Fatalf("payout payload = %+v", payload)
			}
			if got := r.Header.Get("Idempotency-Key"); got != "item-42" {
				t.Fatalf("idempotency key = %q", got)
			}
			return fixtureResponse(r, http.StatusOK, `{"payouts":[{"reference_no":"iris-99","status":"requested"}]}`), nil
		case r.Method == http.MethodGet && r.URL.Path == "/iris/api/v1/payouts/iris-99":
			return fixtureResponse(r, http.StatusOK, `{"reference_no":"iris-99","status":"completed","amount":"10000.25","updated_at":"2026-08-15T09:00:00Z"}`), nil
		case r.Method == http.MethodPost && r.URL.Path == "/iris/api/v1/payouts/reject":
			return fixtureResponse(r, http.StatusOK, `{"status":"rejected"}`), nil
		default:
			return fixtureResponse(r, http.StatusNotFound, `{}`), nil
		}
	})}

	ref := testConnection()
	adapter := NewAdapter(slog.New(slog.NewTextHandler(io.Discard, nil)), nil, Options{
		ProviderOptions:   connectors.ProviderOptions{DevelopmentMode: true, HTTPClient: client},
		StaticCredentials: Credentials{APIKey: "iris-test-key", BaseURL: "https://iris.test/iris/api/v1"},
	})
	instruction := testInstruction(ref, automation.ExactAmount{Amount: accountingmoney.Must("10000.25", 2), Currency: "IDR"})

	submission, err := adapter.Submit(context.Background(), ref, instruction)
	if err != nil {
		t.Fatal(err)
	}
	if submission.Reference.ObjectType != "midtrans_iris_payout" || submission.Reference.ObjectID != "iris-99" || submission.Status != "REQUESTED" {
		t.Fatalf("submission = %+v", submission)
	}

	settlement, err := adapter.Lookup(context.Background(), ref, submission.Reference)
	if err != nil {
		t.Fatal(err)
	}
	if settlement.Status != payments.SettlementStatusSettled || settlement.SettledAmount.Amount.String() != "10000.25" || settlement.SettledAmount.Currency != "IDR" {
		t.Fatalf("settlement = %+v", settlement)
	}

	cancel, err := adapter.Cancel(context.Background(), ref, submission.Reference)
	if err != nil {
		t.Fatal(err)
	}
	if cancel.Status != payments.SettlementStatusCancelled {
		t.Fatalf("cancel = %+v", cancel)
	}
	if calls.Load() != 3 {
		t.Fatalf("request count = %d", calls.Load())
	}
}

func TestLegacySubmitTransportFailureIsAmbiguousAndDoesNotRetry(t *testing.T) {
	var calls atomic.Int32
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		if r.URL.Path != "/iris/api/v1/payouts" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		// A lost response models the provider accepting a payout and the
		// response being unavailable. The adapter must not create a second.
		return nil, errors.New("connection reset after request")
	})}

	ref := testConnection()
	adapter := NewAdapter(nil, nil, Options{
		ProviderOptions:   connectors.ProviderOptions{DevelopmentMode: true, HTTPClient: client},
		StaticCredentials: Credentials{APIKey: "iris-test-key", BaseURL: "https://iris.test/iris/api/v1"},
	})
	_, err := adapter.Submit(context.Background(), ref, testInstruction(ref, automation.ExactAmount{Amount: accountingmoney.Must("1", 0), Currency: "IDR"}))
	if err == nil {
		t.Fatal("expected ambiguous error")
	}
	var providerErr *automation.ProviderError
	if !errors.As(err, &providerErr) || providerErr.Category != automation.ErrorAmbiguous || providerErr.Retryable() {
		t.Fatalf("error = %#v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("request count = %d", calls.Load())
	}
}

func TestBISNAPAccessTokenAndTransactionalSignature(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	privatePEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	const timestamp = "2026-08-15T09:00:00Z"
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/v1.0/access-token/b2b":
			if r.Method != http.MethodPost || r.Header.Get("X-CLIENT-KEY") != "client-1" || r.Header.Get("X-TIMESTAMP") != timestamp || r.Header.Get("X-SIGNATURE") == "" {
				t.Fatalf("token headers = %#v", r.Header)
			}
			return fixtureResponse(r, http.StatusOK, `{"responseCode":"2007300","accessToken":"token-1","expiresIn":"900"}`), nil
		case "/v1.0/debit/payment-host-to-host":
			if r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer token-1" || r.Header.Get("X-PARTNER-ID") != "partner-1" || r.Header.Get("CHANNEL-ID") != "12345" || r.Header.Get("X-SIGNATURE") == "" {
				t.Fatalf("transaction headers = %#v", r.Header)
			}
			return fixtureResponse(r, http.StatusOK, `{"responseCode":"2005400","referenceNo":"provider-1","status":"PROCESSING"}`), nil
		default:
			return fixtureResponse(r, http.StatusNotFound, `{}`), nil
		}
	})}

	ref := testConnection()
	adapter := NewAdapter(nil, nil, Options{
		ProviderOptions: connectors.ProviderOptions{DevelopmentMode: true, HTTPClient: client},
		Now:             func() time.Time { parsed, _ := time.Parse(time.RFC3339, timestamp); return parsed },
		StaticCredentials: Credentials{
			ClientID: "client-1", ClientSecret: "client-secret", PartnerID: "partner-1",
			PrivateKeyPEM: string(privatePEM), BaseURL: "https://iris.test",
			ChannelID: "12345", SubmitPath: "/v1.0/debit/payment-host-to-host",
		},
	})
	submission, err := adapter.Submit(context.Background(), ref, testInstruction(ref, automation.ExactAmount{Amount: accountingmoney.Must("10", 0), Currency: "IDR"}))
	if err != nil {
		t.Fatal(err)
	}
	if submission.Reference.ObjectID != "provider-1" || submission.Status != "PROCESSING" {
		t.Fatalf("submission = %+v", submission)
	}
}

func TestBISNAPLookupAndCancelUseDurableProviderAndInstructionReferences(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		if got := len(r.Header.Get("X-EXTERNAL-ID")); got > 36 {
			t.Fatalf("X-EXTERNAL-ID length = %d, want <= 36", got)
		}
		var payload map[string]string
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatal(err)
		}
		if payload["originalReferenceNo"] != "provider-1" || payload["originalPartnerReferenceNo"] != "item-42" || payload["originalExternalId"] != "item-42" {
			t.Fatalf("durable reference payload = %#v", payload)
		}
		switch r.URL.Path {
		case "/v1.0/debit/status":
			return fixtureResponse(r, http.StatusOK, `{"responseCode":"2005500","originalReferenceNo":"provider-1","latestTransactionStatus":"00","transAmount":{"value":"10.00","currency":"IDR"}}`), nil
		case "/v1.0/debit/cancel":
			return fixtureResponse(r, http.StatusOK, `{"responseCode":"2005700","originalReferenceNo":"provider-1","cancelTime":"2026-08-15T09:00:00Z"}`), nil
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
			return nil, nil
		}
	})}
	ref := testConnection()
	adapter := NewAdapter(nil, nil, Options{
		ProviderOptions: connectors.ProviderOptions{DevelopmentMode: true, HTTPClient: client},
		StaticCredentials: Credentials{
			ClientID: "client-1", PartnerID: "partner-1", AccessToken: "token-1", BaseURL: "https://iris.test",
		},
	})
	instruction := testInstruction(ref, automation.ExactAmount{Amount: accountingmoney.Must("10", 0), Currency: "IDR"})
	payout := providerReference(ref, "provider-1")

	settlement, err := adapter.LookupWithInstruction(context.Background(), ref, instruction.Reference, payout)
	if err != nil {
		t.Fatal(err)
	}
	if settlement.Status != payments.SettlementStatusSettled || settlement.Instruction.ObjectID != instruction.Reference.ObjectID {
		t.Fatalf("lookup settlement = %+v", settlement)
	}

	cancelled, err := adapter.CancelWithInstruction(context.Background(), ref, instruction.Reference, payout)
	if err != nil {
		t.Fatal(err)
	}
	if cancelled.Status != payments.SettlementStatusCancelled {
		t.Fatalf("cancel settlement = %+v", cancelled)
	}
}

func TestTrimExternalIDUsesBISNAPLimit(t *testing.T) {
	if got := trimExternalID(strings.Repeat("x", 64)); len(got) != 36 {
		t.Fatalf("trimExternalID length = %d, want 36", len(got))
	}
}

func TestCredentialsAreNotAcceptedWithoutVaultOrExplicitTestMode(t *testing.T) {
	adapter := NewAdapter(nil, nil, Options{StaticCredentials: Credentials{APIKey: "secret"}})
	err := adapter.ValidateConnection(context.Background(), testConnection())
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("error = %v", err)
	}
}

func testConnection() automation.ConnectionRef {
	return automation.ConnectionRef{CompanyID: 7, ConnectionID: 11, Provider: Provider}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func fixtureResponse(req *http.Request, status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Status: http.StatusText(status), Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header), Request: req, ContentLength: int64(len(body))}
}

func testInstruction(ref automation.ConnectionRef, amount automation.ExactAmount) payments.Instruction {
	return payments.Instruction{
		Reference:         automation.ExternalReference{Connection: ref, ObjectType: "treasury_payment_batch_item", ObjectID: "item-42"},
		Correlation:       automation.Correlation{ID: "corr-42"},
		BeneficiaryRef:    "bca:1234",
		BeneficiaryName:   "Budi",
		Amount:            amount,
		EndToEndReference: "e2e-42",
	}
}
