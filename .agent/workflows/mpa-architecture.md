---
description: Panduan arsitektur MPA untuk Odyssey ERP
---

# Odyssey ERP Architecture Guide

## Prinsip Inti: MPA dengan Islands of Interactivity

```
Server-rendered HTML → CSS styling → JS hanya di titik bernilai tinggi
```

---

## 1. MPA sebagai Tulang Punggung

### Aturan
- **Server-rendered** halaman per modul: Finance, Inventory, HR, Purchasing
- Setiap halaman **berdiri sendiri** dan bisa direfresh tanpa rusak
- State penting via **query params**: `?filter=active&sort=date&page=2&tab=details`

### Validasi Desain
> Kalau page direload dan masih "masuk akal", desainnya sehat.

### Contoh URL yang Benar
```
/sales/orders?status=pending&sort=created_at&page=2
/accounting/gl?account=1100&period=2024-01
/inventory/items?category=raw&search=steel
```

### Contoh yang Salah
```
/sales/orders  ← filter di JS state, hilang saat refresh
/#/sales/orders/pending  ← hash routing, tidak SEO friendly
```

---

## 2. Session Klasik, Bukan Sok Modern

### Wajib
- Cookie `httpOnly` + server session (bukan JWT di localStorage)
- CSRF protection di setiap form POST/PUT/DELETE
- Role & permission diputuskan di **server**, bukan di JS

### Arsitektur Session
```go
// Middleware: Load session dari cookie
sess := sessionManager.Load(ctx, r)

// Handler: Cek permission di server
if !rbac.Can(sess.User(), "sales:order:create") {
    http.Error(w, "Forbidden", 403)
    return
}
```

### Prinsip
> Auth ERP yang boring = Auth ERP yang aman.

---

## 3. Islands of Interactivity

### Default: HTML + CSS
Semua halaman harus berfungsi **tanpa JavaScript**.

### JS hanya untuk titik bernilai tinggi:

| Fitur | Pendekatan |
|-------|------------|
| Tabel besar | Pagination server-side, inline edit terbatas |
| Modal form | Vanilla JS, focus trap |
| Autocomplete | Debounced fetch, dropdown |
| Theme toggle | State-driven architecture |
| Sidebar | State-driven + localStorage |

### Tool yang Diizinkan
1. **Vanilla JS** (prioritas utama)
2. **HTMX** untuk partial update (opsional)
3. **Turbo** untuk navigation (opsional)

### DILARANG
- ❌ "Karena ada React, semua harus React"
- ❌ SPA penuh untuk modul standar
- ❌ Client-side routing untuk data entry

---

## 4. Validasi & Bisnis di Server

### Arsitektur Validasi
```
┌─────────────────────────────────────────────────────────┐
│ Client-side (UX cepat, TIDAK authoritative)             │
│ ├── Required field check                                │
│ ├── Format validation (email, phone)                    │
│ └── Disable submit jika invalid                         │
├─────────────────────────────────────────────────────────┤
│ Server-side (SOURCE OF TRUTH)                           │
│ ├── Business rules (credit limit, stock availability)  │
│ ├── Permission check                                    │
│ ├── Database constraints                                │
│ └── Return error per-field, bukan alert global         │
└─────────────────────────────────────────────────────────┘
```

### Response Error yang Benar
```json
{
  "errors": {
    "customer_id": "Customer has exceeded credit limit",
    "qty": "Only 50 units available in stock"
  }
}
```

### Response Error yang Salah
```json
{
  "error": "Validation failed"  // Tidak informatif
}
```

---

## 5. Tabel = Pusat ERP

### Wajib Implement
- [ ] Pagination **server-side** (bukan client-side slice)
- [ ] Filter & sort via **query params**
- [ ] Bulk action via checkbox + single submit
- [ ] Empty state yang informatif
- [ ] Loading state sederhana

### Query Params Convention
| Param | Contoh | Deskripsi |
|-------|--------|-----------|
| `page` | `1` | Halaman aktif (1-indexed) |
| `limit` | `25` | Items per page |
| `sort` | `created_at` | Field untuk sort |
| `order` | `desc` | asc / desc |
| `filter[status]` | `active` | Filter by field |
| `search` | `keyword` | Full-text search |

