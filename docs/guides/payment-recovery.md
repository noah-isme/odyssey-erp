# Payment Connector Recovery Runbook

This runbook covers provider-status recovery, unmatched-payment alerts, refund
state persistence, connector dead letters, and worker metrics. It applies to the
payment connector worker and is separate from bank-statement reconciliation.

## Runtime contract

The worker registers two scheduled tasks:

| Task | Schedule | Scope |
|---|---:|---|
| `payments:reconcile` | every 5 minutes | Checks up to 100 stale payment intents through a provider status API |
| `connectors:dead_letter_audit` | every 5 minutes | Audits up to 100 unreplayed connector dead letters |

Reconciliation considers intents older than two minutes in `CREATED`, `PENDING`,
`AUTHORIZED`, `CAPTURED`, `SETTLED`, or `PARTIALLY_REFUNDED`. A provider lookup is
passed through the same monotonic reducer as a webhook, so duplicate and
out-of-order results are safe to acknowledge and cannot reopen a terminal payment.

Run the worker and web application from the same release commit. Apply migration
`000120_payment_reconciliation_operations` before starting a worker that contains
these tasks.

## Durable evidence

The following tables are the operational source of truth:

- `payment_reconciliation_runs` — one row per scheduled pass, including status,
  scanned/recovered/matched/unmatched counts, refund count, and error text.
- `payment_reconciliation_issues` — deduplicated open/resolved issues for provider
  outages, lookup failures, unmapped statuses, missing intents, and state mismatches.
- `connector_dead_letter_events` — connector commands that exhausted five attempts,
  including provider, correlation ID, error, alert, and replay timestamps.
- `payment_refunds` — refund request and provider-confirmation state. A request is
  persisted before its `payment.refund` outbox command is queued.

Useful read-only triage queries:

```sql
-- Recent reconciliation runs
SELECT id, started_at, finished_at, status, scanned_count, recovered_count,
       matched_count, unmatched_count, unsupported_count, error_count,
       refunds_persisted, error_message
FROM payment_reconciliation_runs
ORDER BY started_at DESC
LIMIT 20;

-- Open payment issues, oldest first
SELECT id, company_id, connection_id, payment_intent_id, provider,
       provider_reference, issue_type, expected_status, observed_status,
       last_seen_at, alerted_at, details
FROM payment_reconciliation_issues
WHERE status = 'OPEN'
ORDER BY last_seen_at ASC;

-- Unreplayed connector dead letters
SELECT d.id, d.company_id, d.connection_id, c.provider, d.command_type,
       d.correlation_id, d.attempts, d.dead_lettered_at, d.alerted_at,
       d.error_message
FROM connector_dead_letter_events d
JOIN connector_connections c
  ON c.id = d.connection_id AND c.company_id = d.company_id
WHERE d.replayed_at IS NULL
ORDER BY d.dead_lettered_at ASC;

-- Refunds requiring attention
SELECT r.id, i.company_id, i.connection_id, i.provider_reference AS order_reference,
       r.provider_reference AS refund_key, r.amount, r.currency, r.status,
       r.reason, r.created_at, r.updated_at
FROM payment_refunds r
JOIN payment_intents i ON i.id = r.payment_intent_id
WHERE r.status IN ('PENDING', 'PROCESSING', 'FAILED')
ORDER BY r.updated_at ASC;
```

Do not include secret references, decrypted credentials, or raw provider payloads
in tickets or chat messages.

## Alerts and issue resolution

The worker sends in-app and email notifications to active global administrators and
company-scoped administrators. A given issue is alerted when first opened, when it
reopens, or after the one-hour alert interval. Notification keys include the hourly
bucket so a persistent issue can generate a fresh alert without creating duplicates
inside the same interval.

An issue is resolved when a later lookup produces an accepted, duplicate, or
out-of-order lifecycle result for the provider reference. If a provider is
unavailable or does not implement status lookup, leave the issue open while the
connection, credentials, or adapter is repaired; the next scheduled pass retries it.

## Refund recovery

Refunds follow this state contract:

```text
PENDING -> PROCESSING -> PARTIALLY_REFUNDED -> REFUNDED
                     \-> REFUNDED
PENDING/PROCESSING -> FAILED   (after connector retry exhaustion)
```

The stable refund key is the idempotency boundary. Never create a second refund
request because a command response timed out. Check the provider by the original
order reference and reuse the same refund key. A provider callback or status lookup
persists the returned refund key/amount and advances the local payment intent through
the lifecycle reducer. Terminal provider-confirmed states are not overwritten by a
dead-letter update.

For a `PENDING` or `PROCESSING` refund:

1. Inspect the provider reference, refund key, and latest connector error.
2. Query the provider status endpoint or merchant console using the original order
   reference.
