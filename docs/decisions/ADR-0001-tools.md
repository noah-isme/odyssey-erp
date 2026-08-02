# ADR-0001: Pemilihan Tooling Inti

**Status:** Partially superseded. The Go, PostgreSQL, sqlc, Redis, Asynq, and
Gotenberg choices remain current. The CSS decision below records an early phase
and is no longer the active frontend implementation.

## Konteks

Platform Odyssey ERP fase 1 membutuhkan fondasi backend Go yang aman dan mudah dirawat. Kita perlu router ringan, config sederhana, query type-safe, migrasi solid, job queue, dan integrasi PDF.

## Keputusan

1. **Router:** `github.com/go-chi/chi/v5` karena ringan, idiomatik, mendukung middleware chain.
2. **Config:** `github.com/kelseyhightower/envconfig` untuk binding env tanpa refleksi rumit.
3. **DB Driver:** `github.com/jackc/pgx/v5` + `pgxpool` untuk PostgreSQL native.
4. **Migrations:** `github.com/golang-migrate/migrate/v4` karena CLI + library mature.
5. **Query:** `sqlc` menghasilkan kode Go type-safe dari SQL mentah.
6. **Session & Cache:** Redis dengan package `github.com/redis/go-redis/v9`.
7. **Validation:** `github.com/go-playground/validator/v10` de facto.
8. **Background Job:** `github.com/hibiken/asynq` di atas Redis, cocok dengan worker terpisah.
9. **PDF:** Gotenberg (container) untuk konversi HTML -> PDF.
10. **CSS (historical):** Pico.css was selected during the initial phase.

## Konsekuensi

- Tim dapat memahami stack Go standar tanpa framework berat.
- Dependensi minimal, mudah dibawa ke produksi.
- Membutuhkan Docker Compose agar service pendukung (Postgres, Redis, Mailpit, Gotenberg) tersedia.
- Pengembang harus menjalankan `sqlc generate` setelah memodifikasi query SQL.
- The current frontend is a custom component CSS design system under
  `web/static/css`; see `DESIGN.md` and the frontend assets rather than relying
  on the historical Pico.css decision.
