# Arsitektur Odyssey ERP

## Prinsip Utama

- **Monolith modular** dengan batas domain pada package `internal/<domain>`.
- **Clean architecture**: handler HTTP tipis, service domain berisi logika bisnis, repository menangani akses data.
- **SSR-first**: semua UI dirender server menggunakan `html/template`.
- **Keamanan default-on**: middleware request ID, recover, rate limit, secure headers, CSRF, session HttpOnly.

## Lapisan

1. **Interface**: handler (`internal/app`, `internal/auth`, `report`, `jobs`) menerima request, validasi input, panggil service, render view.
2. **Service**: modul domain (`internal/auth/service.go`) mengatur aturan bisnis, orkestrasi repository + komponen lain.
3. **Repository**: repository packages call generated `internal/sqlc` bindings for type-safe PostgreSQL queries.
4. **Infra**: config, logger, router, session, CSRF, view engine, job worker, Gotenberg client.

## Dependency Flow

```
cmd/* -> internal/app -> domain handler -> service -> repository/sqlc
                                |-> shared (session, csrf, respond)
                                |-> view (templates)
```

Modules may use `internal/shared` for cross-cutting helpers, `jobs` for background
work, and `report` for PDF integration. Cross-module integration adapters live in
`internal/integration` where needed.

## Deployment

- **App server** (`cmd/odyssey`) untuk HTTP.
- **Worker** (`cmd/worker`) menjalankan Asynq.
- **Docker Compose** menyatukan Postgres, Redis, Mailpit, Gotenberg.
