# Odyssey ERP: Developer Setup & Contribution Guide

Welcome to the developer documentation for Odyssey ERP! This guide is designed to help you set up your local development environment, understand the project structure, and contribute to the codebase.

## Prerequisites & Installation

Before you begin, ensure you have the following installed on your machine:
- **Go 1.24+**: The primary programming language.
- **Docker and Docker Compose**: For running backing services and the app in isolated environments.
- **PostgreSQL 17**: The primary database (can be run via Docker).
- **Valkey/Redis 8**: For session storage, caching, and background job queues (can be run via Docker).
- **Air**: For hot reload during development.
- **golangci-lint**: For code linting.
- **sqlc**: For generating Go code from SQL queries.

## Quick Start

1. **Clone the repository:**
   ```bash
   git clone https://github.com/noah-isme/odyssey-erp.git
   cd odyssey-erp
   ```

2. **Set up the environment variables:**
   ```bash
   cp .env.example .env
   ```
   *(Update `.env` as needed. See Configuration Reference below.)*

3. **Start the backing services:**
   ```bash
   docker compose up -d
   ```

4. **Run database migrations and seed data:**
   ```bash
   make migrate-up
   make seed
   ```

5. **Start the application with hot reload:**
   ```bash
   ~/go/bin/air  # Or alternatively, run `make dev`
   ```

Access the application at [http://localhost:4005](http://localhost:4005).

## Configuration Reference

The application is configured using environment variables, typically loaded from a `.env` file in the project root.

| Variable | Default Example | Description |
|----------|-----------------|-------------|
| `APP_PORT` | `4005` | The port the HTTP server listens on. |
| `PG_DSN` | `postgres://odyssey:odyssey@localhost:5432/odyssey?sslmode=disable` | PostgreSQL connection string. |
| `REDIS_ADDR` | `localhost:6379` | Valkey/Redis connection string. |
| `SESSION_SECRET` | `your-secret-key-change-in-production` | Secret key for session signing. |
| `GOTENBERG_URL` | `http://localhost:3000` | URL for the Gotenberg PDF generation service. |
| `APP_ENV` | `development` | The application environment (`development`, `production`, `test`). |

## Make Targets Reference

The `Makefile` provides convenient shortcuts for common tasks:

- `make dev` — Run the full Docker Compose stack in the foreground.
- `make build` — Compile application binaries.
- `make test` — Run Go tests.
- `make lint` — Run `golangci-lint`.
- `make migrate-up` / `make migrate-down` — Apply or rollback database schema migrations.
- `make seed` — Load default seed data into the database.
- `make sqlc-gen` — Regenerate Go bindings from SQL queries using `sqlc`.
- `make docs-check` — Verify documentation links.

## Docker Compose Architecture

The project's docker-compose defines the following services:

- **app**: Go HTTP server, port 4005
- **worker**: Asynq background worker
- **db**: PostgreSQL 17, port 5432
- **redis**: Valkey 8, port 6379
- **gotenberg**: PDF generation, port 3000

## Project Structure Walkthrough

*(Full directory tree is documented in the project's architecture documentation)*
Typical Go layout is used with modules for domain boundaries.

## Development Workflow

1. Create a feature branch
2. Make your changes
3. Run `make lint && make test`
4. Commit your changes with conventional commits
5. Push and create a Pull Request

## Module Development Guide (Step-by-Step)

When adding a new domain module, follow these steps:

1. **Database Schema:** Create a new migration for the tables needed.
2. **SQL Queries:** Add the required CRUD queries in the module-specific file under `sql/queries/`.
3. **Generate Models:** Run `make sqlc-gen` to generate the Go structs and interface.
4. **Service Layer:** Create the service incorporating business logic.
5. **HTTP Handlers:** Create the HTTP handlers to handle web requests. Register the routes.
6. **Templates:** Create the necessary HTML views.

## Database Development

1. Create the next numbered migration files under `migrations/`.
2. Write SQL queries in the module-specific file under `sql/queries/`.
3. Run `make sqlc-gen`
4. Run `make migrate-up`

## Template Development

- **Layout Hierarchy:** `base.html` → `authenticated.html` → page template
- Use partials for reusable elements
- CSS goes in `web/static/css/`
- JS goes in `web/static/js/`

## Testing Guide

- **Unit tests:** `go test ./...`
- **E2E tests:** Playwright (`npx playwright test`)
- **Test env:** `ODYSSEY_TEST_MODE=1 GOTENBERG_URL=http://127.0.0.1:0`

## Debugging Tips

Ensure your backend worker and services in docker-compose are running correctly. Use logs for Postgres and Valkey to inspect persistence and queue issues.

## Deployment Guide

- Docker Compose is intended for production.
- Use the Multi-stage Dockerfile provided.
- Two binaries are required to run: `odyssey` (web app) + `worker` (background jobs).
- Set Environment variables for configuration correctly.

## CI/CD Pipeline

- GitHub Actions run on push and PR.
- The pipeline executes: Lint → Test → Build.

## Contributing Guidelines

Follow conventional commits, standard Go formatting, and ensure the CI passes for all PRs.
