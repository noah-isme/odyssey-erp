# Arsitektur Odyssey ERP

## Prinsip Utama

- **Monolith modular** dengan batas domain pada package `internal/<domain>`.
- **Clean architecture**: handler HTTP tipis, service domain berisi logika bisnis, repository menangani akses data.
- **SSR-first**: semua UI dirender server menggunakan `html/template`.
- **Keamanan default-on**: middleware request ID, recover, rate limit, secure headers, CSRF, session HttpOnly.

## Lapisan

1. **Interface**: handler (`internal/app`, `internal/auth`, `report`, `jobs`) menerima request, validasi input, panggil service, render view.
2. **Service**: modul domain (`internal/auth/service.go`) mengatur aturan bisnis melalui repository-owned interfaces dan komponen lain.
3. **Repository**: repository packages own persistence contracts, call generated `internal/sqlc` bindings, and map database-specific values to domain-owned types.
4. **Infra**: config, logger, router, session, CSRF, view engine, job worker, Gotenberg client.

## Dependency Flow

```
cmd/* -> internal/app -> domain handler -> service -> repository/sqlc
                                |-> shared (session, csrf, respond)
                                |-> view (templates)
```

The concrete database direction is:

```text
handler -> service -> repository interface -> PostgreSQL adapter -> sqlc/pgx
```

`internal/sqlc` and `pgtype` are persistence implementation details. They must
not appear in service, domain, or handler method signatures. Composition roots
may construct adapters, while repositories are responsible for translating
generated rows, nullable wrappers, and database enums into package-owned
contracts. This keeps a database representation change from propagating across
the application layer.

## HTTP error boundary

HTTP translation is centralized in `internal/shared/http.go`:

- `HTTPStatus` maps wrapped shared domain errors to status codes.
- `WriteError` and `WriteErrorStatus` emit safe plain-text responses.
- `RespondJSONError` and `JSONErrorFrom` emit safe JSON responses.
- `UserSafeMessage` prevents database, driver, and infrastructure details from
  reaching users; handlers should log the original error separately.

`internal/platform/httpx` uses the same classification for RFC 7807 responses.
Module-specific JSON envelopes may remain where they are part of an existing API
contract, but their internal error message must use the shared safe-message
policy.

Modules may use `internal/shared` for cross-cutting helpers, `jobs` for background
work, and `report` for PDF integration. Cross-module integration adapters live in
`internal/integration` where needed.

## Deployment

- **App server** (`cmd/odyssey`) untuk HTTP.
- **Worker** (`cmd/worker`) menjalankan Asynq.
- **Docker Compose** menyatukan Postgres, Redis, Mailpit, Gotenberg.