### Contoh URL
```
/sales/orders?page=2&limit=25&sort=total&order=desc&filter[status]=pending
```

### DILARANG
- ❌ Realtime update kecuali benar-benar perlu
- ❌ Infinite scroll untuk data entry (susah navigate)
- ❌ Virtual scrolling untuk tabel < 1000 rows

---

## 6. Navigasi Cepat tanpa Ilusi

### Strategi
1. **Prefetch link** saat hover (opsional)
2. **Cache HTML** di CDN untuk halaman non-kritis (list views)
3. **Loading state sederhana**: opacity 0.7 + cursor wait

### DILARANG
- ❌ Spinner sok futuristik yang bohong (fake loading)
- ❌ Skeleton screen untuk halaman < 500ms
- ❌ Page transition animation yang delay real content

### CSS Loading State
```css
.is-loading {
    opacity: 0.7;
    pointer-events: none;
    cursor: wait;
}
```

---

## 7. Audit Trail & Idempotency

### Wajib untuk Aksi Penting
- Semua **POST/PUT/DELETE** dengan data penting = idempotency key
- Audit log disimpan di **server**, bukan client
- Timestamp + user + action + before/after state

### Idempotency Implementation
```go
// Header: X-Idempotency-Key: uuid-v4
key := r.Header.Get("X-Idempotency-Key")
if key != "" {
    if result, found := idempotencyCache.Get(key); found {
        // Return cached result
        return result
    }
}
// Process and cache result
```

### Audit Log Schema
```sql
CREATE TABLE audit_logs (
    id BIGSERIAL PRIMARY KEY,
    entity_type VARCHAR(50),  -- 'sales_order', 'customer'
    entity_id BIGINT,
    action VARCHAR(20),       -- 'create', 'update', 'delete'
    user_id BIGINT,
    before_state JSONB,
    after_state JSONB,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
```

### Prinsip
> Audit trail lebih penting dari animasi transisi.

---

## 8. Rencana Eskalasi

### Klasifikasi Modul

#### MPA Selamanya (Low Interactivity)
- Chart of Accounts
- User Management
- Settings
- Reports (PDF export)
- Audit Log viewer

#### MPA + Heavy Islands (Medium Interactivity)
- Sales Orders (inline edit, autocomplete)
- Purchase Orders
- Inventory Transactions
- Journal Entries

#### Kandidat SPA-like Nanti (High Interactivity)
- Dashboard Analytics (charts, realtime)
- POS Terminal
- Production Schedule (drag-drop)
- Kanban Board

### Prinsip Evolusi
```
Start MPA → Add islands → Evaluate → Maybe SPA for specific module
                    ↑
            JANGAN langsung lompat ke SPA
```

### Checklist Sebelum Upgrade ke SPA
- [ ] Apakah reload page benar-benar mengganggu UX?
- [ ] Apakah interaksi > 10 clicks per menit?
- [ ] Apakah ada realtime requirement?
- [ ] Apakah tim punya kapasitas maintain 2 stack?

---

## Checklist Implementasi

### Per Halaman
- [ ] Berfungsi tanpa JS
- [ ] State penting di query params
- [ ] CSRF token di setiap form
- [ ] Error message per-field
- [ ] Loading state sederhana

### Per API
- [ ] Validasi di server
- [ ] Permission check di server
- [ ] Audit log untuk aksi penting
- [ ] Idempotency key untuk POST/PUT

### Per Tabel
- [ ] Pagination server-side
- [ ] Filter/sort via query params
- [ ] Empty state
- [ ] Bulk action support

---

## Anti-Pattern yang Dilarang

| Anti-Pattern | Masalah | Solusi |
|--------------|---------|--------|
| SPA penuh untuk CRUD | Overkill, maintenance berat | MPA + islands |
| JWT di localStorage | XSS vulnerable | Cookie httpOnly |
| Permission check di JS | Mudah bypass | Check di server |
| Infinite scroll | Susah navigate, bookmark | Pagination |
| Skeleton everywhere | Fake modern | Real loading state |
| Hash routing | SEO buruk, refresh masalah | Server routing |
