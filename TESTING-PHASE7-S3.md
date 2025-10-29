# Testing Plan — Phase 7 Sprint 3

This document captures the consolidated finance testing strategy for Phase 7 Sprint 3.

## Automated coverage

### Consolidated Profit & Loss / Balance Sheet

- View-model caching (TTL 5 minutes) verified via handler unit tests that assert repeated SSR requests reuse the cached payload without re-invoking the domain services.
- FX policy fallback paths ensure missing rates downgrade FX usage, surface `FX rate missing …` warnings, and keep warnings visible across SSR/CSV responses.
- CSV exports enforce MIME `text/csv`, propagate FX warnings through the `X-Consol-Warning` header, and require `finance.export_consolidation` permission.
- PDF exports return `503 Service Unavailable` while the stub exporter reports `Ready() == false`.
- Export endpoints are rate limited to 10 req/min per user; the 11th sequential call returns HTTP 429.
- Cache busting helper is exercised to guarantee `BustConsolViewCache()` wipes memoized view-models.

### Job integration

- `jobs/consolidate_refresh` now calls `consolhttp.BustConsolViewCache()` after successful refresh to invalidate cached SSR payloads for subsequent requests.

## Test commands

- `go test ./internal/consol/http` — validates handler caching, FX warnings, RBAC enforcement, rate-limit behaviour, and PDF stubs.
- `go test ./internal/consol/...` — regression suite for FX policy, service aggregations, and HTTP endpoints.

## Performance checkpoints

- Cache hit tests assert no additional service invocations, supporting the ≤600ms cached response target.
- Cold-path behaviour exercised through full service invocation to confirm correctness before caching.
