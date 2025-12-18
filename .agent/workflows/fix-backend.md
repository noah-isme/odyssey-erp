---
description: Panduan fix kode backend Go dengan build check dan aturan kebersihan kode
---

# Workflow: Fix Backend Code

## Langkah Wajib

### 1. Jalankan Air untuk Lihat Build Result
// turbo
```bash
~/go/bin/air
```

**Tunggu output dari air** untuk melihat:
- Error kompilasi
- Error runtime
- Lokasi file dan line number

### 2. Analisa Error
- Baca error message dengan **TELITI**
- Identifikasi **ROOT CAUSE**, bukan hanya gejala
- Cek apakah error **SATU** atau **BERANTAI** (chain reaction)

### 3. Fix dengan Pendekatan Minimal
- Fix **SATU ERROR** pada satu waktu
- Jangan ubah kode lain yang tidak terkait
- Prefer **PERBAIKAN KECIL** daripada refactoring besar

### 4. Validasi Ulang
// turbo
```bash
# Lihat output air (sudah hot-reload)
# Atau restart jika perlu:
~/go/bin/air
```

---

## Aturan Ketat Kebersihan Kode

### ❌ DILARANG

| Praktik Buruk | Mengapa |
|---------------|---------|
| Menambah parameter yang tidak digunakan | Dead code, confusion |
| Copy-paste tanpa pahami konteks | Bug tersembunyi |
| Magic number/string hardcoded | Sulit maintenance |
| Ignore error dengan `_` tanpa alasan | Silent failure |
| Panic di dalam handler | Server crash |
| Global mutable state | Race condition |
| Nested if lebih dari 3 level | Unreadable |
| Function lebih dari 50 baris | Terlalu kompleks |

### ✅ WAJIB

| Praktik Baik | Cara |
|--------------|------|
| Handle semua error | `if err != nil { return err }` |
| Log dengan context | `slog.Error("msg", "key", value, "err", err)` |
| Konstanta untuk magic value | `const DefaultLimit = 25` |
| Early return | Kurangi nesting |
| Nama deskriptif | `userID` bukan `u`, `getCustomerByID` bukan `get` |
| Comment untuk logic kompleks | Jelaskan MENGAPA, bukan APA |

---

## Layer-Specific Rules

### Handler Layer
```go
// ✅ BENAR - Thin handler
func (h *Handler) handleGet(w http.ResponseWriter, r *http.Request) {
    id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
    if err != nil {
        shared.RespondError(w, http.StatusBadRequest, "Invalid ID")
        return
    }
    
    result, err := h.service.GetByID(r.Context(), id)
    if err != nil {
        shared.RespondError(w, http.StatusInternalServerError, err.Error())
        return
    }
    
    shared.RespondJSON(w, http.StatusOK, result)
}
```

```go
// ❌ SALAH - Business logic di handler
func (h *Handler) handleGet(w http.ResponseWriter, r *http.Request) {
    // Query langsung ke DB di handler = SALAH
    row := h.db.QueryRow("SELECT * FROM users WHERE id = ?", id)
    // Logic validation di handler = SALAH
    if row.Status == "inactive" && row.CreatedAt.Before(time.Now().AddDate(-1, 0, 0)) {
        // ...
    }
}
```

### Service Layer
```go
// ✅ BENAR - Business logic terisolasi
func (s *Service) ApproveQuotation(ctx context.Context, id, approverID int64) error {
    quot, err := s.repo.GetByID(ctx, id)
    if err != nil {
        return fmt.Errorf("get quotation: %w", err)
    }
    
    if quot.Status != StatusDraft {
        return ErrInvalidStatus
    }
    
    quot.Status = StatusApproved
    quot.ApprovedBy = approverID
    quot.ApprovedAt = time.Now()
    
    return s.repo.Update(ctx, quot)
}
```

### Repository Layer
```go
// ✅ BENAR - Pure data access
func (r *Repo) GetByID(ctx context.Context, id int64) (*Entity, error) {
    return r.queries.GetEntityByID(ctx, id)
}

// ❌ SALAH - Business logic di repository
func (r *Repo) GetByID(ctx context.Context, id int64) (*Entity, error) {
    entity, err := r.queries.GetEntityByID(ctx, id)
    // Validasi bisnis di repo = SALAH
    if entity.Status == "deleted" {
        return nil, ErrNotFound
    }
    return entity, err
}
```

---

## Error Handling Pattern

### Wrap Error dengan Context
```go
// ✅ BENAR
if err != nil {
    return fmt.Errorf("create customer %s: %w", input.Name, err)
}

// ❌ SALAH
if err != nil {
    return err  // Kehilangan context
}
```

### Gunakan Sentinel Error untuk Business Error
```go
// domain.go
var (
    ErrNotFound      = errors.New("not found")
    ErrInvalidStatus = errors.New("invalid status")
    ErrUnauthorized  = errors.New("unauthorized")
)

// handler.go
switch {
case errors.Is(err, domain.ErrNotFound):
    http.Error(w, "Not found", http.StatusNotFound)
case errors.Is(err, domain.ErrUnauthorized):
    http.Error(w, "Forbidden", http.StatusForbidden)
default:
    http.Error(w, "Internal error", http.StatusInternalServerError)
}
```

---

## Pre-Fix Checklist

Sebelum mulai fix, jawab:

1. [ ] Apakah saya **PAHAM** error message-nya?
2. [ ] Apakah saya tahu **FILE dan LINE** yang bermasalah?
3. [ ] Apakah fix ini **MINIMAL** dan tidak merusak yang lain?
4. [ ] Apakah fix ini mengikuti **LAYER ARCHITECTURE**?
5. [ ] Apakah saya **HANDLE ERROR** dengan benar?

---

## Post-Fix Checklist

Setelah fix, pastikan:

1. [ ] Air build **SUKSES** tanpa error
2. [ ] Tidak ada **UNUSED IMPORT** atau variabel
3. [ ] Error di-**WRAP** dengan context
4. [ ] **TIDAK ADA** magic number/string baru
5. [ ] Kode masih **READABLE** dan maintainable

---

## Quick Commands

| Command | Fungsi |
|---------|--------|
| `~/go/bin/air` | Hot reload development |
| `go build ./...` | Build semua package |
| `go vet ./...` | Static analysis |
| `go fmt ./...` | Format code |
| `golangci-lint run` | Linting lengkap |

---

## Common Fixes

### Import Error
```bash
# Error: undefined: SomePackage
go mod tidy
```

### Unused Variable
```go
// Error: declared but not used
// Hapus variabel atau gunakan _
_ = unusedValue
```

### Type Mismatch
```go
// Error: cannot use x (type A) as type B
// Gunakan type conversion atau assertion
b := B(a)  // conversion
b := a.(B) // type assertion
```

### Nil Pointer
```go
// Error: nil pointer dereference
// Selalu check nil sebelum akses
if obj != nil {
    obj.Method()
}
```
