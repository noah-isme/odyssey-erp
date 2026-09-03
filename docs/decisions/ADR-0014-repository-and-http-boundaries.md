# ADR-0014: Repository-Owned Persistence and HTTP Error Boundaries

## Status

Accepted

## Context

Generated SQLC models, `pgtype` nullable wrappers, and repeated handler-level
error translation had spread implementation details across application code.
That made database representation changes expensive and caused different
handlers to expose different status codes or internal error messages.

## Decision

### Persistence boundary

- Services and domains depend on package-owned repository interfaces.
- PostgreSQL repository adapters are the only application-layer owners of
  `internal/sqlc`, `pgtype`, generated enum values, and SQL query parameters.
- Adapters map generated rows to local domain/read-model types and map local
  inputs back to SQLC parameters.
- Composition roots may construct repository adapters, but must not inject raw
  `*sqlc.Queries` into services, handlers, or jobs that expose business ports.
- Background jobs and presentation adapters may use database infrastructure only
  when they are explicitly acting as infrastructure adapters; database types
  must not become business-layer contracts.

### HTTP error boundary

- Shared domain sentinels are classified by `shared.HTTPStatus`.
- Plain-text handlers use `shared.WriteError` or `shared.WriteErrorStatus`.
- JSON handlers use `shared.RespondJSONError` or `shared.JSONErrorFrom`.
- SSR redirects and form state use `shared.UserSafeMessage`.
- The original error may be logged with context, but `err.Error()` is never sent
  directly to a client.
- Existing endpoint-specific JSON envelopes may remain stable while delegating
  safe-message translation to the shared policy.

## Consequences

- Service and handler tests can use local fakes without importing SQLC.
- SQLC regeneration or PostgreSQL nullability changes are isolated to adapters.
- SSR, JSON, and RFC 7807 endpoints share a predictable safety policy.
- Some repository adapters are more explicit because they now contain mapping
  code; this is intentional boundary ownership rather than duplicated domain
  logic.

## Verification

The boundary is checked by compiling all internal packages and running `go vet
./...`. Focused tests cover shared status classification and the guarantee that
unknown infrastructure errors are not serialized to clients.

## References

- [Handler Guidelines](../guides/handlers.md)
- [Architecture Overview](../architecture/arsitektur.md)
- [Shared HTTP boundary](../../internal/shared/http.go)
- [Platform HTTP errors](../../internal/platform/httpx/errors.go)
