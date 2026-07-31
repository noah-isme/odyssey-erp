# Odyssey ERP - Quick Reference

Command cheatsheet untuk development dan operations.

## 🐳 Docker

```bash
docker compose up -d                            # Start all services
docker compose up -d postgres redis mailpit gotenberg  # Start infra only (run app separately)
docker compose up -d --build app                # Start all services and rebuild
docker compose down                             # Stop all services
docker compose logs -f app                      # View app logs
docker compose ps                               # Check status
docker compose restart app                      # Restart app only
```

## 🗄️ Database

```bash
make migrate-up                   # Run migrations
make migrate-down                 # Rollback migration
make seed                         # Create test account
make seed-phase4                  # Seed finance data
make refresh-mv                   # Refresh materialized views
```

## 🔥 Hot Reload

```bash
# Install Air
go install github.com/air-verse/air@latest

# Start hot reload
set -a && source .env && set +a
~/go/bin/air
```

## 🧪 Testing

```bash
make test                         # Run all tests
make lint                         # Run linter
make build                        # Build binaries
go test -v ./internal/auth/...    # Test specific package
go test -cover ./...              # With coverage
```

## 📊 Environment

```bash
export PG_DSN='postgres://odyssey:odyssey@localhost:5432/odyssey?sslmode=disable'
export REDIS_ADDR=localhost:6380
export GOTENBERG_URL=http://localhost:3000
```

## 🔐 Default Credentials

| Type | Email | Password |
|------|-------|----------|
| Admin | admin@odyssey.local | admin123 |

## 🌐 Endpoints

| Endpoint | Description |
|----------|-------------|
| http://localhost:8080 | Main application |
| http://localhost:8080/healthz | Health check |
| http://localhost:8025 | Mailpit UI |

## 📚 Documentation

See [`docs/README.md`](docs/README.md) for full documentation.
