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

---

## Struktur Backend (Go)

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

---

## Struktur Frontend (JavaScript)

### 4-File Pattern untuk Features

```
web/static/js/
├── core/
│   └── ui.js         ← Critical theme restore only
├── features/
│   └── <feature>/
│       ├── store.js   ← State + Reducer + Selectors
│       ├── effects.js ← Side effects (fetch, timer, focus)
│       ├── view.js    ← DOM rendering
│       └── index.js   ← Mount + Event delegation + Public API
├── components/
│   └── <simple>.js   ← Simple presentational components
└── main.js           ← Entry point
```

### Responsibility per File

| File | Responsibility | Pure? |
|------|---------------|-------|
| `store.js` | State, Reducer, Selectors | ✅ Yes |
| `effects.js` | Fetch, timer, localStorage, focus | ❌ Side effects |
| `view.js` | DOM rendering, cache nodes | ❌ Side effects |
| `index.js` | Event delegation, init/destroy | ❌ Orchestration |

### Kapan 4-File vs 1-File?

| Struktur | Gunakan Untuk |
|----------|---------------|
| 4-file | Complex state, async, lifecycle needed |
| 1-file | Simple, stateless, no async |

---

## Dependency Flow

### Backend
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

### Frontend
```
main.js (entry point)
    ↓
features/<feature>/index.js   ← init(), destroy()
    ↓
Event → dispatch(action)
    ↓
features/<feature>/store.js   ← reducer(state, action)
    ↓
features/<feature>/effects.js ← side effects
    ↓
features/<feature>/view.js    ← render(state)
    ↓
DOM
```

---

## Naming Convention

### Backend (Go)

```
internal/sales       ← Domain name (singular/plural OK)
internal/auth        ← Lower snake_case
internal/rbac        ← Akronim lowercase
```

```go
// Handler (HTTP verb prefix)
func (h *Handler) handleList(w, r)
func (h *Handler) handleGet(w, r)

// Service (business action)
func (s *Service) CreateCustomer(ctx, input)
func (s *Service) ApproveQuotation(ctx, id, approverID)

// Repository (CRUD prefix)
func (r *Repo) Get(ctx, id)
func (r *Repo) Create(ctx, entity)
```

### Frontend (JavaScript)

```
features/form/       ← feature name lowercase
features/combobox/   ← compound words no separator
features/table-edit/ ← hyphenated if needed
```

```javascript
// Store - Action types (UPPERCASE_SNAKE)
'FORM_SET_VALUE'
'MODAL_OPEN'
'COMBOBOX_SELECT'

// Store - State keys (camelCase)
{ isOpen, selectedValue, highlightIndex }

// Effects - Methods (camelCase)
effects.focusFirst(el)
effects.lockScroll()

// View - Render methods (render prefix)
view.render(id, state)
view.renderError(id, error)
```

---

## Shared Packages

### Backend: `internal/shared`
- `session.go` - Session management
- `csrf.go` - CSRF protection
- `respond.go` - HTTP response helpers
- `context.go` - Context helpers

### Frontend: `js/core`
- `ui.js` - Critical theme restore
- `toast.js` - Loading helper (legacy)
- `shortcuts.js` - Keyboard shortcuts

---

## Testing Pattern

### Backend: Unit Test
```go
func TestService_CreateCustomer(t *testing.T) {
    mockRepo := &MockRepository{}
    svc := NewService(mockRepo)
    
    result, err := svc.CreateCustomer(ctx, input)
    
    require.NoError(t, err)
    assert.Equal(t, expected, result)
}
```

### Frontend: Manual Testing
```javascript
// Console testing
OdysseyForm.register('test-form', {
    email: { required: true }
});
OdysseyForm.setValue('test-form', 'email', 'test@example.com');
OdysseyForm.validate('test-form'); // true/false
OdysseyForm.getValues('test-form');
```

---

## Checklist Domain Baru

### Backend
1. [ ] Buat folder `internal/<domain>/`
2. [ ] Definisikan entities di `domain.go`
3. [ ] Definisikan interfaces di `domain.go`
4. [ ] Implementasi repository di `repository.go`
5. [ ] Implementasi service di `service.go`
6. [ ] Implementasi handler di `handler.go`
7. [ ] Mount routes di `internal/app/router.go`
8. [ ] Tulis unit tests di `*_test.go`

### Frontend Feature
1. [ ] Buat folder `features/<feature>/`
2. [ ] Buat `store.js` dengan state + reducer + selectors
3. [ ] Buat `effects.js` untuk side effects
4. [ ] Buat `view.js` untuk DOM rendering
5. [ ] Buat `index.js` dengan init/destroy + event delegation
6. [ ] Import di `main.js`
7. [ ] Expose globally jika perlu (`window.Odyssey<Feature>`)
8. [ ] Buat CSS di `components/<feature>.css`
9. [ ] Import CSS di `main.css`

---

## Anti-Patterns (JANGAN Lakukan)

### Backend

#### ❌ Handler langsung akses DB
```go
func (h *Handler) handleCreate(w, r) {
    db.Exec("INSERT INTO customers...")  // Bypass service!
}
```

#### ❌ Service return HTTP response
```go
func (s *Service) Create(w http.ResponseWriter) {
    w.WriteHeader(201)  // Service tidak boleh tahu HTTP!
}
```

### Frontend

#### ❌ DOM mutation di event handler
```javascript
function handleClick(e) {
    // SALAH - langsung ubah DOM
    e.target.style.display = 'none';
    document.body.classList.add('loading');
}
```

#### ❌ State tersebar di banyak tempat
```javascript
// SALAH - state di global var, DOM, dan objek terpisah
let isOpen = true;
document.body.dataset.modalOpen = 'true';
myModal.state = { open: true };
```

#### ❌ Effect di dalam reducer
```javascript
function reducer(state, action) {
    // SALAH - side effect di reducer
    localStorage.setItem('key', action.payload);
    return { ...state, value: action.payload };
}
```

---

## Quick Reference

| Layer | Backend (Go) | Frontend (JS) |
|-------|--------------|---------------|
| Entry | `cmd/odyssey/main.go` | `main.js` |
| Domain/Feature | `internal/<domain>/` | `features/<feature>/` |
| Data/State | `repository.go` | `store.js` |
| Business/Logic | `service.go` | `effects.js` |
| Presentation | `handler.go` | `view.js` |
| Shared | `internal/shared/` | `core/` |
