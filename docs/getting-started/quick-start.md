# Quick Start

This is the supported Docker path for evaluating Odyssey locally. It starts the
application and dependencies, runs migrations, and creates the development user.

## Prerequisites

Install Docker with the Compose plugin. For native Go development, use the
[native setup guide](native-setup.md) instead.

## Start Odyssey

```bash
docker compose --profile full up --build
```

Open <http://localhost:8080> once the services are ready. Sign in with
`admin@odyssey.local` / `admin123`.

The full profile runs the one-off `setup` service, which applies migrations and
loads seed data. The default Compose service addresses are internal to Docker;
when running the Go application on the host, use PostgreSQL at `localhost:5432`
and Redis at `localhost:6380`.

## Verify and stop

```bash
curl -fsS http://localhost:8080/healthz
docker compose down
```

For common setup failures, see [troubleshooting](troubleshooting.md). For test
commands, see the [testing runbook](../guides/testing-runbook.md).
