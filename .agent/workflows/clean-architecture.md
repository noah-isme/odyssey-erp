---
description: Panduan Clean Architecture untuk pengembangan Odyssey ERP
---

# Odyssey Clean Architecture Guide

## Prinsip Utama

### 1. MODULAR MONOLITH - Batas Domain Jelas
- **Setiap domain** memiliki package terpisah di `internal/<domain>`
- **Tidak ada dependensi silang** antar domain (gunakan `platform/`)
- Struktur folder:
  ```
  internal/
  ├── platform/           ← Infrastruktur (shared)
  │   ├── db/
  │   ├── cache/
  │   └── httpx/
  ├── sqlc/               ← Centralized sqlc output
  └── <domain>/
      └── <entity>/       ← 7-file pattern
  ```

### 2. DEPENDENCY INVERSION - Interface di Domain
```go
// repository.go - Interface defined in domain
type Repository interface {
    Get(ctx context.Context, id int64) (*Company, error)
}

// service.go - Depend on interface
type Service struct {
    repo Repository
}
```

### 3. LAYERED ARCHITECTURE
```
HTTP Handler (thin, validasi input)
    ↓
Service (business logic, orkestrasi)
    ↓
Repository (wrap sqlc)
    ↓
internal/sqlc (generated)
    ↓
PostgreSQL
```

---

## Struktur Proyek

```
project-root/
├── cmd/odyssey/main.go
├── internal/
│   ├── platform/          ← Infrastruktur
│   │   ├── db/            ← postgres.go, tx.go
│   │   ├── cache/         ← redis.go
│   │   └── httpx/         ← respond.go, errors.go, middleware.go
│   ├── sqlc/              ← Centralized generated code
│   │   ├── db.go
│   │   ├── models.go
│   │   └── *.sql.go
│   └── <domain>/
│       ├── <domain>_routes.go
│       └── <entity>/
├── sql/
│   └── queries/           ← sqlc query files
├── migrations/            ← DDL for migration tool
└── sqlc.yaml
```

> [!IMPORTANT]
> **SQLC centralized** ke `internal/sqlc/`. Repository di domain wrap sqlc queries.

---

## Platform Layer (Build First!)

| File | Purpose |
|------|---------|
| `db/postgres.go` | Connection pool |
| `db/tx.go` | `WithTx()` transaction helper |
| `cache/redis.go` | Redis client |
| `httpx/respond.go` | RFC7807 `Problem()`, `JSON()` |
| `httpx/errors.go` | Sentinel errors + `RespondError()` |
| `httpx/middleware.go` | Logging, request-id, trace_id |

---

## SQLC Configuration

```yaml
# sqlc.yaml
version: "2"
sql:
  - engine: "postgresql"
    schema: "migrations"        # Point to existing migrations
    queries: "sql/queries"      # Centralized queries
    gen:
      go:
        out: "internal/sqlc"
        package: "sqlc"
        sql_package: "pgx/v5"
        emit_json_tags: true
```

> [!NOTE]
> Migration tool tetap happy, sqlc baca DDL dari `migrations/`.

---

## File per Entity (7-File Pattern)

| File | Isi |
|------|-----|
| `model.go` | Domain entity |
| `dto.go` | Request/Response struct |
| `validation.go` | Input validation |
| `repository.go` | Interface + wrap sqlc |
| `service.go` | Business logic |
| `handler.go` | HTTP handlers |
| `routes.go` | Endpoint mapping |

---

## Dependency Flow

```
cmd/odyssey/main.go
    ↓
internal/platform/db          ← Pool init
    ↓
internal/sqlc                 ← sqlc.New(pool)
    ↓
internal/<domain>_routes.go   ← Wire handlers
    ↓
internal/<domain>/<entity>/   ← handler → service → repository
```

---

## Anti-Patterns

| ❌ Don't | ✅ Do |
|---------|-------|
| Handler akses DB langsung | Handler → Service → Repo |
| Service return http.ResponseWriter | Service return error, Handler map ke HTTP |
| Raw SQL di repository | Wrap sqlc queries |
| Import sqlc di handler/service | Import sqlc hanya di repository |

---

## Quick Reference

| Layer | Location |
|-------|----------|
| Migrations | `migrations/` |
| SQL Queries | `sql/queries/` |
| Generated | `internal/sqlc/` |
| Platform | `internal/platform/` |
| Domain | `internal/<domain>/<entity>/` |
