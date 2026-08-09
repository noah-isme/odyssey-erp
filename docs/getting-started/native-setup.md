# Native Development Setup

Use this path when running the Go application on the host while Docker provides
local infrastructure.

## Prerequisites

- Go 1.25+
- Docker with the Compose plugin
- `migrate`, `sqlc`, and optionally Air (`make migrate-up`, `make sqlc-gen`, and
  `make air` use binaries from `~/go/bin` by default)

## Start infrastructure

```bash
docker compose up -d postgres redis mailpit gotenberg
export PG_DSN='postgres://odyssey:odyssey@localhost:5432/odyssey?sslmode=disable'
export REDIS_ADDR='localhost:6380'
export GOTENBERG_URL='http://localhost:3000'
export SESSION_SECRET='local-development-session-secret'
export CSRF_SECRET='local-development-csrf-secret'
make migrate-up
make seed
```

## Run the application

```bash
make air
```

Alternatively, run `go run ./cmd/odyssey`. The application is available at
<http://localhost:8080>; Mailpit is at <http://localhost:8025>.

Start `go run ./cmd/worker` in another terminal when exercising background-job
behavior. Stop the infrastructure with `docker compose down` when finished.

For common failures, see [troubleshooting](troubleshooting.md).
