# Notifications and Transactional Email

Odyssey provides a persisted in-app notification center and asynchronous
transactional email delivery. The HTTP server records notifications and
enqueues email tasks; `cmd/worker` sends queued mail through the configured
SMTP server.

## Data model

Migration `000042_notifications` creates two independent tables:

| Table | Purpose |
|---|---|
| `notifications` | Per-recipient notification records, including type, title, body, URL, read timestamp, audit timestamps, and an internal delivery dedupe key. |
| `notification_preferences` | Per-user, per-notification-type channel switches for in-app and email delivery. |

`users.ui_notifications` remains a global workspace UI toggle. It only controls
whether the bell is visible and does not replace channel preferences.

When a `notification_preferences` row does not exist, both channels default to
enabled. To override a channel for a user and event type:

```sql
INSERT INTO notification_preferences
    (user_id, notification_type, in_app_enabled, email_enabled)
VALUES
    (42, 'report_delivered', TRUE, FALSE)
ON CONFLICT (user_id, notification_type) DO UPDATE
SET in_app_enabled = EXCLUDED.in_app_enabled,
    email_enabled = EXCLUDED.email_enabled,
    updated_at = NOW();
```

Supported initial notification types are:

- `invoice_issued`
- `approval_requested`
- `report_delivered`
- `password_reset`
- `approval_assigned`
- `approval_escalated`
- `approval_approved`
- `approval_rejected`

## HTTP API and workspace bell

All endpoints operate on the authenticated session user. A user cannot list or
mark another user's records.

| Method | Route | Behavior |
|---|---|---|
| `GET` | `/api/notifications?limit=10` | Lists recent notifications. |
| `GET` | `/api/notifications?unread=true&limit=10` | Lists unread notifications. |
| `GET` | `/api/notifications/unread-count` | Returns `{"count": n}`. |
| `POST` | `/api/notifications/{id}/read` | Marks one owned notification as read. |
| `POST` | `/api/notifications/read-all` | Marks all notifications for the session user as read. |

POST requests require the standard CSRF token. The workspace bell loads the
recent list and unread count, then calls the mark-read endpoints as the user
opens notification links or selects **Mark all read**.

## Event integrations

The initial dispatcher hooks are:

| Event | Current trigger | Recipient |
|---|---|---|
| Invoice issued | AR invoice changes from `DRAFT` to `POSTED`. | User posting the invoice. |
| Approval requested | Purchase order is submitted for approval. | User submitting the PO until role-based approver routing is introduced. |
| Report delivered | Board-pack generation reaches `READY`. | User stored in board-pack `requested_by` metadata. |
| Password reset | Authenticated password change succeeds. | User changing the password. |
| Approval assigned | An approval request enters a policy step. | Each resolved approver for the current step. |
| Approval escalated | A pending assignment passes its escalation deadline. | The overdue assignment's approver. |
| Approval approved | An approval request is finalized as approved. | The original requester. |
| Approval rejected | An approval request is finalized as rejected. | The original requester. |

Dispatch checks `notification_preferences` before each channel. An in-app
record is written only when the in-app channel is enabled, and email is queued
only when the email channel is enabled. A notification delivery failure is
logged without reverting the completed invoice, PO, report, or password action.

Each dispatched event has a stable dedupe key. Repeating dispatch after a queue
failure returns the original in-app record instead of inserting a duplicate.
Email tasks use the in-app notification ID as their correlation/task ID (or a
stable event hash when in-app delivery is disabled), so an enqueue retry cannot
create a second pending task. Transactional email tasks retry at most five times.

## SMTP configuration

Configure both the HTTP server and worker with the same environment. Only the
worker opens the SMTP connection; the server enqueues `mail:send` tasks in
Redis.

| Variable | Default | Description |
|---|---|---|
| `SMTP_HOST` | `127.0.0.1` | SMTP server hostname. |
| `SMTP_PORT` | `1025` | SMTP server port. |
| `SMTP_FROM` | `no-reply@odyssey.local` | Envelope and message sender. |
| `SMTP_USERNAME` | empty | Optional SMTP authentication username. |
| `SMTP_PASSWORD` | empty | Optional SMTP authentication password. |

For local Docker development, Mailpit is available as `mailpit:1025` and does
not require credentials. Never commit production SMTP credentials.

## Deployment and verification

1. Apply migrations `000042_notifications` and
   `000045_notification_delivery_idempotency` with `make migrate-up`.
2. Deploy or restart both `cmd/odyssey` and `cmd/worker`.
3. Confirm Redis is reachable by both processes and SMTP is reachable by the
   worker.
4. Post a draft AR invoice and verify the bell count increases.
5. Inspect the Asynq queue and SMTP inbox to verify email delivery.

Run the focused tests with:

```bash
ODYSSEY_TEST_MODE=1 GOTENBERG_URL=http://127.0.0.1:0 \
  go test ./internal/notifications ./jobs
```

## Troubleshooting

| Symptom | Check |
|---|---|
| Bell is hidden | Verify `users.ui_notifications` is `TRUE`. |
| Bell is visible but empty | Verify migration `000042` is applied and the event recipient is valid. |
| In-app works but no email task appears | Check `notification_preferences.email_enabled` and that the user is active with an email address. |
| Email task retries or fails | Check worker logs, `SMTP_HOST`, port, authentication, and network access. |
| Report notification is absent | Verify the board pack contains `requested_by` metadata and reached `READY`. |
