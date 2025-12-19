---
description: Panduan memeeriksa setiap fitur list halaman
---

# Workflow: Standard List Page Architecture

## Prinsip Dasar
Setiap halaman List (Tabel Data) harus mengikuti arsitektur **MPA State-Driven** dimana source of truth adalah **URL Query Parameters**. Frontend hanya merefleksikan state dari Server, dan User Interaction mengubah URL, bukan DOM secara langsung.

---

## 1. Backend Implementation

### A. Domain Layer (Struct Filters)
Setiap fitur list harus memiliki struct Filters yang eksplisit di `domain.go`.

```go
type ListFilters struct {
    Page       int
    Limit      int
    Search     string
    SortBy     string
    SortDir    string // "asc" or "desc"
    // Filter spesifik lain
    Status     string
    SupplierID int64
}
```

### B. Repository Layer (Dynamic Sorting)
Implementasikan sorting dinamis dengan **Whitelist Safety** (mencegah SQL Injection).

```go
// Helper function (private)
func sortOrderEntity(sortBy, sortDir string) string {
    dir := "ASC"
    if sortDir == "desc" { dir = "DESC" }

    switch sortBy {
    case "name": return "name " + dir
    case "created_at": return "created_at " + dir
    case "price": return "price " + dir
    default: return "created_at DESC" // Default fallback wajib ada
    }
}

// Repository Method
func (r *repo) ListEntities(ctx context.Context, filters ListFilters) ([]Entity, int, error) {
    query := `SELECT ... FROM entities WHERE 1=1`
    // ... apply filters ...
    
    // Apply Sorting
    query += " ORDER BY " + sortOrderEntity(filters.SortBy, filters.SortDir)
    
    // Apply Pagination
    // ...
}
```

### C. Handler Layer (Integration)
Handler **WAJIB** mengirimkan kembali state filters ke Template/View agar UI bisa sync dengan URL.

```go
func (h *Handler) listEntities(w http.ResponseWriter, r *http.Request) {
    // 1. Parse URL Params
    filters := ListFilters{
        SortBy:  r.URL.Query().Get("sort"),
        SortDir: r.URL.Query().Get("dir"),
        Search:  r.URL.Query().Get("search"),
        // ...
    }

    // 2. Call Service
    data, count, err := h.service.ListEntities(ctx, filters)

    // 3. Render View passing Filters
    h.render(w, r, "pages/entities_list.html", map[string]any{
        "Data":    data,
        "Filters": filters, // WAJIB ADA untuk UI State
    })
}
```

---

## 2. Template Integration (Frontend View)

Gunakan komponen reusable `data-datatable`. **JANGAN** menulis script JS inline.

### Tabel Header (Sortable)
Header harus memiliki atribut `data-sort-dir` yang diisi secara dinamis dari Filters backend.

```html
<table class="table data-table" data-datatable="entities">
    <thead>
        <tr>
            <!-- Sortable Column -->
            <th scope="col" class="sortable" 
                data-column="name" 
                {{ if eq .Data.Filters.SortBy "name" }}data-sort-dir="{{ .Data.Filters.SortDir }}"{{ end }}>
                
                Name
                <!-- Visual Indicator -->
                {{ if eq .Data.Filters.SortBy "name" }}
                    {{ if eq .Data.Filters.SortDir "asc" }}
                        <span class="sort-icon">↑</span>
                    {{ else }}
                        <span class="sort-icon">↓</span>
                    {{ end }}
                {{ end }}
            </th>

            <!-- Non-sortable Column -->
            <th scope="col">Status</th>
        </tr>
    </thead>
    <tbody>...</tbody>
</table>
```

---

## 3. Reusable Components (Javascript)

### Aturan Frontend
1.  **DILARANG** membuat event listener manual (`onclick`) untuk sorting standar.
2.  Gunakan `web/static/js/features/datatable/index.js` yang sudah ada. Component ini otomatis menangani:
    *   Click event pada `th.sortable`
    *   Navigasi URL (`?sort=...&dir=...`)
    *   Row selection & Context Menu
3.  Jika butuh fitur baru, **update component global**, jangan buat patch lokal.

### Flow Interaksi
1.  User klik Header `Name`.
2.  JS Global (`datatable`) menangkap event.
3.  JS mengupdate `window.location.href` dengan query param baru.
4.  Browser reload page.
5.  Handler menerima request -> Query DB sorted -> Render Template dengan `data-sort-dir` baru.
6.  User melihat hasil terurut dan header visual terupdate.

---

## Checklist Validasi

Sebelum merge list page baru:
- [ ] Apakah `ListFilters` struct ada di domain?
- [ ] Apakah Repository punya whitelist sorting (bukan raw string concat)?
- [ ] Apakah Handler mengirim object `Filters` ke view?
- [ ] Apakah Template menggunakan `.Data.Filters` untuk set `data-sort-dir`?
- [ ] Apakah Tabel menggunakan `data-datatable` attribute?
- [ ] **Tidak ada** inline JS script untuk dealing with table?
