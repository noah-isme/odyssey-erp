-- name: CreatePaymentIntent :one
INSERT INTO payment_intents (
    company_id, connection_id, source_type, source_id, amount, currency, status, provider_reference, checkout_url
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9
) RETURNING *;

-- name: GetPaymentIntent :one
SELECT * FROM payment_intents
WHERE id = $1 AND company_id = $2;

-- name: GetPaymentIntentByProviderRef :one
SELECT * FROM payment_intents
WHERE connection_id = $1 AND provider_reference = $2;

-- name: UpdatePaymentIntentStatus :one
UPDATE payment_intents
SET status = $2, provider_reference = $3, checkout_url = $4, updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: InsertPaymentIntentTransition :one
INSERT INTO payment_intent_transitions (
    payment_intent_id, from_status, to_status, provider_event_id, occurred_at, raw_payload
) VALUES (
    $1, $2, $3, $4, $5, $6
) RETURNING *;

-- name: CreatePaymentRefund :one
INSERT INTO payment_refunds (
    payment_intent_id, amount, currency, reason, status, provider_reference
) VALUES (
    $1, $2, $3, $4, $5, $6
) RETURNING *;

-- name: UpdatePaymentRefundStatus :one
UPDATE payment_refunds
SET status = $2, updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: CreatePaymentDispute :one
INSERT INTO payment_disputes (
    payment_intent_id, amount, currency, reason, status, provider_reference
) VALUES (
    $1, $2, $3, $4, $5, $6
) RETURNING *;

-- name: UpdatePaymentDisputeStatus :one
UPDATE payment_disputes
SET status = $2, updated_at = NOW()
WHERE id = $1
RETURNING *;
