# Running Odyssey ERP Without Docker

This guide explains how to run Odyssey ERP locally without Docker.

## Prerequisites

Make sure you have the following installed and running:

- **Go** 1.24+ 
- **PostgreSQL** (running on port 5432)
- **Redis** (will be auto-started if not running)

## Quick Start

### 1. Run in Foreground (Interactive)

```bash
./run.sh
```

This will:
- Check Redis and PostgreSQL
- Start Redis if not running
- Run the application in foreground
- Show logs in terminal
- Press `Ctrl+C` to stop

### 2. Run in Background (Daemon)

```bash
./run-background.sh
```

This will:
- Start the application in background
- Create PID file at `/tmp/odyssey-erp.pid`
- Log to `/tmp/odyssey-erp.log`

### 3. Check Status

```bash
./status.sh
```

Shows:
- Application status (RUNNING/STOPPED)
- Redis status
- PostgreSQL status
- HTTP server status
- Recent logs

### 4. Stop Application

```bash
./stop.sh
```

Stops the background application and cleans up processes.

## Access Points

- **Web UI**: http://localhost:8080
- **Metrics**: http://localhost:8080/metrics (if observability enabled)

## Configuration

All environment variables are configured in the scripts:

| Variable | Default Value |
|----------|---------------|
| `APP_ADDR` | `:8080` |
| `PG_DSN` | `postgres://odyssey:odyssey@localhost:5432/odyssey?sslmode=disable` |
| `REDIS_ADDR` | `localhost:6379` |
| `SESSION_SECRET` | `local-dev-session-secret-change-in-production` |
| `CSRF_SECRET` | `local-dev-csrf-secret-change-in-production` |
| `LOG_FORMAT` | `pretty` |

### Change Configuration

Edit the scripts (`run.sh` or `run-background.sh`) to modify environment variables.

For production, **always change** `SESSION_SECRET` and `CSRF_SECRET`!

## Database Setup

If database is not initialized:

```bash
# Run migrations
make migrate-up

# Seed initial data
make seed
```

## Troubleshooting

### Port 8080 already in use

```bash
# Check what's using the port
ss -tlnp | grep 8080

# Or use stop script
./stop.sh
```

### Redis not starting

```bash
# Start Redis manually
redis-server --port 6379 --daemonize yes

# Check Redis status
redis-cli -h localhost -p 6379 ping
```

### PostgreSQL not running

```bash
# Check PostgreSQL status
pg_isready -h localhost -p 5432

# Start PostgreSQL (depends on your system)
sudo systemctl start postgresql  # systemd
sudo service postgresql start     # sysvinit
```

### View Logs

```bash
# Real-time logs
tail -f /tmp/odyssey-erp.log

# Last 50 lines
tail -50 /tmp/odyssey-erp.log

# Search for errors
grep ERROR /tmp/odyssey-erp.log
```

## Development Tips

### Hot Reload (Manual)

When you make code changes:

```bash
./stop.sh
./run-background.sh
```

### Build Binary

Instead of `go run`, you can build a binary:

```bash
go build -o odyssey ./cmd/odyssey/main.go
./odyssey
```

### Run Tests

```bash
make test
```

### Lint Code

```bash
make lint
```

## Comparison with Docker

| Feature | Docker | Without Docker |
|---------|--------|----------------|
| Setup | `docker compose up` | `./run-background.sh` |
| Stop | `docker compose down` | `./stop.sh` |
| Logs | `docker compose logs -f` | `tail -f /tmp/odyssey-erp.log` |
| Rebuild | Auto | Manual restart needed |
| Isolation | ✓ Full | ✗ Shared system |
| PostgreSQL | Included | Manual setup |
| Redis | Included | Auto-start available |

## Production Deployment

For production, consider:

1. **Use systemd service** instead of background scripts
2. **Change secrets** (`SESSION_SECRET`, `CSRF_SECRET`)
3. **Use proper PostgreSQL** with authentication
4. **Enable TLS/HTTPS** with reverse proxy (nginx/caddy)
5. **Set up monitoring** and log aggregation
6. **Use production-grade Redis** with persistence

Example systemd service: see `deploy/systemd/odyssey-erp.service`
