package connectors_test

import (
	"testing"
	"time"

	"github.com/odyssey-erp/odyssey-erp/internal/connectors"
)

func TestApplyPaymentTransitionRejectsDuplicateAndOutOfOrderCallbacks(t *testing.T) {
	settledAt := time.Date(2026, 8, 10, 8, 0, 0, 0, time.UTC)
	result, err := connectors.ApplyPaymentTransition(connectors.PaymentStatusPending, time.Time{}, connectors.PaymentEvent{
		EventType:  "payment.settled",
		OccurredAt: settledAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Applied || result.ToStatus != connectors.PaymentStatusSettled {
		t.Fatalf("settlement result = %#v", result)
	}

	duplicate, err := connectors.ApplyPaymentTransition(connectors.PaymentStatusSettled, settledAt, connectors.PaymentEvent{
		EventType:       "payment.settled",
		ProviderEventID: "evt-settled-replay",
		OccurredAt:      settledAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	if duplicate.Applied || !duplicate.Duplicate {
		t.Fatalf("duplicate result = %#v", duplicate)
	}

	stale, err := connectors.ApplyPaymentTransition(connectors.PaymentStatusSettled, settledAt, connectors.PaymentEvent{
		EventType:       "payment.authorized",
		ProviderEventID: "evt-authorized-late",
		OccurredAt:      settledAt.Add(-time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	if stale.Applied || !stale.OutOfOrder {
		t.Fatalf("stale result = %#v", stale)
	}
}

func TestApplyPaymentTransitionCoversExpiryAndRefundBranches(t *testing.T) {
	pendingAt := time.Date(2026, 8, 10, 8, 0, 0, 0, time.UTC)
	expired, err := connectors.ApplyPaymentTransition(connectors.PaymentStatusPending, pendingAt, connectors.PaymentEvent{
		EventType:  "payment.expired",
		OccurredAt: pendingAt.Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !expired.Applied || expired.ToStatus != connectors.PaymentStatusExpired {
		t.Fatalf("expiry result = %#v", expired)
	}

	partial, err := connectors.ApplyPaymentTransition(connectors.PaymentStatusSettled, pendingAt, connectors.PaymentEvent{
		EventType:  "payment.partially_refunded",
		OccurredAt: pendingAt.Add(2 * time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !partial.Applied || partial.ToStatus != connectors.PaymentStatusPartiallyRefunded {
		t.Fatalf("partial refund result = %#v", partial)
	}

	full, err := connectors.ApplyPaymentTransition(connectors.PaymentStatusPartiallyRefunded, pendingAt.Add(2*time.Hour), connectors.PaymentEvent{
		EventType:  "payment.refunded",
		OccurredAt: pendingAt.Add(3 * time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !full.Applied || full.ToStatus != connectors.PaymentStatusRefunded {
		t.Fatalf("full refund result = %#v", full)
	}
}

func TestReconcileSettlement(t *testing.T) {
	matched, err := connectors.ReconcileSettlement(connectors.PaymentSettlement{
		ProviderReference: "order-1",
		PayoutReference:   "payout-1",
		Currency:          "IDR",
		GrossMinor:        100_000,
		FeeMinor:          2_500,
		TaxMinor:          500,
		NetMinor:          97_000,
		PayoutMinor:       97_000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if matched.Status != connectors.ReconciliationMatched || matched.DifferenceMinor != 0 {
		t.Fatalf("matched result = %#v", matched)
	}

	unmatched, err := connectors.ReconcileSettlement(connectors.PaymentSettlement{
		ProviderReference: "order-2",
		Currency:          "IDR",
		GrossMinor:        100_000,
		FeeMinor:          2_500,
		TaxMinor:          500,
		NetMinor:          97_000,
		PayoutMinor:       96_900,
	})
	if err != nil {
		t.Fatal(err)
	}
	if unmatched.Status != connectors.ReconciliationUnmatched || unmatched.DifferenceMinor != -100 {
		t.Fatalf("unmatched result = %#v", unmatched)
	}
}
