# ADR-003: sqlc over ORM

## Status
Accepted

## Date
2024-01-20

## Context
Database interactions in Go can be handled via standard `database/sql`, ORMs (like GORM or Ent), or code generators. ORMs offer convenience but often obscure the generated SQL, use reflection which affects performance, and can lead to N+1 query problems. For an ERP, precise control over SQL queries is crucial for performance and complex data reporting.

## Decision
We will use `sqlc` to generate type-safe Go code from plain SQL queries instead of using an ORM.

## Alternatives Considered
- GORM: Popular Go ORM, but relies heavily on reflection and can generate inefficient queries.
- Ent: Type-safe ORM from Facebook, but introduces its own complex DSL and graph concepts.
- Raw `database/sql`: Too much boilerplate and lack of compile-time type safety.

## Consequences
### Positive
- Better query performance (no reflection overhead).
- Explicit, predictable SQL execution.
- Compile-time checking of SQL queries against the database schema.

### Negative
- More boilerplate for simple CRUD operations.
- Requires writing explicit SQL for every query.

### Neutral
- Forces developers to understand SQL deeply.

## References
- [sqlc Documentation](https://docs.sqlc.dev/en/latest/)
