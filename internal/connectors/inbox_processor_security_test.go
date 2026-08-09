package connectors

import "testing"

func TestWebhookEventIDIsDeterministicWhenProviderIDIsMissing(t *testing.T) {
	payload := []byte(`{"event":"same"}`)
	first := webhookEventID("dhl", nil, payload)
	second := webhookEventID("dhl", nil, payload)
	if first == "" || first != second {
		t.Fatalf("webhookEventID() = %q, %q; replay key must be stable", first, second)
	}
	if first == webhookEventID("dhl", nil, []byte(`{"event":"different"}`)) {
		t.Fatal("different payloads produced the same replay key")
	}
}

func TestWebhookEventIDPrefersProviderEventID(t *testing.T) {
	got := webhookEventID("stripe", map[string]string{"Stripe-Event-Id": "evt_1"}, []byte(`{"id":"evt_2"}`))
	if got != "evt_1" {
		t.Fatalf("webhookEventID() = %q, want provider event id", got)
	}
}

func TestWebhookEventIDExtractsStripePayloadID(t *testing.T) {
	got := webhookEventID("stripe", nil, []byte(`{"id":"evt_2"}`))
	if got != "evt_2" {
		t.Fatalf("webhookEventID() = %q, want Stripe payload id", got)
	}
}
