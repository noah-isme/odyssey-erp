# ADR-002: Server-Side Rendering over SPA

## Status
Accepted

## Date
2024-01-15

## Context
Building modern web applications often involves Single Page Application (SPA) frameworks (React, Vue, Angular). However, these introduce significant frontend complexity, require API versioning to support older clients, and can suffer from slower initial page loads due to large JavaScript bundles. For an ERP system where SEO is less critical but fast, reliable interactions and simple state management are key, we need an efficient approach.

## Decision
We will use Server-Side Rendering (SSR) with Go templates and HTMX instead of a heavy SPA framework. 

## Alternatives Considered
- React/Vue/Angular SPAs: Rejected due to increased complexity, build steps, and API synchronization overhead.
- Traditional SSR (full page reloads only): Rejected due to poor user experience compared to modern web apps.

## Consequences
### Positive
- Simpler technology stack (no complex node/npm build pipelines).
- Fewer dependencies to maintain.
- Faster initial page loads.
- Eliminates the need for a separate API for the frontend.

### Negative
- Less rich client-side interactivity compared to SPAs.
- Server must handle all rendering, potentially increasing CPU load on the backend.

### Neutral
- Developers must learn HTMX paradigm instead of traditional React/Vue patterns.

## References
- [HTMX Documentation](https://htmx.org/docs/)
- [Go html/template](https://pkg.go.dev/html/template)