3. If the provider accepted the refund, deliver/allow the callback or reconciliation
   pass to persist `PARTIALLY_REFUNDED` or `REFUNDED`.
4. If the provider rejected it, record the reason and submit a new request only with
   an intentionally new refund key and approved amount.

## Dead-letter recovery

After five failed connector attempts, the outbox command enters `dead_letter`, a
row is written to `connector_dead_letter_events`, and the associated refund is
marked `FAILED` when applicable. The audit task alerts administrators once per hour.

Replay is deliberately explicit. Before replaying, confirm that the provider did
not already accept the command and that its idempotency/correlation key is safe to
reuse. The application service boundary is
`PaymentReconciliationService.ReplayDeadLetter(ctx, deadLetterID)`; expose that
method only through an authenticated, audited operator surface. Do not replay by
editing the outbox row directly and do not automatically replay ambiguous financial
commands.

After an approved replay, verify that the outbox command returns to `pending`, the
dead-letter row receives `replayed_at`, and the next sweep produces the expected
provider callback or a new, actionable failure.

## Metrics and health checks

Set `WORKER_METRICS_ADDR` to an internal-only listener; it defaults to `:9091`.
Scrape:

```bash
curl -fsS http://127.0.0.1:9091/metrics \
  | rg 'odyssey_payment_reconciliation|odyssey_payment_recovery|odyssey_payment_refund|odyssey_connector_dead_letters'
```

The recovery metric families are:

- `odyssey_payment_reconciliation_runs_total{status}`
- `odyssey_payment_reconciliation_candidates_total{provider,local_status}`
- `odyssey_payment_recovery_transitions_total{provider,from_status,to_status}`
- `odyssey_payment_reconciliation_issues_total{provider,issue_type}`
- `odyssey_payment_refund_status_total{provider,status}`
- `odyssey_connector_dead_letters_total{command_type}`

Alert when reconciliation runs stop arriving, `PARTIAL`/`FAILED` runs persist,
open issues or unreplayed dead letters grow, or refund states remain `PENDING`,
`PROCESSING`, or `FAILED` beyond the provider's documented SLA. Keep the worker
metrics listener off the public interface.

## Local acceptance

The deterministic Midtrans contract covers duplicate/out-of-order callbacks,
expiry, partial/full refunds, payout equation checks, and timeout recovery:

```bash
make midtrans-sandbox-certify
```

The local contract does not replace merchant sandbox evidence for customer
completion, provider expiry timing, bank-confirmed refunds, payout reports, or
limited-production credentials.

## v0.11 finance payment settlement recovery

The isolated `finance-sandbox` worker also runs
`finance:payment_recovery_scan` every five minutes when
`APP_ENV=finance-sandbox` and `RELEASE_PROFILE=v0.11-finance`. This scan is
separate from the legacy connector reconciliation alert sink. Its read-only
projection covers:

- `payment_executions` in `AMBIGUOUS`, `PARTIALLY_SETTLED`, or `FAILED`;
- confirmed `payment_settlement_results` with unapplied effects or without a
  linked bank reconciliation record after the configured threshold; and
- `payment.execute`, `payment.submit`, and `payment.result.import` commands in a
  stale `PROCESSING` lease or `DEAD_LETTERED` state.

Cases are deduplicated by company, connection, provider-neutral instruction
reference, and issue type. Notifications go only to active company-scoped
administrators/finance operators and contain a safe status summary; provider
payloads, credentials, beneficiary details, and raw error bodies are never
copied into notifications. Repeated scans reuse the same notification key.

The scan never submits, cancels, looks up, or replays a provider command. For an
ambiguous execution, use the guarded operations workbench recovery action; the
provider lookup must complete before a deliberate replay can be enqueued. For a
failed result-effect import, retry the idempotent result-import command. Never
declare settlement manually, edit an outbox row directly, or run a blind retry.

The additional finance metrics are exposed on the internal worker listener:

- `odyssey_finance_payment_execution_outcomes_total{provider,state}`;
- `odyssey_finance_payment_ambiguous_age_seconds{provider}`;
- `odyssey_finance_payment_unapplied_effects{provider,state}`;
- `odyssey_finance_payment_unmatched_settlements{provider}`;
- `odyssey_finance_payment_dead_letters_total{operation}`;
- `odyssey_finance_payment_recovery_attempts_total{action,outcome}`;
- `odyssey_finance_payment_recovery_success_total{action}`; and
- `odyssey_finance_payment_provider_lookup_latency_seconds{provider}`.

For certification, retain the metric capture, notification evidence, provider
lookup trace, effect links, and previous-release rollback proof in the
[v0.11 finance sandbox certification record](../releases/v0.11-finance-sandbox-certification.md).
