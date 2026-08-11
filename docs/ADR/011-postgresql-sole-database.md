# ADR-011: PostgreSQL as Sole Database

## Status
Accepted

## Date
2024-04-18

## Context
Financial data needs ACID guarantees, rich query capabilities, and a mature ecosystem for extensions and tooling.

## Decision
We will use PostgreSQL 17 as our sole and exclusive database. We will not support multi-database capability or abstract our queries to support other RDBMS systems.

## Alternatives
- MySQL
- SQLite
- CockroachDB

## Consequences
### Positive
- Full utilization of Postgres-specific features (e.g., advanced JSONB, arrays, CTEs)
- Simpler operations, deployments, and testing
### Negative
- Limits user choice for self-hosting environments that prefer other databases
