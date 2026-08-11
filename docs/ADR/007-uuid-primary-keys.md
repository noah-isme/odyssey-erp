# ADR-007: UUID Primary Keys

## Status
Accepted

## Date
2024-03-01

## Context
Choosing the right primary key strategy is fundamental. Auto-incrementing integers are efficient but expose business intelligence (e.g., order volume), can be vulnerable to enumeration attacks, and are problematic in distributed systems or data merging scenarios.

## Decision
We will use UUID v4 (Universally Unique Identifier) for all primary keys across the application database tables instead of auto-increment integers.

## Alternatives Considered
- Auto-increment Integers: Fast, but exposes ID sequences and prevents offline creation.
- ULID/Snowflake: Good alternatives that maintain sortability, but UUID v4 is standard, universally supported in Postgres, and sufficient for our scale.

## Consequences
### Positive
- Globally unique identifiers, safe for distributed creation.
- Prevents ID enumeration attacks (e.g., guessing `/orders/123`).
- Merge-safe for data imports/exports.

### Negative
- Slightly larger indexes and storage (16 bytes vs 8 bytes for bigint).
- Random distribution can cause index fragmentation in some databases, though Postgres handles UUIDs reasonably well.
- Less readable in URLs for debugging.

### Neutral
- Standard UUID v4 does not encode time, so temporal sorting requires a separate timestamp column.

## References
- [PostgreSQL UUID Type](https://www.postgresql.org/docs/current/datatype-uuid.html)
