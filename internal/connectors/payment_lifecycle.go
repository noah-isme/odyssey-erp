package connectors

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// PaymentStatus is the provider-neutral state of a customer payment intent.
// The values intentionally match the payment_intents.status contract so the
// reducer can be used by both database repositories and sandbox certification.
type PaymentStatus string

const (
	PaymentStatusCreated           PaymentStatus = "CREATED"
	PaymentStatusPending           PaymentStatus = "PENDING"
	PaymentStatusAuthorized        PaymentStatus = "AUTHORIZED"
	PaymentStatusCaptured          PaymentStatus = "CAPTURED"
	PaymentStatusSettled           PaymentStatus = "SETTLED"
	PaymentStatusExpired           PaymentStatus = "EXPIRED"
	PaymentStatusFailed            PaymentStatus = "FAILED"
	PaymentStatusCancelled         PaymentStatus = "CANCELLED"
	PaymentStatusPartiallyRefunded PaymentStatus = "PARTIALLY_REFUNDED"
	PaymentStatusRefunded          PaymentStatus = "REFUNDED"
	PaymentStatusDisputed          PaymentStatus = "DISPUTED"
)

// PaymentEvent is the small canonical event shape needed to advance a
// payment. Provider-specific payloads stay in the inbox/transition record.
type PaymentEvent struct {
	EventType       string
	ProviderEventID string
	OccurredAt      time.Time
}

// PaymentStatusSnapshot is the provider-neutral result of an ambiguous
// request recovery lookup. The provider adapter owns the raw response; only
// these fields cross into the connector service.
type PaymentStatusSnapshot struct {
	ProviderReference string
	TransactionID     string
	Status            string
	EventType         string
	OccurredAt        time.Time
	RefundKey         string
	RefundAmount      string
	RefundReason      string
}

// PaymentStatusLookup is implemented by adapters that can query a provider
// after a timeout or worker crash before retrying a financial operation.
type PaymentStatusLookup interface {
	LookupPaymentStatus(context.Context, *Connection, string) (PaymentStatusSnapshot, error)
}

// PaymentTransitionResult explains why an event did or did not change state.
// Applied=false is deliberately not an error for a duplicate or stale
// callback; callers should acknowledge those callbacks without regressing the
// intent.
type PaymentTransitionResult struct {
	Applied       bool
	Duplicate     bool
	OutOfOrder    bool
	FromStatus    PaymentStatus
	ToStatus      PaymentStatus
	IgnoredReason string
}

var ErrUnknownPaymentEvent = errors.New("connectors: unknown payment event")

// PaymentStatusForEvent maps canonical connector events to payment states.
func PaymentStatusForEvent(eventType string) (PaymentStatus, bool) {
	switch strings.ToLower(strings.TrimSpace(eventType)) {
	case "payment.created":
		return PaymentStatusCreated, true
	case "payment.pending":
		return PaymentStatusPending, true
	case "payment.authorized":
		return PaymentStatusAuthorized, true
	case "payment.captured":
		return PaymentStatusCaptured, true
	case "payment.settled":
		return PaymentStatusSettled, true
	case "payment.expired":
		return PaymentStatusExpired, true
	case "payment.failed":
		return PaymentStatusFailed, true
	case "payment.cancelled", "payment.canceled":
		return PaymentStatusCancelled, true
	case "payment.partially_refunded":
		return PaymentStatusPartiallyRefunded, true
	case "payment.refunded":
		return PaymentStatusRefunded, true
	case "payment.disputed":
		return PaymentStatusDisputed, true
	default:
		return "", false
	}
}

// ApplyPaymentTransition applies one canonical event to the current state.
// It is intentionally side-effect free; the database implementation locks an
// intent row, calls this function, and persists the decision in one
// transaction.
func ApplyPaymentTransition(current PaymentStatus, lastEventAt time.Time, event PaymentEvent) (PaymentTransitionResult, error) {
	target, ok := PaymentStatusForEvent(event.EventType)
	if !ok {
		return PaymentTransitionResult{}, fmt.Errorf("%w: %s", ErrUnknownPaymentEvent, event.EventType)
	}
	if current == "" {
		current = PaymentStatusCreated
	}
	result := PaymentTransitionResult{FromStatus: current, ToStatus: target}

	if !event.OccurredAt.IsZero() && !lastEventAt.IsZero() && event.OccurredAt.Before(lastEventAt) {
		result.OutOfOrder = true
		result.IgnoredReason = "event occurred before the latest accepted provider event"
		return result, nil
	}
	if current == target {
		result.Duplicate = true
		result.IgnoredReason = "payment is already in the requested state"
		return result, nil
	}
	if !paymentTransitionAllowed(current, target) {
		result.IgnoredReason = "state transition would regress or reopen the payment"
		return result, nil
	}

	result.Applied = true
	return result, nil
}

