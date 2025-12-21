# Odyssey ERP - Quick Reference

## 🚀 Quick Start

```bash
# Docker (Recommended)
docker-compose up -d
make migrate-up
make seed

# Native
./tools/db-setup/setup-db.sh
./tools/scripts/run-background.sh
```

**Access:** http://localhost:8080  
**Login:** `admin@odyssey.local` / `admin123`

## 📂 Project Structure
```
odyssey-erp/
├── documentation/     All docs (START HERE!)
├── tools/             Scripts & DB setup
├── cmd/               Entry points (odyssey, worker)
├── internal/
│   ├── platform/      Infrastructure (DB, Cache, HTTP)
│   ├── sqlc/          Generated database code
│   └── <domain>/      Business logic (e.g., sales, accounting)
├── web/               Frontend assets
└── migrations/        Database migrations
```

## 🏗️ Architecture Reference

### 1. Modular Monolith
- **Domain Isolation:** Each package in `internal/` is a self-contained domain.
- **Dependency Rule:** Domains should not depend on each other directly (use public interfaces or events).
- **Platform Layer:** `internal/platform` handles all infrastructure (DB, Redis, Logging).

### 2. The 7-File Pattern (Per Entity)
Each entity (e.g., `internal/sales/customers`) follows this structure:

| File | Purpose |
|------|---------|
| `model.go` | Domain entity struct |
| `dto.go` | Request/Response structs |
| `repository.go` | Interface + SQLC wrapper |
| `service.go` | Business logic & validation |
| `handler.go` | HTTP handlers (parse -> interact -> respond) |
| `routes.go` | Route definitions (`MountRoutes`) |
| `validation.go` | Input validation rules |

### 3. Database Access (SQLC)
- **Queries:** Defined in `sql/queries/<domain>.sql`.
- **Generation:** Run `sqlc generate` to create code in `internal/sqlc`.
- **Usage:** Repositories import `internal/sqlc` and wrap usage.
- **Transactions:** Use `repo.WithTx(ctx, fn)`.

## 🔧 Common Commands

### Docker
```bash
docker-compose up -d              # Start all services
docker-compose down               # Stop all services
docker-compose logs -f app        # View app logs
docker-compose ps                 # Check status
```

### Native
```bash
./tools/scripts/run.sh            # Run in foreground
./tools/scripts/run-background.sh # Run in background
./tools/scripts/status.sh         # Check status
./tools/scripts/stop.sh           # Stop application
```

### Database
```bash
make migrate-up                   # Run migrations
make migrate-down                 # Rollback migration
make seed                         # Create test account
```

### Development
```bash
make dev                          # Start with docker
make build                        # Build binaries
make test                         # Run tests
make lint                         # Run linter
```

### Coding Standards
- **Imports:** Group stdlib, 3rd party, and internal imports.
- **Errors:** Use `httpx.RespondError` in handlers.
- **Config:** Use type-safe config in `cmd/odyssey/main.go`.
- **Commits:** Follow conventional commits (e.g., `feat(sales): add order creation`).

## 📖 Documentation

| File | Description |
|------|-------------|
| [GETTING_STARTED.md](documentation/GETTING_STARTED.md) | Quick start guide (3 steps) |
| [SETUP_DATABASE.md](documentation/SETUP_DATABASE.md) | Database setup guide |
| [TEST_ACCOUNTS.md](documentation/TEST_ACCOUNTS.md) | Test credentials |
| [RUN_WITHOUT_DOCKER.md](documentation/RUN_WITHOUT_DOCKER.md) | Native installation |

## 🐳 Docker Services

| Service | Port | Description |
|---------|------|-------------|
| app | 8080 | Odyssey ERP application |
| postgres | 5432 | PostgreSQL database |
| redis | 6379 | Redis cache |
| mailpit | 8025 | Email testing UI |
| gotenberg | 3000 | PDF generator |

## 🆘 Troubleshooting

**Port already in use:**
```bash
./tools/scripts/stop.sh
docker-compose down
```

**Database connection error:**
```bash
./tools/db-setup/setup-db.sh
```

**Login not working:**
```bash
make seed  # Recreate test account
```

## 📚 More Help

- 📖 Full docs: `documentation/README.md`
- 🔧 Scripts help: `tools/scripts/README.md`
- 🗄️ DB setup help: `tools/db-setup/README.md`
- 💬 Issues: Check `documentation/QUICK_FIX.md`
