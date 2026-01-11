# Odyssey ERP

Modern ERP system built with Go, PostgreSQL, and Alpine Linux.

## 🚀 Quick Start

```bash
# Start all services
docker-compose up -d

# Run migrations and seed
export PG_DSN='postgres://odyssey:odyssey@localhost:5432/odyssey?sslmode=disable'
make migrate-up
make seed

# Access application
open http://localhost:8080
```

**Login:** `admin@odyssey.local` / `admin123`

## 📖 Documentation

All documentation is in [`docs/`](docs/README.md):

| Section | Description |
|---------|-------------|
| [Getting Started](docs/getting-started/quick-start.md) | Setup & installation |
| [Architecture](docs/architecture/arsitektur.md) | System design |
| [Guides](docs/guides/) | How-to guides & runbooks |
| [Reference](docs/reference/) | Technical reference |
| [ADRs](docs/decisions/) | Architecture decisions |

## 🔧 Development

```bash
# Hot reload (recommended)
~/go/bin/air

# Or run scripts
./tools/scripts/run.sh            # Foreground
./tools/scripts/run-background.sh # Background
./tools/scripts/status.sh         # Check status
./tools/scripts/stop.sh           # Stop
```

## 🐳 Docker Services

| Service | Port | Description |
|---------|------|-------------|
| App | 8080 | Odyssey ERP |
| PostgreSQL | 5432 | Database |
| Redis | 6379 | Cache |
| Mailpit | 8025 | Email testing |
| Gotenberg | 3000 | PDF generator |

## 🏗️ Tech Stack

- **Architecture:** Modular Monolith (Clean Architecture)
- **Backend:** Go 1.24+, Chi router
- **Database:** PostgreSQL 15, sqlc
- **Cache:** Redis 7
- **Frontend:** HTML, Pico CSS (SSR)
- **Container:** Docker with Alpine Linux

## 📝 License

See LICENSE file for details.
