# Odyssey ERP – Full-Go Stack Tanpa Framework JS

## Arsitektur & Pola

* **Monolith modular (hexagonal/clean architecture)**: struktur `internal/<domain>` untuk setiap modul ERP (Auth, User/Roles, Inventory, Sales, Purchase, Accounting, HR, Warehouse, Reports).
* **Server-Side Render (SSR)** menggunakan `html/template` + form HTML standar (pagination, sort, filter dilakukan di server).
* **State Auth**: session cookie HTTP-only + rotasi refresh token (hindari localStorage).

---

## Stack Backend (Go)

| Komponen            | Teknologi / Paket                              | Catatan                                          |
| ------------------- | ---------------------------------------------- | ------------------------------------------------ |
| **Runtime**         | Go ≥ 1.22/1.23                                 | range iterators, slog tersedia                   |
| **HTTP Router**     | `chi` atau `gorilla/mux`                       | `chi` lebih ringan dan idiomatik                 |
| **DB Driver**       | `pgx`                                          | PostgreSQL native driver                         |
| **Query Layer**     | `sqlc` atau `ent`                              | `sqlc` disarankan untuk performa & kontrol penuh |
| **Migrations**      | `golang-migrate`                               | CLI + library                                    |
| **Config**          | `envconfig` / `caarlos0/env`                   | tanpa dependensi berat                           |
| **Logging**         | `slog` + `zerolog` / `tint` handler            | standar Go                                       |
| **Validation**      | `go-playground/validator/v10`                  | de facto validator                               |
| **Security**        | `httprate`, CSRF token, `unrolled/secure`      | hardening HTTP                                   |
| **Caching**         | Redis (`go-redis/v9`)                          | untuk session, rate limit, query berat           |
| **Background Jobs** | `hibiken/asynq`                                | Redis-based job queue                            |
| **Scheduling**      | `robfig/cron/v3`                               | cron scheduler                                   |
| **Search/Index**    | PostgreSQL FTS, Meilisearch/Typesense opsional | fleksibel                                        |
| **File Storage**    | MinIO/S3 via `minio-go`                        | MinIO untuk dev, S3 untuk prod                   |
| **Email**           | SMTP + Mailpit                                 | preview email lokal                              |
| **PDF/Report**      | `gofpdf` atau Gotenberg                        | tergantung kompleksitas                          |
| **I18n**            | `go-i18n`                                      | multi-bahasa                                     |

---

## UI Tanpa Framework JS

* **Templating**: `html/template` + komponen partial (`layout`, `navbar`, `table`, `form`).
* **CSS**:

  * Pilihan 1: **Pico.css** atau **Skeleton** (tanpa toolchain).
  * Pilihan 2: **TailwindCSS** (jika build-time dengan Node diperbolehkan).
* **Interaktivitas ringan**:

  * Vanilla JS saja (form helper, masking, datepicker).
  * Opsi opsional: **HTMX** (non-framework, tetap SSR-friendly).

---

## Modul ERP Awal

* **Auth & RBAC**: user, role, permission, session store Redis, CSRF.
* **Organization**: perusahaan, cabang, gudang, satuan, pajak.
* **Master Data**: produk, kategori, supplier, customer, akun (CoA).
* **Inventory & Warehouse**: stok, penyesuaian, transfer, multi-gudang.
* **Procurement**: PR, PO, GRN, AP.
* **Sales**: quotation, SO, delivery, AR.
* **Accounting**: jurnal, GL, neraca, laba rugi.
* **HR (lanjut)**: karyawan, gaji, kehadiran.
* **Reports**: PDF/CSV export, dashboard SSR.

---

## Struktur Direktori Contoh

```bash
/cmd/odyssey
  main.go
/internal
  /app           # router, middleware, DI
  /auth          # domain + repo + service + handler
  /org
  /master
  /inventory
  /sales
  /purchase
  /accounting
  /report
  /shared        # util umum: errors, pagination, response helpers
  /view          # helper untuk template
/migrations
/pkg             # lib publik (jika ada)
/web
  /static        # css, img
  /templates
    /layouts     # base.html
    /partials    # nav.html, flash.html
    /* pages */
```

---

## Pola Handler & View

* **Handler** menerima `r.Context()`, validasi input (query/form), panggil service, dan render `Template(name, data)` atau redirect (PRG pattern).
* **Pagination & Sorting**: whitelist kolom, sanitasi filter.
* **Form CSRF**: token hidden per form, diverifikasi via middleware.

---

## Quality Bar

* **Testing**: `testing`, `testify`, `httpexpect` untuk HTTP test; gunakan Docker PostgreSQL untuk repo test.
* **Lint/Format**: `golangci-lint`, `gofumpt`, `govulncheck`.
* **CI/CD**: GitHub Actions untuk build, lint, test, dan migrate dry-run.
* **Deploy**: Docker Compose (web + postgres + redis + mailpit + gotenberg), lalu ke VM/Bare metal/K8s.

---

## Roadmap Tanpa JS Framework

### Phase 1 – Core Platform

* Bootstrap project, config, router, middleware security.
* Auth + RBAC + session Redis + CSRF.
* Template dasar + Pico.css + layout & flash messages.

### Phase 2 – Master & Org

