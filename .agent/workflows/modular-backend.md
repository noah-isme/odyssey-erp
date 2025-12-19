---
description: Panduan modular backend dengan subdirectory per entity
---

# Backend Modular Architecture

## Prinsip Utama

> **Handler bodoh, Service pintar, Repo fokus DB.**

### Batas Ukuran File
| Metrik | Batas Max | Tindakan |
|--------|-----------|----------|
| Lines of Code | 400 baris | Split per concern |
| Functions per file | 15 fungsi | Grup fungsi terkait ke file baru |

---

## Struktur Modular (Subdirectory per Entity)

### Target Structure
```
internal/masterdata/
├── companies/
│   ├── handler.go      ← HTTP layer (parse request, return response)
│   ├── routes.go       ← Mapping endpoint -> handler
│   ├── service.go      ← Business logic, validasi rules, orchestrasi
│   ├── repository.go   ← Query DB (CRUD, search, pagination)
│   ├── model.go        ← Entity/domain struct (representasi data inti)
│   ├── dto.go          ← Request/response struct (yang keluar masuk API)
│   └── validation.go   ← Validasi input
├── branches/
│   ├── handler.go
│   ├── routes.go
│   ├── service.go
│   ├── repository.go
│   ├── model.go
│   ├── dto.go
│   └── validation.go
├── warehouses/
├── products/
├── suppliers/
├── categories/
├── units/
├── taxes/
└── shared/
    ├── errors.go       ← Custom errors
    ├── constants.go    ← Shared constants
    └── mapper.go       ← Shared mappers
```

### Kenapa Struktur Ini?
1. `internal/masterdata/` jadi "kota besar", tiap entity jadi "rumah"
2. Tim bisa kerja paralel tanpa merge conflict
3. Kalau nanti Master Data meledak (import, bulk update, audit log), tetap rapi

---

## Peran File (Biar Gak Rancu)

| File | Tanggung Jawab |
|------|----------------|
| `handler.go` | HTTP layer. Parse request, panggil service, return response |
| `service.go` | Business logic/usecase. Validasi rules, orchestrasi, transaksi |
| `repository.go` | Query DB (CRUD, search, pagination) |
| `model.go` | Entity/domain struct (representasi data inti) |
| `dto.go` | Request/response struct (yang keluar masuk API) |
| `validation.go` | Validasi input (manual atau pakai validator) |
| `routes.go` | Mapping endpoint -> handler |

---

## Contoh Implementasi

### model.go
```go
package companies

type Company struct {
    ID     string
    Code   string
    Name   string
    Active bool
}
```

### dto.go
```go
package companies

type CreateCompanyRequest struct {
    Code string `json:"code"`
    Name string `json:"name"`
}

type CompanyResponse struct {
    ID     string `json:"id"`
    Code   string `json:"code"`
    Name   string `json:"name"`
    Active bool   `json:"active"`
}
```

### repository.go
```go
package companies

import "context"

type Repository interface {
    Create(ctx context.Context, c Company) (Company, error)
    GetByID(ctx context.Context, id string) (Company, error)
    List(ctx context.Context, limit, offset int) ([]Company, error)
}

type repository struct {
    db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) Repository {
    return &repository{db: db}
}

func (r *repository) Create(ctx context.Context, c Company) (Company, error) {
    // INSERT query...
}

func (r *repository) GetByID(ctx context.Context, id string) (Company, error) {
    // SELECT query...
}

func (r *repository) List(ctx context.Context, limit, offset int) ([]Company, error) {
    // SELECT with LIMIT OFFSET...
}
```

### service.go
```go
package companies

import (
    "context"
    "errors"
    "strings"
)

type Service struct {
    repo Repository
}

func NewService(repo Repository) *Service {
    return &Service{repo: repo}
}

func (s *Service) Create(ctx context.Context, req CreateCompanyRequest) (Company, error) {
    req.Code = strings.TrimSpace(req.Code)
    req.Name = strings.TrimSpace(req.Name)

    if req.Code == "" || req.Name == "" {
        return Company{}, errors.New("code and name are required")
    }

    c := Company{
        ID:     "", // biasanya di-generate repo/DB
        Code:   req.Code,
        Name:   req.Name,
        Active: true,
    }

    return s.repo.Create(ctx, c)
}
```

