# Testing Guide

Panduan lengkap untuk testing Odyssey ERP.

## Prerequisites

- Docker & Docker Compose
- Go 1.22+
- sqlc (`go install github.com/kyleconroy/sqlc/cmd/sqlc@latest`)

## Quick Commands

```bash
make lint     # Run linter
make test     # Run all tests
make build    # Build binaries
```

## Unit Tests

```bash
# Run all unit tests
go test ./...

# Run specific package
go test ./internal/auth/...

# With verbose output
go test -v ./internal/sales/...

# With coverage
go test -cover ./...
```

## Integration Tests

Lihat [Integration Tests Guide](integration-tests.md) untuk detail lengkap.

```bash
# Run integration tests
go test -tags=integration ./test/...
```

## Smoke Test Manual

1. `make dev` (menjalankan app + worker + dependencies)
2. Buka `http://localhost:8080/healthz` → `{"status":"ok"}`
3. Buka `/auth/login` → form login tampil
4. POST login invalid → error 400 dengan pesan
5. POST login valid (setelah seed user) → redirect `/` + flash sukses
6. POST `/auth/logout` → redirect `/` cookie terhapus
7. POST `/report/sample` → unduh PDF

## Database Testing

```bash
# Run migrations up
make migrate-up

# Run migrations down (rollback)
make migrate-down

# Reset database
make migrate-down && make migrate-up && make seed
```

## Test Data

```bash
# Seed test data
make seed

# Seed specific phase data
make seed-phase4
```

## Coverage Reports

```bash
# Generate coverage report
go test -coverprofile=coverage.out ./...

# View in browser
go tool cover -html=coverage.out
```

## CI/CD Testing

GitHub Actions runs:
1. `make lint` - Linting
2. `make test` - Unit tests
3. `make build` - Build verification
4. Migration dry-run

## Historical Testing Docs

Testing documentation untuk phases sebelumnya ada di [archive/](../archive/).
