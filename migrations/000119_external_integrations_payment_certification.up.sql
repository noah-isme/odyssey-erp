-- Payment certification hardening: provider event IDs are immutable replay
-- keys, including refund callbacks. A repeated callback must not create a
-- second lifecycle transition.
CREATE UNIQUE INDEX IF NOT EXISTS idx_payment_intent_transitions_event
    ON payment_intent_transitions(payment_intent_id, provider_event_id)
    WHERE provider_event_id IS NOT NULL AND provider_event_id <> '';

-- Refund keys are persisted as provider_reference so a retry can be matched to
-- the original refund request rather than issuing a second refund.
CREATE UNIQUE INDEX IF NOT EXISTS idx_payment_refunds_provider_reference
    ON payment_refunds(payment_intent_id, provider_reference)
    WHERE provider_reference IS NOT NULL AND provider_reference <> '';

-- A checkout order is the provider idempotency boundary for one connection.
-- Prevent a repeated local checkout request from creating a second intent for
-- the same provider reference.
CREATE UNIQUE INDEX IF NOT EXISTS idx_payment_intents_connection_reference
    ON payment_intents(connection_id, provider_reference)
    WHERE provider_reference IS NOT NULL AND provider_reference <> '';
