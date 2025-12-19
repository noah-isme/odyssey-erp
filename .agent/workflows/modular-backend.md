---
description: Panduan modular backend dengan subdirectory per entity
---

# Backend Modular Architecture

## Prinsip Utama

> **Handler bodoh, Service pintar, Repo fokus wrap sqlc.**

### Batas Ukuran File
| Metrik | Batas Max | Tindakan |
|--------|-----------|----------|
| Lines of Code | 400 baris | Split per concern |
| Functions per file | 15 fungsi | Grup ke file baru |

---

## Struktur Proyek

```
internal/
  platform/
    db/
      postgres.go           ← Pool init
      tx.go                 ← WithTx helper
    cache/
      redis.go              ← Redis client
    httpx/
      middleware.go         ← Logging, request-id, trace_id
      respond.go            ← RFC7807 problem+json
      errors.go             ← Sentinel errors + mapping
  sqlc/                     ← Centralized generated code
    db.go
    models.go
    companies.sql.go
    branches.sql.go
  <domain>/
    <domain>_routes.go      ← Wire all entities
    <entity>/
      handler.go
      routes.go
      service.go
      repository.go
      model.go
      dto.go
      validation.go

sql/
  queries/
    companies.sql           ← sqlc query definitions
    branches.sql

migrations/                 ← DDL for migration tool (goose/flyway)
  000001_init.up.sql
  000001_init.down.sql
```

### Kenapa Struktur Ini?
1. **Migration tool tetap happy** - `migrations/` tidak diubah
2. **SQLC centralized** - satu package, mudah wiring tx
3. **Platform dulu** - fondasi standar sebelum refactor domain
4. **Repository wrap sqlc** - domain tetap bersih

---

## SQLC Configuration

```yaml
# sqlc.yaml
version: "2"
sql:
  - engine: "postgresql"
    schema: "migrations"       # ← Existing migrations folder
    queries: "sql/queries"     # ← Centralized queries
    gen:
      go:
        out: "internal/sqlc"
        package: "sqlc"
        sql_package: "pgx/v5"
        emit_json_tags: true
```

> [!NOTE]
> sqlc baca DDL dari `migrations/` tanpa mengganggu migration tool.

---

## Peran File

### Platform Layer

| File | Tanggung Jawab |
|------|----------------|
| `db/postgres.go` | Connection pool, health check |
| `db/tx.go` | `WithTx()` transaction wrapper |
| `cache/redis.go` | Redis client + helpers |
| `httpx/respond.go` | `JSON()`, `Problem()` RFC7807 |
| `httpx/errors.go` | Sentinel errors, `RespondError()` |
| `httpx/middleware.go` | Logging, request-id, trace_id |

### Entity Layer (7-File Pattern)

| File | Tanggung Jawab |
|------|----------------|
| `model.go` | Domain entity struct |
| `dto.go` | Request/response struct |
| `validation.go` | Input validation rules |
| `repository.go` | Interface + wrap sqlc |
| `service.go` | Business logic, orchestrasi |
| `handler.go` | Parse request, call service, respond |
| `routes.go` | Endpoint → handler mapping |

---

## Contoh Implementasi

### sql/queries/companies.sql
```sql
-- name: GetCompany :one
SELECT id, code, name, active, created_at, updated_at
FROM companies WHERE id = $1;

-- name: ListCompanies :many
SELECT id, code, name, active, created_at, updated_at
FROM companies ORDER BY name LIMIT $1 OFFSET $2;

-- name: CreateCompany :one
INSERT INTO companies (code, name, active)
VALUES ($1, $2, $3)
RETURNING *;
```

### repository.go
```go
package companies

import (
    "context"
    "odyssey/internal/sqlc"
)

type Repository interface {
    Create(ctx context.Context, c Company) (Company, error)
    GetByID(ctx context.Context, id int64) (Company, error)
}

type repository struct {
    q *sqlc.Queries
}

func NewRepository(q *sqlc.Queries) Repository {
    return &repository{q: q}
}

func (r *repository) GetByID(ctx context.Context, id int64) (Company, error) {
    row, err := r.q.GetCompany(ctx, id)
    if err != nil {
        return Company{}, err
    }
    return mapFromSqlc(row), nil
}
```

### handler.go
```go
package companies

import (
    "net/http"
    "odyssey/internal/platform/httpx"
)

type Handler struct {
    svc *Service
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
    var req CreateCompanyRequest
    if err := httpx.DecodeJSON(r, &req); err != nil {
        httpx.Problem(w, http.StatusBadRequest, "Invalid JSON", err.Error())
        return
    }

    company, err := h.svc.Create(r.Context(), req)
    if err != nil {
        httpx.RespondError(w, err)
        return
    }

    httpx.JSON(w, http.StatusCreated, toResponse(company))
}
```

### masterdata_routes.go (Domain Level)
```go
package masterdata

import (
    "github.com/go-chi/chi/v5"
    "odyssey/internal/sqlc"
    "odyssey/internal/masterdata/companies"
)

func MountRoutes(r chi.Router, q *sqlc.Queries) {
    companyRepo := companies.NewRepository(q)
    companySvc := companies.NewService(companyRepo)
    companyHandler := companies.NewHandler(companySvc)

    r.Route("/companies", func(r chi.Router) {
        companyHandler.MountRoutes(r)
    })
}
```

---

## Urutan Refactoring

### Phase 1: Platform (Fondasi)
```bash
mkdir -p internal/platform/{db,cache,httpx}
mkdir -p sql/queries
mkdir -p internal/sqlc
```

1. `httpx/respond.go` - RFC7807 problem+json
2. `httpx/errors.go` - Sentinel errors + RespondError
3. `db/tx.go` - Transaction helper
4. `httpx/middleware.go` - Logging, request-id

### Phase 2: SQLC Setup
1. Create `sql/queries/*.sql`
2. Update `sqlc.yaml` (centralized)
3. Run `sqlc generate`

### Phase 3: Vertical Slice (1 Domain)
1. Pick one domain (e.g., `masterdata`)
2. Implement full pattern: routes → handler → service → repo
3. Test end-to-end

### Phase 4: Copy Pattern
1. Apply to other domains
2. Refactor large files (> 400 LOC)

---

## Aturan Ketat

### WAJIB
1. **SQLC centralized** di `internal/sqlc/`
2. **Repository wrap sqlc** - jangan import sqlc di handler/service
3. **Platform dulu** - sebelum refactor domain
4. **Domain routes file** - `<domain>_routes.go` sebagai wiring

### JANGAN
1. **JANGAN** raw SQL di repository
2. **JANGAN** import sqlc di handler atau service
3. **JANGAN** file > 400 LOC
4. **JANGAN** skip platform layer

---

## Quick Reference

| Layer | Location | Purpose |
|-------|----------|---------|
| Migrations | `migrations/` | DDL for migration tool |
| SQL Queries | `sql/queries/` | sqlc query definitions |
| Generated | `internal/sqlc/` | Centralized sqlc output |
| Platform | `internal/platform/` | Infrastructure |
| Domain | `internal/<domain>/` | Business domains |
| Entity | `internal/<domain>/<entity>/` | 7-file pattern |