### handler.go
```go
package companies

import (
    "encoding/json"
    "net/http"
)

type Handler struct {
    svc *Service
}

func NewHandler(svc *Service) *Handler {
    return &Handler{svc: svc}
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
    var req CreateCompanyRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, "invalid json", http.StatusBadRequest)
        return
    }

    company, err := h.svc.Create(r.Context(), req)
    if err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }

    resp := CompanyResponse{
        ID:     company.ID,
        Code:   company.Code,
        Name:   company.Name,
        Active: company.Active,
    }

    w.Header().Set("Content-Type", "application/json")
    _ = json.NewEncoder(w).Encode(resp)
}
```

### routes.go
```go
package companies

import "github.com/go-chi/chi/v5"

func (h *Handler) MountRoutes(r chi.Router) {
    r.Get("/", h.List)
    r.Post("/", h.Create)
    r.Get("/{id}", h.GetByID)
    r.Put("/{id}", h.Update)
    r.Delete("/{id}", h.Delete)
}
```

---

## Transaksi & Relasi Lintas Entity

Karena Branch tergantung Company, dan Warehouse tergantung Branch:

### Cara 1: Inject Repo Lintas Module (Simple, OK untuk Monolith)
```go
// branches/service.go
type Service struct {
    repo        Repository
    companyRepo companies.Repository  // inject dari luar
}
```

### Cara 2: Port Interface Kecil (Lebih Rapi)
```go
// branches/service.go
type CompanyReader interface {
    GetByID(ctx context.Context, id string) (companies.Company, error)
}

type Service struct {
    repo          Repository
    companyReader CompanyReader  // implementasi di-wire saat bootstrap
}
```

---

## Naming Convention

| Item | Convention | Contoh |
|------|------------|--------|
| Folder | **plural** | `companies/`, `branches/` |
| Package | **plural** | `package companies` |
| Struct | **singular** | `type Company struct` |

Biar kebaca natural: `companies.NewService(...)`, `companies.Company{}`

---

## Shared Folder

Untuk kode yang dipakai bersama antar entity:

```
internal/masterdata/shared/
├── errors.go      ← ErrNotFound, ErrDuplicate, etc
├── constants.go   ← Status codes, default limits
├── mapper.go      ← Shared mapping utilities
└── pagination.go  ← ListFilters, pagination helpers
```

```go
// shared/errors.go
package shared

import "errors"

var (
    ErrNotFound   = errors.New("resource not found")
    ErrDuplicate  = errors.New("duplicate entry")
    ErrValidation = errors.New("validation failed")
)
```

---

## Langkah Refactoring

### 1. Buat Struktur Folder
```bash
mkdir -p internal/masterdata/{companies,branches,warehouses,products,suppliers,categories,units,taxes,shared}
```

### 2. Untuk Setiap Entity, Buat File
```bash
touch internal/masterdata/companies/{handler,routes,service,repository,model,dto,validation}.go
```

### 3. Pindahkan Kode
1. Entity struct → `model.go`
2. Request/Response struct → `dto.go`
3. Repository interface + impl → `repository.go`
4. Service logic → `service.go`
5. Handler methods → `handler.go`
6. Route mounting → `routes.go`
7. Validation helper → `validation.go`

### 4. Update Imports
Sesuaikan import path ke struktur baru.

### 5. Verifikasi Build
// turbo
```bash
go build -v ./...
```

---

## Aturan Ketat

### WAJIB
1. **Satu entity = satu folder** dengan file terpisah per concern
2. **File < 400 LOC** kecuali ada alasan kuat
3. **Konsisten** - semua entities ikut pola yang sama
4. **Shared code di `shared/`** - errors, constants, mappers

### JANGAN
1. **JANGAN** campur entity di satu folder
2. **JANGAN** campur repository dan handler di satu file
3. **JANGAN** duplikasi helper functions (taruh di shared)
4. **JANGAN** hardcode values (taruh di constants)

---

## Kapan Pakai `/pkg`?

- Kalau ada modul yang memang **reusable lintas project**
- Kalau cuma dipakai internal app, taruh di `/internal` saja
- `/pkg` sering jadi tempat "barang numpuk" kalau tidak disiplin

---

## Quick Reference

| Situasi | Tindakan |
|---------|----------|
| File > 400 LOC | Split per concern |
| Multiple entities in folder | Buat subfolder per entity |
| Shared logic | Taruh di `shared/` |
| Cross-entity dependency | Inject via interface |
| Request/Response types | Taruh di `dto.go` |
| Validation rules | Taruh di `validation.go` |
