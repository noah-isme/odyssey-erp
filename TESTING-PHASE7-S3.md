# Testing Plan — Phase 7 Sprint 3

This document captures the consolidated finance testing strategy for Phase 7 Sprint 3.

## Automated coverage

### Consolidated Profit & Loss / Balance Sheet

- View-model caching (TTL 5 minutes) verified via handler unit tests that assert repeated SSR requests reuse the cached payload without re-invoking the domain services.
- FX policy fallback paths ensure missing rates downgrade FX usage, surface `FX rate missing …` warnings, and keep warnings visible across SSR/CSV responses.
- CSV exports enforce MIME `text/csv`, propagate FX warnings through the `X-Consol-Warning` header, and require `finance.export_consolidation` permission.
- CSV writer streams through a 32 KiB buffer, flushing every 200 rows, and prefixes comment metadata (`# …`) for report filters and warnings so the files remain Excel/LibreOffice friendly.
- PDF exports return `503 Service Unavailable` while the stub exporter reports `Ready() == false`.
- Production PDF exporter (tag `prod`) retries two times on 5xx responses, enforces a 10s timeout, validates payload size ≥1 KiB, and raises `TimeoutError`, `InvalidResponse`, or `TooSmallError` sentinels for observability.
- View-model warnings are cached alongside SSR payloads and rendered consistently across HTML banners, CSV metadata, and PDF warning lists (FX gaps, unbalanced balance sheet, truncated entity filters).
- Export endpoints are rate limited to 10 req/min per user; the 11th sequential call returns HTTP 429.
- Cache busting helper is exercised to guarantee `BustConsolViewCache()` wipes memoized view-models.

### Job integration

- `jobs/consolidate_refresh` now calls `consolhttp.BustConsolViewCache()` after successful refresh to invalidate cached SSR payloads for subsequent requests.

## Test commands

- `go test ./internal/consol/http` — validates handler caching, FX warnings, RBAC enforcement, rate-limit behaviour, and PDF stubs.
- `go test -tags=prod ./internal/consol/http` — exercises the Gotenberg client, retry/timeout logic, and PDF size guardrails under the `prod` build tag.
- `go test ./internal/consol/...` — regression suite for FX policy, service aggregations, and HTTP endpoints.

## Final review status

- Manual QA validated SSR warning banners against CSV metadata headers and PDF caption lists using seeded finance groups.
- CSV and PDF exports executed via `make export-demo` with RBAC-protected credentials; rate-limit guard returned HTTP 429 on the 11th rapid request.
- Runbook (`docs/runbook-consol-plbs.md`) and release artefacts reviewed with observability, FX, and cache refresh procedures in scope. No open TODO/FIXME items remain for Phase 7 code paths.

## Performance checkpoints

- Cache hit tests assert no additional service invocations, supporting the ≤600ms cached response target.
- Cold-path behaviour exercised through full service invocation to confirm correctness before caching.
