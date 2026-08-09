# Repository Guidelines

## Project Structure & Module Organization
Odyssey is a Go modular monolith. Key paths:
- `cmd/odyssey` HTTP server entrypoint; `cmd/worker` Asynq worker.
- `internal/` domain modules (auth, sales, procurement, etc.), app wiring in `internal/app`, shared helpers in `internal/shared`.
- `jobs/` background job wiring; `report/` Gotenberg client + handlers.
- `web/templates` SSR pages and partials; `web/static` CSS assets.
- `migrations/` schema changes; `sql/queries` and `sqlc.yaml` feed sqlc.
- `docs/` architecture/guides, `scripts/` automation, `testing/` test helpers.

## Build, Test, and Development Commands
- `docker compose up -d` – start app and dependencies; `make dev` runs the full Compose stack in the foreground.
- `~/go/bin/air` – hot reload (see `QUICK_REFERENCE.md`).
- `make lint` – run `golangci-lint`.
- `make test` – run Go tests; `make build` – compile binaries.
- `make migrate-up`/`make migrate-down` – apply/rollback migrations.
- `make seed` – load default data; `make sqlc-gen` – regenerate SQL bindings.

## Coding Style & Naming Conventions
- Follow Go conventions: `gofmt` formatting, `go vet`, `golangci-lint`.
- Package names are lowercase; exported identifiers use CamelCase.
- Tests use `*_test.go` and table-driven patterns (see `docs/guides/handlers.md`).
- SQL lives in `sql/queries` with `-- name: QueryName` blocks for sqlc.

## Testing Guidelines
- Set `ODYSSEY_TEST_MODE=1` and `GOTENBERG_URL=http://127.0.0.1:0` for fast, isolated runs.
- Unit tests: `go test ./...` or `make test`.
- Integration tests live beside the relevant module when present. Run the documented target package rather than assuming every module has an `integration_test.go` or uses `testify/suite`.

## Commit & Pull Request Guidelines
- Commit style follows conventional commits: `type(scope): summary` (e.g., `feat(ui): ...`, `refactor(backend): ...`).
- PRs should include a clear description, test results, and linked issues; add screenshots for `web/` UI changes and update docs when behavior changes.

## Security & Configuration Tips
- Keep secrets out of git; configure via env vars like `PG_DSN`, `REDIS_ADDR`, and `GOTENBERG_URL`.
- Default credentials in `README.md` are for local development only.

## Documentation
- Keep current instructions in this file and current product/developer documentation under `docs/`.
- Put completed phase notes, audits, and superseded runbooks in `docs/archive/` with a clear historical label.
- Run `make docs-check` after changing documentation links or documented Make commands.

## graphify

This project has a knowledge graph at graphify-out/ with god nodes, community structure, and cross-file relationships.

When the user types `/graphify`, use the installed graphify skill or instructions before doing anything else.

Rules:
- For codebase questions, first run `graphify query "<question>"` when graphify-out/graph.json exists. Use `graphify path "<A>" "<B>"` for relationships and `graphify explain "<concept>"` for focused concepts. These return a scoped subgraph, usually much smaller than GRAPH_REPORT.md or raw grep output.
- Dirty graphify-out/ files are expected after hooks or incremental updates; dirty graph files are not a reason to skip graphify. Only skip graphify if the task is about stale or incorrect graph output, or the user explicitly says not to use it.
- If graphify-out/wiki/index.md exists, use it for broad navigation instead of raw source browsing.
- Read graphify-out/GRAPH_REPORT.md only for broad architecture review or when query/path/explain do not surface enough context.
- After modifying code, run `graphify update .` to keep the graph current (AST-only, no API cost).
