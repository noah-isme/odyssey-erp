# TESTING

## Prasyarat

- Docker & Docker Compose
- Go 1.22+
- sqlc (`go install github.com/kyleconroy/sqlc/cmd/sqlc@latest`)

## Perintah Umum

```bash
make lint
make test
make build
```

## Smoke Test Manual

1. `make dev` (menjalankan app + worker + dependencies).
2. Buka `http://localhost:8080/healthz` → `{"status":"ok"}`.
3. Buka `/auth/login` → form login tampil.
4. POST login invalid → error 400 dengan pesan.
5. POST login valid (setelah seed user) → redirect `/` + flash sukses.
6. POST `/auth/logout` → redirect `/` cookie terhapus.
7. POST `/report/sample` → unduh PDF.

## Pengujian Otomatis

- `go test ./...` mencakup handler smoke test di `internal/auth` & util.
- Migrasi diuji via `migrate -path migrations -database "$PG_DSN" up` (lihat `Makefile`).
