# Odyssey ERP

Modern ERP system built with Go, PostgreSQL, and Alpine Linux.

## 🚀 Quick Start (Docker)

```bash
# Start all services
docker-compose up -d

# Run migrations and seed test account
export PG_DSN='postgres://odyssey:odyssey@localhost:5432/odyssey?sslmode=disable'
make migrate-up
make seed

# Access application
open http://localhost:8080
```

**Default credentials:**
- Email: `admin@odyssey.local`
- Password: `admin123`

## 📁 Project Structure

```
odyssey-erp/
├── cmd/                    # Application entry points
├── internal/              # Internal packages
├── web/                   # Web assets (templates, CSS, JS)
├── migrations/            # Database migrations
├── scripts/               # Build and seed scripts
├── tools/                 # Utility tools
│   ├── scripts/          # Runtime management scripts
│   └── db-setup/         # Database setup scripts
├── documentation/         # Project documentation
├── deploy/               # Deployment configurations
├── docker-compose.yml    # Docker services
└── Dockerfile           # Application container
```

## 📖 Documentation

- [Getting Started](documentation/GETTING_STARTED.md) - Quick start guide
- [Setup Database](documentation/SETUP_DATABASE.md) - Database setup
- [Run Without Docker](documentation/RUN_WITHOUT_DOCKER.md) - Native setup
- [Scripts Usage](documentation/SCRIPTS_USAGE.txt) - Available scripts

## 🔧 Development

```bash
# Run without Docker
./tools/scripts/run.sh

# Run in background
./tools/scripts/run-background.sh

# Check status
./tools/scripts/status.sh

# Stop application
./tools/scripts/stop.sh
```

## 🔥 Hot Reload (Recommended for Development)

Use [Air](https://github.com/air-verse/air) for automatic rebuild on file changes (~3s vs ~80s Docker rebuild).

### Setup

```bash
# Install Air
go install github.com/air-verse/air@latest

# Copy environment file
cp .env.example .env

# Edit .env - change hostnames from Docker to localhost:
# PG_DSN=postgres://odyssey:odyssey@localhost:5432/odyssey?sslmode=disable
# REDIS_ADDR=localhost:6379
# GOTENBERG_URL=http://localhost:3000
# ODYSSEY_TEST_MODE=0  # Required!
```

### Run with Hot Reload

```bash
# Start supporting services (keep Docker for database, redis, etc)
docker-compose up -d postgres redis gotenberg mailpit
docker-compose stop app

# Load environment and start Air
set -a && source .env && set +a
~/go/bin/air

# Or run in background
nohup ~/go/bin/air > /tmp/air.log 2>&1 &
tail -f /tmp/air.log
```

### Stop Hot Reload

```bash
pkill -f "air"
# To return to Docker:
docker-compose start app
```

## 🐳 Docker Services

- **App** - Odyssey ERP (Port 8080)
- **PostgreSQL** - Database (Port 5432)
- **Redis** - Cache (Port 6379)
- **Mailpit** - Email testing (Port 8025)
- **Gotenberg** - PDF generator (Port 3000)

All services use Alpine Linux for minimal footprint and security.

## 🏗️ Tech Stack

- **Backend:** Go 1.24+
- **Database:** PostgreSQL 15
- **Cache:** Redis 7
- **Frontend:** HTML, Pico CSS
- **Container:** Docker with Alpine Linux

## 📝 License

See LICENSE file for details.