* Company, branch, warehouse.
* Master data (product, category, unit, tax).
* Import CSV server-side.

### Phase 3 – Inventory & Procurement

* Stock card, adjustment, transfer.
* PR → PO → GRN, AP aging.
* Report stok & pembelian (PDF/CSV).

#### Highlights (Phase 3)

* **Inventory module**: transaction journal (`inventory_tx`) dengan average moving cost, form SSR untuk adjustment/transfer, dan kartu stok PDF.
* **Procurement/AP module**: lifecycle PR→PO→GRN→AP Invoice→Payment, approval single-level, serta integrasi ke inventory saat GRN posting.
* **RBAC & Controls**: permission granular `inventory.*`, `procurement.*`, `finance.ap.*`, approval log di tabel `approvals`, dan idempotency key untuk GRN/adjustment.
* **Jobs & Reports**: tambahan tugas Asynq (`inventory:revaluation`, `procurement:reindex`) dan template PDF (`stock-card`, `grn`).

### Phase 4 – Sales

* Quotation → SO → Delivery.
* AR aging, invoice & receipt PDF.

### Phase 5 – Accounting

* CoA, jurnal otomatis, GL, neraca, laba rugi.
* Lock period & audit trail.

### Phase 6 – Hardening

* Audit log, rate limit, caching, background jobs.
* Backup/restore, observability (pprof, metrics, tracing opsional).

---

## Contoh Dependencies

```bash
go get github.com/go-chi/chi/v5
go get github.com/go-chi/httprate
go get github.com/jackc/pgx/v5
go get github.com/jackc/pgx/v5/pgxpool
go get github.com/kyleconroy/sqlc/cmd/sqlc@latest
go get github.com/golang-migrate/migrate/v4
go get github.com/redis/go-redis/v9
go get github.com/go-playground/validator/v10
go get github.com/robfig/cron/v3
go get github.com/stretchr/testify
```

---

## Tips Implementasi Tanpa JS

* **Tabel**: sorting & pagination via query string; SSR ulang halaman.
* **Form**: gunakan PRG, tampilkan error per field, validasi HTML5 + server.
* **Feedback**: flash message via session.
* **Aksesibilitas**: fokus ring, label ARIA, tab order konsisten.

---

##  Diagram Arsitektur (High-Level)

```text
+-----------------------------------------------------------+
|                       Client (Browser)                    |
|  - HTML form                                              |
|  - Table view (list, pagination)                          |
+---------------------------+-------------------------------+
                            |
                            | HTTP (GET/POST form submit)
                            v
+-----------------------------------------------------------+
|                 HTTP Router (chi / mux)                   |
+---------------------------+-------------------------------+
                            |
                            v
+-----------------------------------------------------------+
| Middleware Stack                                          |
| - Auth session check (Redis session)                      |
| - CSRF verify (per-form token)                            |
| - Rate limit (httprate)                                   |
| - Secure headers (unrolled/secure)                        |
+---------------------------+-------------------------------+
                            |
                            v
+-----------------------------------------------------------+
| Handler per Modul                                         |
|  /auth, /inventory, /sales, /purchase, /accounting, ...   |
|                                                           |
|  Tugas handler:                                           |
|   - Ambil & validasi input (query/form)                   |
|   - Panggil service domain                                |
|   - Pilih view / redirect (PRG)                           |
+---------------------------+-------------------------------+
                            |
                            v
+-----------------------------------------------------------+
| Service / Domain Logic                                    |
| - Aturan bisnis (stok cukup? user punya permission?)      |
| - Orkestrasi beberapa repo / job / cache                  |
|                                                           |
|   |------------------- dependensi -------------------|    |
|   |                                                  |    |
|   v                                                  v    v
|  (A) Repository Layer                          (B) Cache / Session
|      - Query DB via sqlc/pgx                        Redis (session,
|      - Tx control                                   rate limit,
|                                                     cache query berat)
|
|   v                                                   
|  (C) Background Jobs Queue                      
|      Asynq (Redis)                              
|      - Email                                    
|      - Reindex                                  
|      - Rekonsiliasi akunting                    
|
|   v                                                   
|  (D) File/Object Storage                        
|      MinIO / S3 (lampiran PO, invoice PDF)      
|
|   v                                                   
|  (E) PDF / Reporting                           
|      - Gotenberg (HTML -> PDF)                 
|      - gofpdf (tabel laporan)                  
|
|   v                                                   
|  (F) Scheduler / Cron                          
|      robfig/cron (aging AR/AP, backup, dll)    
|
|   v                                                   
|  (G) I18n / Localization                       
|      go-i18n                                   
+-----------------------------------------------------------+
                            |
                            v
+-----------------------------------------------------------+
| View Rendering (SSR)                                      |
| - html/template                                           |
| - Pico.css / Skeleton (tanpa JS framework)                |
| - Flash message dari session                              |
| - Pagination/sort di server                               |
+---------------------------+-------------------------------+
                            |
                            v
+-----------------------------------------------------------+
|             HTML Response kembali ke Browser              |
+-----------------------------------------------------------+
```

>  *"Odyssey" membuktikan: full-Go ERP dengan SSR itu tidak hanya mungkin, tapi juga lebih cepat, aman, dan mudah dirawat. Tidak semua pahlawan butuh JavaScript... cape.*
