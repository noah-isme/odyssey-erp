# ADR-001: Modular Monolith over Microservices

## Status
Accepted

## Date
2024-01-10

## Context
Odyssey ERP targets the SMB market. The system needs to be simple to deploy, maintain, and operate. While microservices offer independent scalability, the operational overhead, network latency, and deployment complexity are significant. The system's scale for our target market does not require independent scaling of different modules.

## Decision
We will use a Modular Monolith architecture. The application will be compiled into a single Go binary, with clear module boundaries defined under the `internal/` package structure.

## Alternatives Considered
- Microservices Architecture: Rejected due to operational complexity and unnecessary scaling overhead for our target market.
- Traditional Monolith without clear module boundaries: Rejected due to long-term maintainability issues and tight coupling ("big ball of mud").

## Consequences
### Positive
- Simpler deployment (single binary).
- Shared database, eliminating distributed transaction complexities.
- Faster development and debugging experience.
- Refactoring across modules is easier.

### Negative
- Harder to scale modules independently.
- Any module crashing brings down the entire application.

### Neutral
- Requires strict discipline to maintain module boundaries in code.

## References
- [Majestic Monolith](https://m.signalvnoise.com/the-majestic-monolith/)
- [Modular Monoliths in Go](https://threedots.tech/post/microservices-or-monolith-its-detail/)
