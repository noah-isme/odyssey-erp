# ADR-010: Valkey over Redis

## Status
Accepted

## Date
2024-04-05

## Context
Redis recently changed its licensing model away from the open-source BSD license. For a project aiming for open-source purity and avoiding potential future licensing issues, relying on proprietary or source-available licenses is a risk. The Linux Foundation has backed Valkey as the open-source continuation of Redis.

## Decision
We will use Valkey 8 (a Redis-compatible fork) for our in-memory data store, cache, and Asynq background job queue.

## Alternatives Considered
- Redis Stack/Enterprise: Rejected due to licensing changes (SSPL/RSAL).
- Memcached: Lacks the data structures (lists, sets) required by Asynq.

## Consequences
### Positive
- Truly open-source (Linux Foundation).
- Fully compatible with existing Redis clients and libraries (like Asynq).
- Avoids vendor lock-in and licensing ambiguity.

### Negative
- Less mainstream documentation and community tutorials currently compared to Redis.

### Neutral
- Drop-in replacement means no code changes are required on the Go side.

## References
- [Valkey GitHub Repository](https://github.com/valkey-io/valkey)
- [Linux Foundation Valkey Announcement](https://www.linuxfoundation.org/press/linux-foundation-launches-valkey)