func paymentTransitionAllowed(from, to PaymentStatus) bool {
	switch to {
	case PaymentStatusCreated:
		return false
	case PaymentStatusPending:
		return from == PaymentStatusCreated
	case PaymentStatusAuthorized:
		return from == PaymentStatusCreated || from == PaymentStatusPending
	case PaymentStatusCaptured:
		return from == PaymentStatusCreated || from == PaymentStatusPending || from == PaymentStatusAuthorized
	case PaymentStatusSettled:
		return from == PaymentStatusCreated || from == PaymentStatusPending || from == PaymentStatusAuthorized || from == PaymentStatusCaptured
	case PaymentStatusExpired:
		return from == PaymentStatusCreated || from == PaymentStatusPending
	case PaymentStatusFailed:
		return from == PaymentStatusCreated || from == PaymentStatusPending || from == PaymentStatusAuthorized
	case PaymentStatusCancelled:
		return from == PaymentStatusCreated || from == PaymentStatusPending || from == PaymentStatusAuthorized || from == PaymentStatusCaptured
	case PaymentStatusPartiallyRefunded:
		return from == PaymentStatusCaptured || from == PaymentStatusSettled || from == PaymentStatusPartiallyRefunded
	case PaymentStatusRefunded:
		return from == PaymentStatusCaptured || from == PaymentStatusSettled || from == PaymentStatusPartiallyRefunded
	case PaymentStatusDisputed:
		return from == PaymentStatusCaptured || from == PaymentStatusSettled || from == PaymentStatusPartiallyRefunded
	default:
		return false
	}
}

const (
	ReconciliationMatched   = "MATCHED"
	ReconciliationUnmatched = "UNMATCHED"
)

// PaymentSettlement contains exact minor-unit amounts from a gateway payout
// statement. Keeping amounts as integers prevents a reconciliation decision
// from changing because of float rounding.
type PaymentSettlement struct {
	ProviderReference string
	PayoutReference   string
	Currency          string
	PayoutCurrency    string
	GrossMinor        int64
	FeeMinor          int64
	TaxMinor          int64
	NetMinor          int64
	PayoutMinor       int64
}

type ReconciliationResult struct {
	Status           string
	ExpectedNetMinor int64
	DifferenceMinor  int64
	Reason           string
}

// ReconcileSettlement checks the accounting equation gross - fee - tax = net
// and then checks that the payout equals the expected net amount. A mismatch
// is a visible result, not an error, so operators can investigate provider
// fees, tax withholding, FX, or an unmatched payout.
func ReconcileSettlement(input PaymentSettlement) (ReconciliationResult, error) {
	if strings.TrimSpace(input.ProviderReference) == "" {
		return ReconciliationResult{}, errors.New("connectors: provider reference is required")
	}
	if len(strings.TrimSpace(input.Currency)) != 3 {
		return ReconciliationResult{}, errors.New("connectors: settlement currency must be ISO-4217")
	}
	if input.PayoutCurrency == "" {
		input.PayoutCurrency = input.Currency
	}
	if len(strings.TrimSpace(input.PayoutCurrency)) != 3 {
		return ReconciliationResult{}, errors.New("connectors: payout currency must be ISO-4217")
	}
	if input.GrossMinor < 0 || input.FeeMinor < 0 || input.TaxMinor < 0 || input.NetMinor < 0 || input.PayoutMinor < 0 {
		return ReconciliationResult{}, errors.New("connectors: settlement amounts cannot be negative")
	}

	expected := input.GrossMinor - input.FeeMinor - input.TaxMinor
	result := ReconciliationResult{
		Status:           ReconciliationMatched,
		ExpectedNetMinor: expected,
		DifferenceMinor:  input.PayoutMinor - expected,
	}
	if expected < 0 {
		result.Status = ReconciliationUnmatched
		result.Reason = "fees and tax exceed gross amount"
		return result, nil
	}
	if input.NetMinor != expected {
		result.Status = ReconciliationUnmatched
		result.Reason = "reported net does not equal gross less fee and tax"
		return result, nil
	}
	if !strings.EqualFold(input.Currency, input.PayoutCurrency) {
		result.Status = ReconciliationUnmatched
		result.Reason = "payout currency differs from transaction currency without an FX valuation"
		return result, nil
	}
	if input.PayoutMinor != expected {
		result.Status = ReconciliationUnmatched
		result.Reason = "payout amount does not equal expected net"
		return result, nil
	}
	result.Reason = "gross, fee, tax, net, and payout reconcile"
	return result, nil
}
