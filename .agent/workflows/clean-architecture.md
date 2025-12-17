---
description: Panduan Clean Architecture untuk pengembangan Odyssey ERP
---

# Odyssey Clean Architecture Guide

## Prinsip Utama

### 1. MODULAR MONOLITH - Batas Domain Jelas
- **Setiap domain** memiliki package terpisah di `internal/<domain>`
- **Tidak ada dependensi silang** antar domain (gunakan `shared/`)
- Struktur folder domain:
  ```
  internal/<domain>/
  ├── domain.go      ← Entity/DTO + interfaces
  ├── service.go     ← Business logic
  ├── repository.go  ← Data access (sqlc)
  ├── handler.go     ← HTTP handlers
  └── *_test.go      ← Unit/integration tests
  ```

### 2. DEPENDENCY INVERSION - Interface di Domain
- **Service mendefinisikan interface** untuk repository
- **Repository implementasi** ditempatkan di file terpisah
- Contoh benar:
  ```go
  // domain.go
  type CustomerRepository interface {
      Get(ctx context.Context, id int64) (*Customer, error)
  }
  
  // service.go
  type Service struct {
      repo CustomerRepository  // Depend on interface
  }
  ```

### 3. LAYERED ARCHITECTURE - 4 Lapisan
```
┌─────────────────────────────────────────┐
│  HTTP Handler (thin, validasi input)    │
├─────────────────────────────────────────┤
│  Service (business logic, orkestrasi)   │
├─────────────────────────────────────────┤
│  Repository (data access, sqlc)         │
├─────────────────────────────────────────┤
│  Database (PostgreSQL)                  │
└─────────────────────────────────────────┘
```

## Struktur Domain

### File Wajib per Domain

| File | Isi | Contoh |
|------|-----|--------|
| `domain.go` | Entity, DTO, interfaces | `Customer`, `CustomerRepository` |
| `service.go` | Business logic | `CreateCustomer()`, `ApproveQuotation()` |
| `repository.go` | SQL implementation | `repo.GetByID()`, `repo.Create()` |
| `handler.go` | HTTP handlers | `handleList()`, `handleCreate()` |

### File Opsional

| File | Kapan Digunakan |
|------|-----------------|
| `db/` folder | Generated sqlc queries |
| `http/` folder | Jika handler kompleks |
| `export/` folder | PDF/CSV export logic |
| `fx/` folder | Domain-specific utilities |

## Dependency Flow

```
cmd/odyssey/main.go
    ↓
internal/app/router.go   ← Mount semua routes
    ↓
internal/<domain>/handler.go
    ↓ (validasi, parse input)
internal/<domain>/service.go
    ↓ (business logic)
internal/<domain>/repository.go
    ↓ (SQL queries)
PostgreSQL
```

## Naming Convention

### Package Names
```
internal/sales       ← Domain name (singular/plural OK)
internal/auth        ← Lower snake_case
internal/rbac        ← Akronim lowercase
```

### File Names
```
handler.go           ← Main handler file
handler_quotation.go ← Sub-handler (jika kompleks)
service.go           ← Main service
service_test.go      ← Unit tests
integration_test.go  ← Integration tests
```

### Function Names
```go
// Handler (HTTP verb prefix)
func (h *Handler) handleList(w, r)
func (h *Handler) handleCreate(w, r)
func (h *Handler) handleGet(w, r)
func (h *Handler) handleUpdate(w, r)
func (h *Handler) handleDelete(w, r)

// Service (business action)
func (s *Service) CreateCustomer(ctx, input)
func (s *Service) ApproveQuotation(ctx, id, approverID)
func (s *Service) CalculateTotals(items)

// Repository (CRUD prefix)
func (r *Repo) Get(ctx, id)
func (r *Repo) List(ctx, filter)
func (r *Repo) Create(ctx, entity)
func (r *Repo) Update(ctx, entity)
func (r *Repo) Delete(ctx, id)
```

## Shared Packages

### `internal/shared`
Untuk utilities yang dipakai banyak domain:
- `session.go` - Session management
- `csrf.go` - CSRF protection
- `respond.go` - HTTP response helpers
- `context.go` - Context helpers

### `internal/view`
Template rendering engine

### `internal/rbac`
Permission checking (dipakai semua protected handlers)

## Testing Pattern

### Unit Test (`*_test.go`)
```go
func TestService_CreateCustomer(t *testing.T) {
    // Arrange
    mockRepo := &MockRepository{}
    svc := NewService(mockRepo)
    
    // Act
    result, err := svc.CreateCustomer(ctx, input)
    
    // Assert
    require.NoError(t, err)
    assert.Equal(t, expected, result)
}
```

### Integration Test (`integration_test.go`)
```go
func TestHandler_Integration(t *testing.T) {
    // Setup real DB connection
    db := setupTestDB(t)
    defer cleanupTestDB(t, db)
    
    // Create real dependencies
    repo := NewRepository(db)
    svc := NewService(repo)
    handler := NewHandler(svc)
    
    // Test via HTTP
    req := httptest.NewRequest("POST", "/api/customers", body)
    rec := httptest.NewRecorder()
    handler.ServeHTTP(rec, req)
    
    assert.Equal(t, 201, rec.Code)
}
```

## Checklist Domain Baru

1. [ ] Buat folder `internal/<domain>/`
2. [ ] Definisikan entities di `domain.go`
3. [ ] Definisikan interfaces di `domain.go`
4. [ ] Implementasi repository di `repository.go`
5. [ ] Implementasi service di `service.go`
6. [ ] Implementasi handler di `handler.go`
7. [ ] Mount routes di `internal/app/router.go`
8. [ ] Tulis unit tests di `*_test.go`
9. [ ] Tulis integration tests di `integration_test.go`
10. [ ] Tambahkan RBAC permissions jika perlu

## Anti-Patterns (JANGAN Lakukan)

### ❌ Handler langsung akses DB
```go
// SALAH
func (h *Handler) handleCreate(w, r) {
    db.Exec("INSERT INTO customers...")  // Bypass service!
}
```

### ❌ Service return HTTP response
```go
// SALAH
func (s *Service) Create(w http.ResponseWriter) {
    w.WriteHeader(201)  // Service tidak boleh tahu HTTP!
}
```

### ❌ Import domain lain langsung
```go
// SALAH
import "internal/sales"  // Di package accounting

// BENAR: Gunakan shared types atau event
import "internal/shared"
```

### ❌ Business logic di handler
```go
// SALAH
func handleApprove(w, r) {
    if order.Status != "pending" { return }  // Logic di handler!
    if order.Total > user.Limit { return }
}

// BENAR
func handleApprove(w, r) {
    err := svc.ApproveOrder(ctx, orderID, userID)  // Delegate ke service
}
```
