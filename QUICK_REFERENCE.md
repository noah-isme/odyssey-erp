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
├── tools/
│   ├── scripts/      Run/stop/status commands
│   └── db-setup/     Database initialization
├── cmd/              Application entry points
├── internal/         Business logic
├── web/              Frontend assets
└── migrations/       Database migrations
```

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
