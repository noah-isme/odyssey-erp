# ADR-006: Manual Dependency Injection

## Status
Accepted

## Date
2024-02-20

## Context
Dependency Injection (DI) is critical for testability and decoupling components. In the Go ecosystem, there is a debate between using manual DI (passing dependencies via constructors) and using automated DI frameworks (like Google's Wire or Uber's Dig). Go's philosophy generally favors explicit, readable code over "magic."

## Decision
We will use Manual Dependency Injection via constructor functions and central `Application` or module structs, avoiding any automated DI frameworks.

## Alternatives Considered
- Google Wire: Compile-time DI. Reduces boilerplate but adds a code generation step and learning curve.
- Uber Dig: Runtime DI. Uses reflection, can fail at runtime, and obscures the dependency graph.

## Consequences
### Positive
- Clear, explicit dependency graph visible in `main.go`.
- No extra build steps or reflection overhead.
- Easier for new Go developers to trace execution.

### Negative
- More wiring code and boilerplate to write when initializing the application.
- `main.go` can become large as the application grows.

### Neutral
- Grouping dependencies into coarse-grained structs can help manage the boilerplate.

## References
- [Go Dependency Injection without Frameworks](https://www.alexedwards.net/blog/organising-database-access)
