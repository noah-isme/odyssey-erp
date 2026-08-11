# ADR-004: Asynq for Background Jobs

## Status
Accepted

## Date
2024-02-05

## Context
The ERP system requires robust background processing for tasks like generating large financial reports, sending emails, and running scheduled nightly jobs. Since we are already planning to use a Redis-compatible data store for session management, we can leverage it for queuing.

## Decision
We will use `Asynq`, a Redis-backed task queue library for Go, and deploy a separate worker binary to process background jobs.

## Alternatives Considered
- RabbitMQ/Kafka: Overkill for our SMB target market and adds significant operational complexity.
- In-memory Goroutines: Not durable; tasks are lost if the server restarts. No built-in retry logic.
- PostgreSQL-based queues (e.g., River): Good alternative, but since we need Redis for sessions anyway, Asynq is a well-established and performant choice in the Go ecosystem.

## Consequences
### Positive
- Simple queue semantics and robust retry logic.
- Scheduled tasks and recurring jobs support.
- Worker scaling is straightforward.

### Negative
- Tied to Redis/Valkey as a dependency for the queue.

### Neutral
- Requires managing a separate worker process or routine.

## References
- [Asynq GitHub Repository](https://github.com/hibiken/asynq)
